package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"strings"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/sequentialagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
)

func greetingRun(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		input := lastUserText(ctx)
		text := fmt.Sprintf("Hello, %s!", input)
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content.NewTextContent(text, content.RoleModel)
		event.Author = ctx.Agent().Name()
		yield(event, nil)
	}
}

func farewellRun(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		input := lastUserText(ctx)
		text := fmt.Sprintf("Goodbye, %s! Have a great day.", input)
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content.NewTextContent(text, content.RoleModel)
		event.Author = ctx.Agent().Name()
		yield(event, nil)
	}
}

func lastUserText(ctx agent.InvocationContext) string {
	var texts []string
	for ev := range ctx.Session().Events().All() {
		if ev.Author == "user" && ev.ModelResponse != nil {
			for _, part := range ev.ModelResponse.Parts {
				if part.Text != nil {
					texts = append(texts, part.Text.Text)
				}
			}
		}
	}
	if len(texts) == 0 {
		return ""
	}
	return strings.TrimSpace(texts[len(texts)-1])
}

func main() {
	ctx := context.Background()

	greeter := agent.New(
		agent.WithName("greeter"),
		agent.WithDescription("greets the user"),
		agent.WithRun(greetingRun),
	)

	fareweller := agent.New(
		agent.WithName("fareweller"),
		agent.WithDescription("says goodbye to the user"),
		agent.WithRun(farewellRun),
	)

	seqAgent, err := sequentialagent.New(sequentialagent.Config{
		Name:        "sequential-demo",
		Description: "A sequential agent that runs greeting then farewell",
		SubAgents:   []agent.Agent{greeter, fareweller},
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "sequential-agent",
		Agent:             seqAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("World", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		if text := event.Text(); text != "" {
			fmt.Println(text)
		}
	}
}
