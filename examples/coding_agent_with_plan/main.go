package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/flow"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/provider/openai"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/session/sqlite"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/bash"
	"github.com/codeownersnet/goa/tool/difftool"
	"github.com/codeownersnet/goa/tool/editfile"
	"github.com/codeownersnet/goa/tool/glob"
	"github.com/codeownersnet/goa/tool/grep"
	"github.com/codeownersnet/goa/tool/listdir"
	"github.com/codeownersnet/goa/tool/plantool"
	"github.com/codeownersnet/goa/tool/readfile"
	"github.com/codeownersnet/goa/tool/writefile"
)

const (
	reset    = "\033[0m"
	bold     = "\033[1m"
	dim      = "\033[2m"
	red      = "\033[31m"
	green    = "\033[32m"
	yellow   = "\033[33m"
	blue     = "\033[34m"
	cyan     = "\033[36m"
	gray     = "\033[90m"
	yellowBG = "\033[43;30m"
	greenBG  = "\033[42;30m"
)

var iteration int

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("SYNTHETIC_API_KEY")
	if apiKey == "" {
		log.Fatal("SYNTHETIC_API_KEY environment variable is required")
	}

	reg, err := provider.NewRegistry(ctx,
		provider.WithCustomProvider("synthetic", &provider.ProviderInfo{
			ID:      "synthetic",
			Name:    "Synthetic",
			APIBase: "https://api.synthetic.new/openai/v1",
			EnvVars: []string{"SYNTHETIC_API_KEY"},
			Type:    "openai_compat",
		}),
		provider.WithFactory("openai_compat", &openai.Factory{}),
	)
	if err != nil {
		log.Fatal(err)
	}

	m, err := reg.Resolve(ctx, "synthetic/hf:zai-org/GLM-4.7-Flash",
		provider.WithAPIKey(apiKey),
	)
	if err != nil {
		log.Fatal(err)
	}

	workDir := filepath.Join(os.TempDir(), "goa-plan-coding-agent-v4")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		log.Fatal(err)
	}
	allowedPaths := []string{workDir + "/**"}

	planBundle, err := plantool.NewBundle()
	if err != nil {
		log.Fatal(err)
	}

	bashTool, err := bash.New(bash.Config{
		WorkDir:         workDir,
		Timeout:         60,
		AllowedCommands: []string{"go", "gofmt", "ls", "pwd", "test"},
		DeniedCommands:  []string{"cd", "cat", "mkdir", "sh", "bash"},
	})
	if err != nil {
		log.Fatal(err)
	}

	readTool, err := readfile.New(readfile.Config{AllowedPaths: allowedPaths})
	if err != nil {
		log.Fatal(err)
	}

	writeTool, err := writefile.New(writefile.Config{AllowedPaths: allowedPaths, CreateDirs: true})
	if err != nil {
		log.Fatal(err)
	}

	editTool, err := editfile.New(editfile.Config{AllowedPaths: allowedPaths})
	if err != nil {
		log.Fatal(err)
	}

	globTool, err := glob.New(glob.Config{AllowedPaths: allowedPaths})
	if err != nil {
		log.Fatal(err)
	}

	grepTool, err := grep.New(grep.Config{AllowedPaths: allowedPaths})
	if err != nil {
		log.Fatal(err)
	}

	listDirTool, err := listdir.New(listdir.Config{AllowedPaths: allowedPaths})
	if err != nil {
		log.Fatal(err)
	}

	diffTool, err := difftool.New(difftool.Config{AllowedPaths: allowedPaths})
	if err != nil {
		log.Fatal(err)
	}

	tools := append([]tool.Tool{}, planBundle.Tools...)
	tools = append(tools,
		bashTool, readTool, writeTool, editTool,
		globTool, grepTool, listDirTool, diffTool,
	)

	codingAgent, err := llmagent.New(llmagent.Config{
		Name:  "coding-agent-with-plan",
		Model: m,
		Instruction: `You are a coding assistant with file and shell tools. ` +
			`Before modifying files, create a plan with plan_create. Pass plan_create steps as an array of plain strings, not objects. ` +
			`Call plan_show before starting and after finishing to display the plan to the user. ` +
			`Work one step at a time and call plan_update after completing or skipping each step. ` +
			`If a plan reminder is present, resume from its current step instead of starting over. ` +
			`The workspace directory already exists. Use write_file and edit_file with absolute paths for file changes. ` +
			`Do not use shell redirection, heredocs, cat >, mkdir, or cd. Bash already runs in the workspace. ` +
			`Verify code changes before reporting success.`,
		Tools:             tools,
		RequestProcessors: []flow.RequestProcessor{planBundle.Reminder},
	})
	if err != nil {
		log.Fatal(err)
	}

	sessionService, err := sqlite.NewService(ctx, sqlite.Config{Path: filepath.Join(workDir, "sessions.db")})
	if err != nil {
		log.Fatal(err)
	}
	defer sessionService.Close()

	r, err := runner.New(runner.Config{
		AppName:           "coding-agent-with-plan",
		Agent:             codingAgent,
		SessionService:    sessionService,
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n%s==============================================%s\n", blue, reset)
	fmt.Printf("  %sWorkspace%s: %s%s%s\n", bold, reset, dim, workDir, reset)
	fmt.Printf("  %sSession%s:   %s%s%s\n", bold, reset, dim, filepath.Join(workDir, "sessions.db"), reset)
	fmt.Printf("  %sModel%s:     %sGLM-4.7-Flash%s\n", bold, reset, dim, reset)
	fmt.Printf("  %sNote%s:      Re-run to resume from an existing plan.\n", bold, reset)
	fmt.Printf("%s==============================================%s\n\n", blue, reset)

	userMsg := content.NewTextContent(
		"Create a tiny Go CLI in the existing workspace "+workDir+" that prints a generated request ID using only the standard library. "+
			"Write "+filepath.Join(workDir, "go.mod")+" and "+filepath.Join(workDir, "main.go")+", format the code, run go test ./..., and report the result.",
		content.RoleUser,
	)

	fmt.Printf("\n%s%s[USER]%s %s\n\n", yellowBG, bold, reset, userMsg.Parts[0].Text.Text)

	for ev, err := range r.Run(ctx, "user1", "plan-session-v4", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatalf("%s%s ERROR %s %v", red, red, reset, err)
		}
		handleEvent(ev)
	}
	fmt.Println()

	fmt.Printf("\n%s==============================================%s\n", green, reset)
	fmt.Printf("  %sDONE%s\n", bold, reset)
	fmt.Printf("%s==============================================%s\n\n", green, reset)
}

