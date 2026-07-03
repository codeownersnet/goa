package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"strings"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/agent/loopagent"
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

func echoRun(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		input := lastUserText(ctx)
		iterNum := getIteration(ctx.Branch())
		text := fmt.Sprintf("[echo] Iteration %d received: %q", iterNum, input)
		event := session.NewEvent(ctx.InvocationID())
		event.ModelResponse = content.NewTextContent(text, content.RoleModel)
		event.Author = ctx.Agent().Name()
		yield(event, nil)
	}
}

func getIteration(branch string) int {
	var iter int
	_, _ = fmt.Sscanf(branch, "loop-demo/iter%d/", &iter)
	return iter
}

func main() {
	ctx := context.Background()

	echoAgent := agent.New(
		agent.WithName("echo"),
		agent.WithDescription("echoes input with iteration info"),
		agent.WithRun(echoRun),
	)

	loopAgent, err := loopagent.New(loopagent.Config{
		Name:          "loop-demo",
		Description:   "A loop agent that repeats echo up to 3 times",
		SubAgents:     []agent.Agent{echoAgent},
		MaxIterations: 3,
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "loop-agent",
		Agent:             loopAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("Hello loop", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		if text := event.Text(); text != "" {
			fmt.Println(text)
		}
	}
}
