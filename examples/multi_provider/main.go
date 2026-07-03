package main

import (
	"context"
	"fmt"
	"log"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/provider"
	"github.com/codeownersnet/goa/provider/anthropic"
	"github.com/codeownersnet/goa/provider/openai"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
)

func main() {
	ctx := context.Background()

	reg, err := provider.NewRegistry(ctx)
	if err != nil {
		log.Fatal(err)
	}
	reg.RegisterFactory("openai_compat", &openai.Factory{})
	reg.RegisterFactory("anthropic", &anthropic.Factory{})

	models := []string{
		"openai/gpt-4o",
		"anthropic/claude-4-opus",
		"openrouter/deepseek/deepseek-r1",
	}

	for _, modelName := range models {
		fmt.Printf("\n=== Using %s ===\n", modelName)

		m, err := reg.Resolve(ctx, modelName)
		if err != nil {
			fmt.Printf("  (skipped: %v)\n", err)
			continue
		}

		myAgent, err := llmagent.New(llmagent.Config{
			Name:        "multi-agent",
			Model:       m,
			Instruction: "You are a helpful assistant. Respond in one sentence.",
		})
		if err != nil {
			log.Fatal(err)
		}

		r, err := runner.New(runner.Config{
			AppName:        "multi-provider",
			Agent:          myAgent,
			SessionService: session.InMemoryService(),
		})
		if err != nil {
			log.Fatal(err)
		}

		userMsg := content.NewTextContent("What is 2+2?", content.RoleUser)
		for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
			if err != nil {
				fmt.Printf("  (error: %v)\n", err)
				break
			}
			fmt.Print(event.Text())
		}
	}
}
