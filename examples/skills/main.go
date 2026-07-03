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
	"github.com/codeownersnet/goa/skill"
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

	skillReg := skill.NewRegistry(
		skill.WithRunScripts(true),
	)
	if err := skillReg.Discover(); err != nil {
		log.Printf("skill discovery: %v", err)
	}

	availableSkills := skillReg.List()
	if len(availableSkills) > 0 {
		fmt.Println("Available skills:")
		for _, s := range availableSkills {
			fmt.Printf("  - %s: %s\n", s.Name, s.Description)
		}
	} else {
		fmt.Println("No skills found. Create one in .agents/skills/ or .goa/skills/")
	}

	myAgent, err := llmagent.New(llmagent.Config{
		Name:        "skilled-agent",
		Model:       m,
		Instruction: "You are a helpful assistant with specialized skills.",
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "skills-demo",
		Agent:          myAgent,
		SessionService: session.InMemoryService(),
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("Hello!", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(event.Text())
	}
}
