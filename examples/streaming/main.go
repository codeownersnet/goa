package main

import (
	"context"
	"fmt"
	"log"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/llmagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/provider"
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

	m, err := reg.Resolve(ctx, "openai/gpt-4o")
	if err != nil {
		log.Fatal(err)
	}

	myAgent, err := llmagent.New(llmagent.Config{
		Name:        "stream-agent",
		Model:       m,
		Instruction: "You are a helpful assistant. Be detailed.",
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "streaming",
		Agent:          myAgent,
		SessionService: session.InMemoryService(),
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("Explain quantum computing in simple terms.", content.RoleUser)

	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	}) {
		if err != nil {
			log.Fatal(err)
		}
		if event.Partial {
			fmt.Print(event.Text())
		} else {
			fmt.Println()
			fmt.Println("--- Complete ---")
		}
	}
}
