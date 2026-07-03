package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/agent/loopagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/provider/openai"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/exitlooptool"
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

	exitTool, err := exitlooptool.New()
	if err != nil {
		log.Fatal(err)
	}

	refinerAgent, err := llmagent.New(llmagent.Config{
		Name:        "refiner",
		Description: "Iteratively refines text",
		Model:       m,
		Instruction: "You are a text refiner. Improve the user's text to make it more professional and polished. " +
			"If the text is already good enough, call the exit_loop tool to finish. " +
			"Otherwise, output the improved version.",
		Tools: []tool.Tool{exitTool},
	})
	if err != nil {
		log.Fatal(err)
	}

	loopAgent, err := loopagent.New(loopagent.Config{
		Name:          "loop-llm-demo",
		Description:   "Iteratively refines text up to 3 times",
		SubAgents:     []agent.Agent{refinerAgent},
		MaxIterations: 3,
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "loop-llm-agent",
		Agent:             loopAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("The product is ok but could be better.", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		if text := event.Text(); text != "" {
			fmt.Println(text)
		}
	}
}
