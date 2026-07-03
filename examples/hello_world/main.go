package main

import (
	"context"
	"fmt"
	"log"
	"os"

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

	modelName := "openrouter/deepseek/deepseek-r1"
	if len(os.Args) > 1 {
		modelName = os.Args[1]
	}

	m, err := reg.Resolve(ctx, modelName)
	if err != nil {
		log.Fatalf("resolve model: %v", err)
	}

	myAgent, err := llmagent.New(llmagent.Config{
		Name:        "hello-agent",
		Model:       m,
		Instruction: "You are a helpful assistant. Be concise.",
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "hello-world",
		Agent:          myAgent,
		SessionService: session.InMemoryService(),
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("Say hello in 3 languages.", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(event.Text())
	}
}