func handleEvent(ev *session.Event) {
	if ev == nil {
		return
	}

	if ev.Author == "system" {
		if text := ev.Text(); text != "" {
			printSystemMessage(text)
		}
		return
	}

	if ev.Partial {
		return
	}

	if ev.Author != "" {
		iteration++
		fmt.Printf("\n%s%s[%s] Iteration %3d  Agent: %s%s\n", dim, gray, timestamp(), iteration, ev.Author, reset)
		fmt.Printf("%s%s%s\n", dim, strings.Repeat("-", 58), reset)
	}

	printAgentText("AGENT", green, ev.Text())
	printToolRequests("CALLS", cyan, ev.ModelResponse)
	printPlanShowResult(ev.ModelResponse)
	printToolResults(ev.ModelResponse)
	printFileWriteActions(ev.ModelResponse)
}

func timestamp() string {
	return fmt.Sprintf("%02d:%02d:%02d",
		os.Getpid()/1000000, os.Getpid()/1000%1000, os.Getpid()%1000)
}

func printSystemMessage(text string) {
	fmt.Printf("\n%s%s[SYSTEM]%s %s\n", yellow, bold, reset, strings.TrimSpace(text))
}

func printAgentText(label string, color string, text string) {
	if text = strings.TrimSpace(text); text == "" {
		return
	}
	if strings.Contains(text, "[PLAN REMINDER]") {
		return
	}
	fmt.Printf("\n%s%s[%s]%s\n%s\n", color, bold, label, reset, text)
}

