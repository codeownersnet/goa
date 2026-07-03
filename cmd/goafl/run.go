package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/provider/anthropic"
	"github.com/codeownersnet/goa/provider/openai"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/session/sqlite"
	"github.com/codeownersnet/goa/skill"
	toolregistry "github.com/codeownersnet/goa/tool/registry"
	"github.com/codeownersnet/goa/workflow"
)

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	sessionID := fs.String("session", "", "session ID (auto-generated if omitted)")
	userID := fs.String("user", "cli", "user ID")
	storeDir := fs.String("store", ".goa/state", "session storage directory")
	modelOverride := fs.String("model", "", "override all step models with this model string")
	offline := fs.Bool("offline", false, "don't fetch models.dev catalog")
	verbose := fs.Bool("verbose", false, "print partial events")
	workDir := fs.String("workdir", "", "working directory for file tools (defaults to current directory)")

	var providerIDs providerFlagSlice
	var providerBases providerFlagSlice
	var providerEnvs providerFlagSlice
	var providerTypes providerFlagSlice
	fs.Var(&providerIDs, "provider-id", "custom provider ID (repeatable, paired by position with other provider flags)")
	fs.Var(&providerBases, "provider-base", "custom provider API base URL (repeatable)")
	fs.Var(&providerEnvs, "provider-env", "custom provider env var for API key (repeatable, optional)")
	fs.Var(&providerTypes, "provider-type", "custom provider type (repeatable, optional, defaults to openai_compat)")

	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: goafl run [flags] <workflow> [prompt]")
		os.Exit(1)
	}
	workflowPath := resolveWorkflowPath(fs.Arg(0))

	prompt := "run workflow"
	if fs.NArg() > 1 {
		prompt = fs.Arg(1)
	}

	ctx := context.Background()

	wd := *workDir
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: get working directory: %v\n", err)
			os.Exit(1)
		}
	}
	absWd, err := filepath.Abs(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve working directory: %v\n", err)
		os.Exit(1)
	}
	allowedPaths := []string{absWd + "/**"}

	toolReg := toolregistry.DefaultBuiltinRegistry(
		toolregistry.WithBuiltinAllowedPaths(allowedPaths),
	)

	skillReg := skill.NewRegistry()
	_ = skillReg.Discover()

	provOpts := []provider.RegistryOption{
		provider.WithFactory("openai_compat", &openai.Factory{}),
		provider.WithFactory("anthropic", &anthropic.Factory{}),
	}

	for i, id := range providerIDs {
		if i >= len(providerBases) {
			fmt.Fprintf(os.Stderr, "error: --provider-id %q missing corresponding --provider-base\n", id)
			os.Exit(1)
		}
		provType := "openai_compat"
		if i < len(providerTypes) && providerTypes[i] != "" {
			provType = providerTypes[i]
		}
		var envVars []string
		if i < len(providerEnvs) && providerEnvs[i] != "" {
			envVars = []string{providerEnvs[i]}
		} else {
			envVars = []string{strings.ToUpper(id) + "_API_KEY"}
		}
		provOpts = append(provOpts, provider.WithCustomProvider(id, &provider.ProviderInfo{
			ID:      id,
			Name:    id,
			APIBase: providerBases[i],
			EnvVars: envVars,
			Type:    provType,
		}))
	}

	if *offline {
		provOpts = append(provOpts, provider.WithRegistryOffline())
	}

	provReg, err := provider.NewRegistry(ctx, provOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: provider registry: %v\n", err)
		os.Exit(1)
	}

	loadOpts := []workflow.LoadOption{
		workflow.WithProviderRegistry(provReg),
		workflow.WithToolRegistry(toolReg),
		workflow.WithSkillRegistry(skillReg),
		workflow.WithAllowedPaths(allowedPaths),
	}
	if *modelOverride != "" {
		loadOpts = append(loadOpts, workflow.WithModelOverride(*modelOverride))
	}

	wf, err := workflow.Load(ctx, workflowPath, loadOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		if _, ok := err.(*workflow.WorkflowError); ok {
			os.Exit(2)
		}
		os.Exit(1)
	}
	defer wf.Close()

	sessID := *sessionID
	if sessID == "" {
		sessID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	if err := os.MkdirAll(*storeDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: create store dir: %v\n", err)
		os.Exit(1)
	}

	sessSvc, err := sqlite.NewService(ctx, sqlite.Config{
		Path: *storeDir + "/sessions.db",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: sqlite: %v\n", err)
		os.Exit(1)
	}

	artifactSvc := artifact.InMemoryService()
	memorySvc := memory.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:           wf.Name(),
		Agent:             wf.Agent(),
		SessionService:    sessSvc,
		ArtifactService:   artifactSvc,
		MemoryService:     memorySvc,
		AutoCreateSession: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: runner: %v\n", err)
		os.Exit(1)
	}

	userMsg := content.NewTextContent(prompt, content.RoleUser)

	exitCond := wf.ExitCondition()
	start := time.Now()

	for event, err := range r.Run(ctx, *userID, sessID, userMsg, agent.RunConfig{}) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if event == nil {
			continue
		}
		if event.Partial && !*verbose {
			continue
		}
		if text := event.Text(); text != "" {
			fmt.Printf("[%s] %s\n", event.Author, text)
		}
		if !event.Partial && exitCond != nil {
			state := make(map[string]any)
			sess, serr := sessSvc.Get(ctx, &session.GetRequest{
				AppName:   wf.Name(),
				UserID:    *userID,
				SessionID: sessID,
			})
			if serr == nil && sess != nil {
				for k, v := range sess.Session.State().All() {
					state[k] = v
				}
			}
			elapsed := time.Since(start)
			if exitCond(state, elapsed) {
				fmt.Fprintln(os.Stderr, "Exit criteria met")
				os.Exit(0)
			}
		}
	}

	log.Println("Workflow completed")
}

type providerFlagSlice []string

func (p *providerFlagSlice) String() string         { return strings.Join(*p, ", ") }
func (p *providerFlagSlice) Set(value string) error { *p = append(*p, value); return nil }
