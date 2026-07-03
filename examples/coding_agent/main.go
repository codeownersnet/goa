package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/artifact"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/memory"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/provider/openai"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/skill"
	"github.com/codeownersnet/goa/skill/skilltool"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/bash"
	"github.com/codeownersnet/goa/tool/difftool"
	"github.com/codeownersnet/goa/tool/editfile"
	"github.com/codeownersnet/goa/tool/glob"
	"github.com/codeownersnet/goa/tool/grep"
	"github.com/codeownersnet/goa/tool/listdir"
	"github.com/codeownersnet/goa/tool/readfile"
	"github.com/codeownersnet/goa/tool/writefile"
)

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

	m, err := reg.Resolve(ctx, "synthetic/hf:zai-org/GLM-5.1",
		provider.WithAPIKey(apiKey),
	)
	if err != nil {
		log.Fatal(err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	workDir := homeDir + "/temporary/helloworld"
	allowedPaths := []string{workDir + "/**"}

	skillReg := skill.NewRegistry()
	if err := skillReg.Discover(); err != nil {
		log.Printf("skill discovery: %v", err)
	}
	availableSkills := skillReg.List()
	if len(availableSkills) > 0 {
		fmt.Printf("Discovered %d skills.\n", len(availableSkills))
	}
	skillPrompt := ""
	if len(availableSkills) > 0 {
		skillPrompt = "\n\nAvailable skills are listed below. When the task matches an available skill, call activate_skill before doing the work and follow the returned instructions.\n" + skill.ToPromptXML(availableSkills)
	}
	if _, ok := skillReg.Lookup("golang"); ok {
		golangSkill, err := skillReg.Activate("golang")
		if err != nil {
			log.Printf("activate golang skill: %v", err)
		} else {
			fmt.Println("Activated skill: golang")
			skillPrompt += "\n\nThe golang skill is already activated for this Go coding task. Follow these instructions:\n" + formatSkillContent(golangSkill)
		}
	}

	bashTool, err := bash.New(bash.Config{
		WorkDir:         workDir,
		Timeout:         60,
		AllowedCommands: []string{"go", "ls", "cat", "mkdir", "rm", "cp", "mv", "pwd", "echo", "test", "which", "head", "tail", "wc", "touch", "chmod", "git"},
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

	codingAgent, err := llmagent.New(llmagent.Config{
		Name:  "coding-agent",
		Model: m,
		Instruction: "You are a coding assistant with file and shell tools. " +
			"When asked to create a program, write the file, verify it compiles, and report the result." + skillPrompt,
		Tools: []tool.Tool{
			bashTool, readTool, writeTool, editTool,
			globTool, grepTool, listDirTool, diffTool,
			skilltool.NewActivateTool(skillReg), skilltool.NewResourceTool(skillReg),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "coding-agent",
		Agent:             codingAgent,
		SessionService:    session.InMemoryService(),
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memory.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent(
		"Create a Go hello world program in "+workDir+
			". Write the file, verify it compiles with go build, and report the result.",
		content.RoleUser,
	)

	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		if text := event.Text(); text != "" {
			fmt.Print(text)
		}
		if event.ModelResponse != nil {
			for _, part := range event.ModelResponse.Parts {
				if part.FunctionCall != nil {
					args := part.FunctionCall.Args
					if s, ok := args["command"].(string); ok {
						fmt.Printf("[%s: %s]\n", part.FunctionCall.Name, s)
					} else if s, ok := args["path"].(string); ok {
						fmt.Printf("[%s: %s]\n", part.FunctionCall.Name, s)
					} else if s, ok := args["name"].(string); ok {
						fmt.Printf("[%s: %s]\n", part.FunctionCall.Name, s)
					} else {
						fmt.Printf("[%s]\n", part.FunctionCall.Name)
					}
				}
			}
		}
	}
	fmt.Println()
}

func formatSkillContent(s *skill.Skill) string {
	return fmt.Sprintf("<skill_content name=%q>\n%s\n</skill_content>", s.Name, s.Body)
}