func printToolRequests(label string, color string, resp *content.Content) {
	if resp == nil {
		return
	}
	var calls []string
	for _, part := range resp.Parts {
		if part.FunctionCall == nil {
			continue
		}
		name := part.FunctionCall.Name
		args := part.FunctionCall.Args
		var kv []string

		numericKeys := []string{"step_id", "path", "content", "command", "status"}
		for _, k := range numericKeys {
			if v, ok := args[k]; ok {
				var s string
				switch val := v.(type) {
				case string:
					s = val
					if len(s) > 60 {
						s = s[:57] + "..."
					}
				case []any:
					var items []string
					for i, item := range val {
						if i >= 3 {
							items = append(items, "...")
							break
						}
						if s, ok := item.(string); ok {
							items = append(items, s)
						}
					}
					s = fmt.Sprintf("[%s]", strings.Join(items, ", "))
				default:
					s = fmt.Sprintf("%v", v)
				}
				kv = append(kv, fmt.Sprintf("%s=%s", k, s))
			}
		}
		var extra []string
		for k, v := range args {
			if k == "step_id" || k == "path" || k == "content" || k == "command" || k == "status" {
				continue
			}
			s := fmt.Sprintf("%v", v)
			if len(s) > 40 {
				s = s[:37] + "..."
			}
			extra = append(extra, fmt.Sprintf("%s=%s", k, s))
		}
		kv = append(kv, extra...)

		callStr := fmt.Sprintf("  → %s", name)
		if len(kv) > 0 {
			callStr += " " + strings.Join(kv, " ")
		}
		calls = append(calls, callStr)
	}
	if len(calls) == 0 {
		return
	}
	fmt.Printf("\n%s%s[%s]%s\n", color, bold, label, reset)
	for _, c := range calls {
		fmt.Println(c)
	}
}

func printToolResults(resp *content.Content) {
	if resp == nil {
		return
	}
	for _, part := range resp.Parts {
		if part.FunctionResponse == nil {
			continue
		}
		fr := part.FunctionResponse
		if fr.IsError {
			fmt.Printf("  %s%s[ERROR]%s %s %q\n", red, bold, reset, fr.Name, fr.Response["error"])
			continue
		}
		if fr.Name != "plan_show" {
			if result, ok := fr.Response["result"].(string); ok && result != "" {
				short := result
				if len(short) > 120 {
					short = short[:117] + "..."
				}
				fmt.Printf("  %s%s[RESULT]%s %s: %s\n", dim, gray, reset, fr.Name, short)
			} else {
				if len(fr.Response) > 0 {
					short := fmt.Sprintf("%v", fr.Response)
					if len(short) > 120 {
						short = short[:117] + "..."
					}
					fmt.Printf("  %s%s[RESULT]%s %s: %s\n", dim, gray, reset, fr.Name, short)
				}
			}
		}
	}
}

func printFileWriteActions(resp *content.Content) {
	if resp == nil {
		return
	}
	for _, part := range resp.Parts {
		if part.FunctionCall == nil {
			continue
		}
		if part.FunctionCall.Name == "write_file" {
			if path, ok := part.FunctionCall.Args["path"].(string); ok {
				fmt.Printf("  %s%s[IO]%s WROTE %s%s%s\n", yellow, bold, reset, dim, path, reset)
			}
		}
	}
}

func printPlanShowResult(resp *content.Content) {
	if resp == nil {
		return
	}
	for _, part := range resp.Parts {
		if part.FunctionResponse == nil {
			continue
		}
		if part.FunctionResponse.Name == "plan_show" {
			if result, ok := part.FunctionResponse.Response["display_text"].(string); ok {
				// Only print if there's substantial content (not just the frame)
				if len(result) > 50 {
					lines := strings.Split(strings.TrimSpace(result), "\n")
					if len(lines) > 3 {
						fmt.Printf("\n%s%s[PLAN DISPLAY]%s\n", blue, bold, reset)
						for _, line := range lines {
							fmt.Println(line)
						}
					}
				}
			}
		}
	}
}
