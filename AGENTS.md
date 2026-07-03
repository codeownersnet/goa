# AGENTS.md — Project Conventions for Goa

## Project Overview

- **Module**: `github.com/codeownersnet/goa`
- **Language**: Go 1.23+ (required for `iter.Seq2`)
- **Purpose**: Multi-provider Go agent framework
- **License**: MIT

## Build & Test Commands

```bash
make build          # go build ./...
make test           # go test ./... -race -count=1
make lint           # golangci-lint run ./...
make vet            # go vet ./...
make coverage       # go test ./... -race -coverprofile=coverage.out && go tool cover -func=coverage.out
make generate       # go generate ./modelsdev/... (embeds models.dev catalog)
```

The full CI-style script: `./scripts/test.sh` (build + vet + test with race detector).

## Lint Configuration

Enabled linters (`.golangci.yml`):
- `errcheck` — check for unchecked errors (test files exempt)
- `govet` — go vet
- `revive` — `exported` (warning), `unused-parameter` (warning)
- `gofmt` — formatting
- `goimports` — import ordering
- `misspell` — spelling
- `unused` — unused code

5-minute timeout.

## Code Style & Conventions

- **Functional options pattern**: `type Option func(*config)` — used for all configuration (agent, provider, skill, telemetry, modelsdev)
- **Interface-driven design**: define interfaces in core packages, implement in sub-packages
- **Error wrapping**: always use `fmt.Errorf("context: %w", err)` with `%w`
- **iter.Seq2 for streaming**: all streaming APIs return `iter.Seq2[T, error]` — never channels
- **Constructor pattern**: `New(Config)` or `New(opts ...Option)` — never export zero-value structs
- **Interface compliance**: `var _ Interface = (*impl)(nil)` at end of implementation files
- **JSON tags**: always use `` `json:"field_name"` `` on exported struct fields; use `omitempty` where appropriate
- **YAML tags**: used for skill frontmatter parsing via `yaml:"field_name"`
- **No comments** unless explicitly requested

## Package Structure

```
agent/              — Agent interface, InvocationContext, RunConfig, base agent, callbacks
agent/llmagent/     — LLM-powered agent (uses Flow internally, handles agent transfer + escalation)
agent/sequentialagent/ — Sequential workflow agent: runs sub-agents in order
agent/loopagent/    — Loop workflow agent: repeats sub-agents until escalation or max iterations
agent/parallelagent/ — Parallel workflow agent: runs sub-agents concurrently
artifact/           — Artifact service: versioned file storage per session
cmd/embed-catalog/  — Tool to embed models.dev catalog (placeholder)
cmd/goafl/          — CLI: run, list, validate workflow YAML files
content/            — Provider-agnostic content types: Content, Part, FunctionCall, ToolDeclaration
flow/               — Execution loop: preprocess → callLLM → postprocess → handleTools → loop
memory/             — Memory service: cross-session retrieval
model/              — Model interface, GenerateConfig, Capabilities, Usage, FinishReason
modelsdev/          — models.dev catalog loading + provider type inference
parentmap/          — Agent tree parent mapping + context storage
provider/           — Registry, AdapterFactory, ProviderInfo, ModelEntry, API key resolution
provider/openai/    — OpenAI Chat Completions adapter (88+ providers)
provider/anthropic/ — Anthropic Messages API adapter
runner/             — Runner: ties agent + session + services together
schema/             — JSON Schema types + constructor helpers
session/            — Session service, State, Event, EventActions, in-memory impl
session/sqlite/      — SQLite session backend (persistent storage)
skill/              — Skill loading, registry, validation, prompt generation
skill/skilltool/    — Built-in tools: ActivateTool, ResourceTool, ScriptTool
telemetry/          — OpenTelemetry stub (placeholder)
tool/               — Tool interface, Toolset, Context, Declarer, EventActions
tool/agenttool/     — Agent-as-Tool: wraps an Agent as a Tool
tool/bash/          — Bash: runs a single shell command, configurable allow/deny lists
tool/difftool/      — Diff: previews unified diff of a file against proposed content
tool/editfile/      — EditFile: applies string replacements to a file (9 fuzzy match strategies)
tool/exitlooptool/  — ExitLoopTool: signals LoopAgent to break via EventActions.Escalate
tool/functiontool/  — Generic type-safe tool creation with schema inference
tool/git/           — Git: clone, pull, push, add, branch, commit, stash, status, diff, log, checkout
tool/glob/          — Glob: finds files by glob pattern in a directory
tool/grep/          — Grep: searches file contents by regex in a directory
tool/internal/pathguard/ — Internal: path allowlist enforcement for file tools
tool/listdir/       — ListDir: lists directory contents with file metadata
tool/mcptoolset/    — MCP toolset: connects to MCP servers, discovers tools/resources/prompts
tool/plantool/      — Plan: create, get, show, update, reminder operations
tool/readfile/      — ReadFile: reads file contents with offset/limit support
tool/registry/      — Tool registry: name→factory mapping, builtin registry
tool/truncate/      — Truncate: output truncation with disk spillover
tool/writefile/     — WriteFile: creates or overwrites a file
workflow/           — Workflow YAML: parse, validate, resolve to agent tree + MCP servers
```

