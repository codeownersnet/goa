package testutil

import (
	"context"
	"iter"
	"sync"

	"github.com/codeownersnet/goa/model"
)

type ScriptedModel struct {
	Script func(call int, req *model.ModelRequest, stream bool) iter.Seq2[*model.ModelResponse, error]

	mu       sync.Mutex
	calls    int
	streams  []bool
	requests []*model.ModelRequest
}

func (m *ScriptedModel) Name() string { return "testutil-scripted" }

func (m *ScriptedModel) Capabilities() model.ModelCapabilities {
	return model.ModelCapabilities{}
}

func (m *ScriptedModel) GenerateContent(_ context.Context, req *model.ModelRequest, stream bool) iter.Seq2[*model.ModelResponse, error] {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.streams = append(m.streams, stream)
	m.requests = append(m.requests, req)
	m.mu.Unlock()
	return m.Script(call, req, stream)
}

func (m *ScriptedModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *ScriptedModel) Streams() []bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]bool, len(m.streams))
	copy(out, m.streams)
	return out
}

func (m *ScriptedModel) Requests() []*model.ModelRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.ModelRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

var _ model.Model = (*ScriptedModel)(nil)
