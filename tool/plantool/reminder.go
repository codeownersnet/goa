package plantool

import (
	"iter"

	"github.com/codeownersnet/goa/agent"
	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/flow"
	"github.com/codeownersnet/goa/model"
	"github.com/codeownersnet/goa/session"
	"github.com/codeownersnet/goa/tool"
)

type PlanBundle struct {
	Tools    []tool.Tool
	Reminder flow.RequestProcessor
}

func NewBundle() (*PlanBundle, error) {
	tools, err := New()
	if err != nil {
		return nil, err
	}
	return &PlanBundle{
		Tools:    tools,
		Reminder: PlanReminder(),
	}, nil
}

func PlanReminder() flow.RequestProcessor {
	return func(ctx agent.InvocationContext, req *model.ModelRequest, _ *flow.Flow) iter.Seq2[*session.Event, error] {
		return func(_ func(*session.Event, error) bool) {
			state := ctx.Session().State()
			plan, err := getPlan(state)
			if err != nil || plan == nil {
				return
			}

			text := renderPlanReminder(plan)
			req.Contents = append(req.Contents, &content.Content{
				Role:  content.RoleUser,
				Parts: []content.Part{content.NewTextPart(text)},
			})
		}
	}
}