## Core Type Organization

**Provider-agnostic packages** (never import provider SDKs):
`content/`, `model/`, `schema/`, `agent/`, `flow/`, `session/`, `memory/`, `artifact/`, `tool/`

**Provider packages** (may import SDKs):
``provider/openai/`, `provider/anthropic/`

**Bridge packages**:
- `provider/` — registry that connects catalog to model instances
- `modelsdev/` — maps models.dev catalog to `model.ModelCapabilities`

**Rule**: Core packages import each other freely. Provider packages import core + SDK.

## Adding a New Provider

1. Create `provider/<name>/` with:
   - `adapter.go` — implement `model.Model` interface (`Name()`, `GenerateContent()`, `Capabilities()`)
   - `convert.go` — conversion functions: Goa types ↔ provider API types
   - `factory.go` — implement `provider.AdapterFactory` interface (`NewModel(ctx, *ModelEntry, ...Option) (model.Model, error)`)
   - `stream.go` — SSE scanner if the provider supports streaming
2. Add provider ID → `ProviderType` mapping in `modelsdev/infer.go`
3. Register with the registry: `reg.RegisterFactory("<provider_type>", &Factory{})`
4. API key resolution: add env var names in models.dev catalog, or use `provider.WithCustomProvider()` / `provider.WithAPIKey()`
5. Follow `provider/openai/` as the reference implementation (HTTP client, SSE streaming, tool call accumulator)

## Adding a New Tool

**Simple tool**: Implement `tool.Tool` interface:
```go
type myTool struct{}
func (t *myTool) Name() string { return "my_tool" }
func (t *myTool) Description() string { return "Does something" }
func (t *myTool) Process(ctx context.Context, args map[string]any) (map[string]any, error) { ... }
```

**Type-safe tool**: Use generics:
```go
tool, err := functiontool.New(functiontool.Config{
    Name:        "my_tool",
    Description: "Does something",
}, handler)
```

Schema is auto-inferred from struct field types and `json` tags. Override with `Config.InputSchema`.

Register with agent: `llmagent.Config{Tools: []tool.Tool{myTool}}`.

For skill-integrated tools, add to `skill/skilltool/`.

## Testing Patterns

- **Test command**: `go test ./... -race -count=1` (or `make test`)
- **Assertions**: use `github.com/stretchr/testify` (`assert`, `require`)
- **Mock services**: use built-in in-memory implementations:
  - `session.InMemoryService()`
  - `sqlite.NewService()` for persistent session tests
  - `memory.InMemoryService()`
  - `artifact.InMemoryService()`
- **Offline provider tests**: use `provider.NewRegistry(ctx, provider.WithRegistryOffline())` to avoid network calls
- **Test utility**: `testutil/` exists but is currently empty — reserved for shared test helpers

## Import Conventions

- Internal imports use full module path: `github.com/codeownersnet/goa/<package>`
- No third-party imports in core packages (exception: `gopkg.in/yaml.v3` in `skill/`, `workflow/`, and `modelsdev/`, `github.com/mattn/go-sqlite3` in `session/sqlite/`, `github.com/modelcontextprotocol/go-sdk/mcp` in `tool/mcptoolset/`)
- Test dependency: `github.com/stretchr/testify` (`assert`, `require`)
- Provider packages may import HTTP clients and SDKs as needed

## Known Limitations

- **MCP toolset**: `tool/mcptoolset/` — supports tools, resources, and prompts; uses `github.com/modelcontextprotocol/go-sdk/mcp`; stdio and Streamable HTTP transports; no OAuth yet
- **Workflow YAML**: `parallel` agent type is not supported in YAML; `BeforeAgentCallback`/`AfterAgentCallbacks` and `RequestProcessors` not configurable from YAML
- **Telemetry**: `telemetry/` is a stub with no-op implementations
- **Memory search**: `memory.InMemoryService()` uses substring matching only (no vector search)
- **Embedded catalog fallback**: `cmd/embed-catalog/` is empty — offline mode (`WithRegistryOffline()`) will fail if catalog hasn't been previously fetched
- **Provider type inference**: `modelsdev/infer.go` maps only `anthropic` and `kimi-for-coding` to `ProviderTypeAnthropic`; all other providers default to `ProviderTypeOpenAI`

## SKILL.md Format Reference

```yaml
---
name: my-skill          # Required, lowercase + hyphens, max 64 chars, must match directory name
description: What it does  # Required, max 1024 chars
license: MIT            # Optional
compatibility: goa/v1   # Optional, max 500 chars
allowed-tools: bash git  # Optional, space-separated
metadata:               # Optional, arbitrary key-value
  version: "1.0"
---

Full skill instructions in markdown...
```

- Skill name must match directory name (validated by `skill.ValidateNameMatchesDir()`)
- `scripts/` files must be executable (mode `0o111`)
- Path traversal protection: no `..` in relative paths for resources/scripts
- Validation: `skill.Validate()`, `skill.ValidateName()`
