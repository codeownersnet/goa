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
	"github.com/codeownersnet/goa/runner"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
	"github.com/codeownersnet/goa/tool/functiontool"
)

type WeatherArgs struct {
	Location string `json:"location"`
}

type WeatherResult struct {
	Temp      int    `json:"temp"`
	Condition string `json:"condition"`
}

func main() {
	ctx := context.Background()

	reg, err := provider.NewRegistry(ctx)
	if err != nil {
		log.Fatal(err)
	}
	reg.RegisterFactory("anthropic", &anthropic.Factory{})

	m, err := reg.Resolve(ctx, "anthropic/claude-4-opus")
	if err != nil {
		log.Fatal(err)
	}

	weatherTool, err := functiontool.New(functiontool.Config{
		Name:        "get_weather",
		Description: "Get the current weather for a location",
	}, func(_ context.Context, _ WeatherArgs) (WeatherResult, error) {
		return WeatherResult{Temp: 72, Condition: "sunny"}, nil
	})
	if err != nil {
		log.Fatal(err)
	}

	myAgent, err := llmagent.New(llmagent.Config{
		Name:        "weather-agent",
		Model:       m,
		Instruction: "You help users check the weather. Use the get_weather tool when asked about weather.",
		Tools:       []tool.Tool{weatherTool},
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "tool-usage",
		Agent:          myAgent,
		SessionService: session.InMemoryService(),
	})
	if err != nil {
		log.Fatal(err)
	}

	userMsg := content.NewTextContent("What's the weather in San Francisco?", content.RoleUser)
	for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(event.Text())
	}
}
