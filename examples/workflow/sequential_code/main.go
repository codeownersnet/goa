package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/agent/sequentialagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/provider/openai"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
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

	m, err := reg.Resolve(ctx, "synthetic/hf:zai-org/GLM-4.7-Flash",
		provider.WithAPIKey(apiKey),
	)
	if err != nil {
		log.Fatal(err)
	}

	writerAgent, err := llmagent.New(llmagent.Config{
		Name:        "writer",
		Description: "Generates Python code",
		Model:       m,
		Instruction: "You are a Python code generator. Write a simple hello-world program. Output only the Python code, no explanations.",
	})
	if err != nil {
		log.Fatal(err)
	}

	reviewerAgent, err := llmagent.New(llmagent.Config{
		Name:        "reviewer",
		Description: "Reviews Python code",
		Model:       m,
		Instruction: "You are a Python code reviewer. Review the code provided to you. Give brief constructive feedback in one or two sentences.",
	})
	if err != nil {
		log.Fatal(err)
	}

	refactorerAgent, err := llmagent.New(llmagent.Config{
		Name:        "refactorer",
		Description: "Refactors Python code based on review",
		Model:       m,
		Instruction: "You are a Python expert. Based on the review comments provided to you, refactor the code. Output only the final Python code, no explanations.",
	})
	if err != nil {
		log.Fatal(err)
	}

	codePipeline, err := sequentialagent.New(sequentialagent.Config{
		Name:        "code-pipeline",
		Description: "Sequential pipeline: write -> review -> refactor",
		SubAgents:   []agent.Agent{writerAgent, reviewerAgent, refactorerAgent},
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "code-pipeline",
		Agent:             codePipeline,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("Write a hello world program.", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		if text := event.Text(); text != "" {
			fmt.Println(text)
		}
	}
}
