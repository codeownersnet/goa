package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/parallelagent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
)

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

func fastAgentRun(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		time.Sleep(time.Duration(1+rand.Intn(10)) * time.Millisecond)
		input := lastUserText(ctx)
		text := fmt.Sprintf("[fast-agent] Received message: %q", input)
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content.NewTextContent(text, content.RoleModel)
		event.Author = ctx.Agent().Name()
		yield(event, nil)
	}
}

func slowAgentRun(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		time.Sleep(time.Duration(10+rand.Intn(50)) * time.Millisecond)
		input := lastUserText(ctx)
		text := fmt.Sprintf("[slow-agent] Received message: %q", input)
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content.NewTextContent(text, content.RoleModel)
		event.Author = ctx.Agent().Name()
		yield(event, nil)
	}
}

func main() {
	ctx := context.Background()

	fastAgent := agent.New(
		agent.WithName("fast-agent"),
		agent.WithDescription("responds quickly"),
		agent.WithRun(fastAgentRun),
	)

	slowAgent := agent.New(
		agent.WithName("slow-agent"),
		agent.WithDescription("responds slowly"),
		agent.WithRun(slowAgentRun),
	)

	parallelAgent, err := parallelagent.New(parallelagent.Config{
		Name:        "parallel-demo",
		Description: "A parallel agent that runs sub-agents concurrently",
		SubAgents:   []agent.Agent{fastAgent, slowAgent},
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "parallel-agent",
		Agent:             parallelAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("Hello from parallel demo", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		if text := event.Text(); text != "" {
			fmt.Println(text)
		}
	}
}
