# Goa — Multi-Provider Go Agent Framework

A Go agent framework with provider-agnostic core types and first-class multi-provider LLM support via [models.dev](https://models.dev).

**Go 1.23+** · **MIT License** · **116 providers, 2000+ models**

## Why Goa?

Goa's core types — `Content`, `Schema`, `GenerateConfig`, and more — are framework-native, not tied to any single provider SDK. This means you can use **any LLM provider** without rewriting your agent code.

- **Provider-agnostic core**: `content/`, `model/`, `schema/` contain zero provider imports
- **Adapter pattern**: Each provider implements a thin adapter — swap providers without changing your agent code
- **models.dev catalog**: 116 providers and 2000+ models discoverable at runtime

## Installation

```bash
go get github.com/codeownersnet/goa
```

**Prerequisites**:
- Go 1.23+ (required for `iter.Seq2` streaming)
- API keys for your chosen providers (set as environment variables)

Goa has minimal dependencies — `gopkg.in/yaml.v3` (skill and workflow parsing), `github.com/stretchr/testify` (testing), `github.com/mattn/go-sqlite3` (persistent sessions), and `github.com/modelcontextprotocol/go-sdk/mcp` (MCP toolset).

## Quick Start

```go
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

    // Create a provider registry (loads models.dev catalog)
    reg, err := provider.NewRegistry(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Register the OpenAI-compatible adapter factory
    // This covers 88+ providers: OpenAI, OpenRouter, DeepSeek, Groq, etc.
    reg.RegisterFactory("openai_compat", &openai.Factory{})

    // Resolve a model by provider_id/model_id string
    m, err := reg.Resolve(ctx, "openrouter/deepseek/deepseek-r1")
    if err != nil {
        log.Fatal(err)
    }

    // Create an LLM-backed agent
    myAgent, err := llmagent.New(llmagent.Config{
        Name:        "assistant",
        Model:       m,
        Instruction: "You are a helpful assistant.",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create a runner to orchestrate session + agent
    r, err := runner.New(runner.Config{
        AppName:        "my-app",
        Agent:          myAgent,
        SessionService: session.InMemoryService(),
    })
    if err != nil {
        log.Fatal(err)
    }

    // Send a message and stream the response
    userMsg := content.NewTextContent("Hello!", content.RoleUser)
    for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
        if err != nil {
            log.Fatal(err)
        }
        fmt.Print(event.Text())
    }
}
```

See [`examples/hello_world/`](examples/hello_world/) for the runnable version.

## Supported Providers

Goa uses [models.dev](https://models.dev) for provider and model discovery — **116 providers, 2000+ models** out of the box.

### Provider String Format

```
provider_id/model_id
```

Examples:
- `openai/gpt-5`
- `anthropic/claude-4-opus`
- `openrouter/deepseek/deepseek-r1`


### Built-in Adapters

| Adapter | Provider Type | API Format | Providers |
|---------|--------------|-----------|-----------|
| `openai` | `openai_compat` | Chat Completions | 88+ providers (OpenAI, OpenRouter, DeepSeek, Groq, ...) |
| `anthropic` | `anthropic` | Messages API | Anthropic, Kimi, and other Anthropic-compatible providers |

### API Key Resolution

Keys are resolved in this order:
1. **Explicit override**: `provider.WithAPIKey("sk-...")` passed to `Resolve`
2. **Environment variables**: read from models.dev catalog (e.g., `OPENROUTER_API_KEY`, `ANTHROPIC_API_KEY`)
3. **Error**: if no key is found, `Resolve` returns an error

## Core Concepts

### Agent

The `agent.Agent` interface is the top-level abstraction:

```go
type Agent interface {
    Name() string
    Description() string
    Run(ctx InvocationContext) iter.Seq2[*session.Event, error]
    SubAgents() []Agent
    FindAgent(name string) Agent
    FindSubAgent(name string) Agent
}
```

`agent.InvocationContext` extends `context.Context` with: `Agent()`, `ArtifactService()`, `MemoryService()`, `Session()`, `InvocationID()`, `Branch()`, `UserContent()`, `RunConfig()`, `EndInvocation()`, `Ended()`, `WithContext(ctx) InvocationContext`.

`agent.ReadonlyContext` provides read-only access: `UserContent()`, `InvocationID()`, `AgentName()`, `State()`, `UserID()`, `AppName()`, `SessionID()`, `Branch()`.

`agent.CallbackContext` extends `ReadonlyContext` with write access: `Artifacts()`, `State()`.

Create agents with functional options:

```go
myAgent := agent.New(
    agent.WithName("greeter"),
    agent.WithDescription("A simple greeting agent"),
    agent.WithRun(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
        // custom agent logic
    }),
    agent.WithBeforeAgentCallbacks(func(ctx agent.CallbackContext) (*content.Content, error) {
        // runs before the agent
        return nil, nil
    }),
)
```

Available options: `WithName`, `WithDescription`, `WithSubAgents`, `WithRun`, `WithBeforeAgentCallbacks`, `WithAfterAgentCallbacks`.

### LLM Agent

`llmagent.Agent` is the primary concrete agent — backed by an LLM model via the `Flow` execution loop:

```go
myAgent, err := llmagent.New(llmagent.Config{
    Name:         "assistant",
    Model:        model,          // model.Model implementation
    Instruction:  "You are a helpful assistant.",
    Tools:        []tool.Tool{weatherTool},
    GenerateConfig: &model.GenerateConfig{
        Temperature: ptr(0.7),
        MaxTokens:   1024,
    },
    SubAgents:    []agent.Agent{subAgent},
})
```

`llmagent.Config` fields: `Name`, `Description`, `SubAgents`, `Model`, `Instruction`, `Tools`, `GenerateConfig`, `BeforeAgentCallbacks`, `AfterAgentCallbacks`.

Model and tool-level callbacks (`BeforeModelCallbacks`, `AfterModelCallbacks`, `OnModelErrorCallbacks`, `BeforeToolCallbacks`, `AfterToolCallbacks`, `OnToolErrorCallbacks`) are configured on `flow.Flow` directly (see [Flow & Callbacks](#flow--callbacks)).

### Model

The `model.Model` interface is the core LLM abstraction:

```go
type Model interface {
    Name() string
    GenerateContent(ctx context.Context, req *ModelRequest, stream bool) iter.Seq2[*ModelResponse, error]
    Capabilities() ModelCapabilities
}
```

`ModelCapabilities` reports what a model supports:
- `ToolCall`, `StructuredOutput`, `Reasoning`, `Attachment` — boolean flags
- `InputModalities`, `OutputModalities` — e.g., `{"text": true, "streaming": true}`
- `ContextLimit`, `OutputLimit` — token limits

Models are created via the provider registry (see [Provider Registry](#provider-registry--model-resolution)).

### Content

`content.Content` represents a message with a role and list of parts:

```go
// Create a user message
msg := content.NewTextContent("Hello!", content.RoleUser)

// Create content with multiple parts
c := content.NewContent(content.RoleUser,
    content.NewTextPart("Here is an image:"),
    content.NewInlineDataPart("image/png", imageData),
)
```

**Roles**: `RoleUser`, `RoleModel`, `RoleSystem`, `RoleTool`

**Part types**:
| Type | Constructor | Description |
|------|------------|-------------|
| Text | `NewTextPart(text)` | Plain text |
| Inline Data | `NewInlineDataPart(mime, data)` | Base64-encoded binary |
| File Data | `NewFileDataPart(uri, mime)` | File reference by URL |
| Function Call | `NewFunctionCallPart(id, name, args)` | LLM requesting tool invocation |
| Function Response | `NewFunctionResponsePart(id, name, result, isErr)` | Tool execution result |
| Thinking | `NewThinkingPart(text)` | LLM reasoning content (with optional `Signature []byte`) |
| Code Execution | — | LLM-generated code execution result (`Code`, `Output`, `Error`) |

### Schema

`schema.Schema` represents JSON Schema for structured output and tool parameters:

```go
// Object with required fields
weatherSchema := schema.Object(map[string]*schema.Schema{
    "location":  schema.String(),
    "unit":      schema.String(),  // optional
}, "location")

// Nested types
profileSchema := schema.Object(map[string]*schema.Schema{
    "name": schema.String(),
    "age":  schema.Int(),
    "tags": schema.Array(schema.String()),
}, "name", "age")
```

Helper constructors: `schema.Object()`, `schema.String()`, `schema.Int()`, `schema.Float()`, `schema.Bool()`, `schema.Array()`.

### Tool

The `tool.Tool` interface defines how agents interact with external systems:

```go
type Tool interface {
    Name() string
    Description() string
    Process(ctx context.Context, args map[string]any) (map[string]any, error)
}
```

`tool.Toolset` groups tools: `Tools() []tool.Tool`.

`tool.Declarer` provides tool declarations for model registration: `Declaration() *content.ToolDeclaration`. Tools implementing `Declarer` are automatically declared to the model.

`tool.Context` extends `agent.CallbackContext` with: `FunctionCallID() string`, `Actions() *EventActions`. It provides access to the invocation context and allows tools to signal state changes, agent transfers, or escalation.

`tool.EventActions`: `StateDelta map[string]any`, `TransferToAgent string`, `Escalate bool`, `SkipSummarization bool`.

### Function Tools (Generics)

`functiontool.New[TArgs, TResults]` creates type-safe tools with automatic schema inference from Go struct tags:

```go
type WeatherArgs struct {
    Location string `json:"location"`
    Unit     string `json:"unit,omitempty"`
}

type WeatherResult struct {
    Temp      int    `json:"temp"`
    Condition string `json:"condition"`
}

weatherTool, err := functiontool.New(functiontool.Config{
    Name:        "get_weather",
    Description: "Get the current weather for a location",
}, func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
    return WeatherResult{Temp: 72, Condition: "sunny"}, nil
})
```

Schema is inferred from struct field types and `json` tags. Override with `Config.InputSchema`.

`functiontool.Config` fields: `Name`, `Description`, `InputSchema`, `IsLongRunning`.

`functiontool.Declaration()` returns a `content.ToolDeclaration` for registration with the model.

### MCP Toolset

Connect to MCP (Model Context Protocol) servers and use their tools, resources, and prompts:

```go
ts, err := mcptoolset.New(ctx,
    mcptoolset.WithName("filesystem"),
    mcptoolset.WithCommand(exec.Command("npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")),
)
if err != nil {
    log.Fatal(err)
}
defer ts.Close()

// All discovered tools implement tool.Tool and tool.Declarer
mcpTools := ts.Tools()

// Discover resources and prompts
resources := ts.Resources()
prompts := ts.Prompts()
```

**Options**: `WithName`, `WithCommand` (stdio transport), `WithURL` (Streamable HTTP transport), `WithHeaders`, `WithEnv`, `WithConnectTimeout`, `WithToolTimeout`.

Tool names are prefixed as `{server}_{tool}` (e.g., `filesystem_read_file`) to prevent cross-server collisions.

### Plan Tool

`tool/plantool/` provides structured plan management for agents:

```go
planTool := plantool.New()
// Operations: create, get, show, update, reminder
```

### Tool Registry

`tool/registry/` maps tool names to factories for use in workflows and CLI:

```go
reg := toolregistry.DefaultBuiltinRegistry(
    toolregistry.WithBuiltinAllowedPaths([]string{"/workspace/**"}),
)

t, err := reg.Lookup("bash")
```

Built-in tools: `bash`, `read_file`, `write_file`, `edit_file`, `glob`, `grep`, `list_dir`, `exit_loop`, `diff`.

## Streaming

Enable SSE streaming via `RunConfig`:

```go
for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{
    StreamingMode: agent.StreamingModeSSE,
}) {
    if err != nil {
        log.Fatal(err)
    }
    if event.Partial {
        fmt.Print(event.Text())   // incremental text chunk
    } else {
        fmt.Println("\n--- Complete ---")
    }
}
```

- `agent.StreamingModeSSE` — enable streaming partial responses
- `agent.StreamingModeNone` — wait for complete response (default)
- `event.Partial` — `true` for incremental chunks, `false` for the final event
- `event.IsFinalResponse()` — returns `true` when the response is complete

`flow.StreamingResponseAggregator` accumulates partial SSE chunks into a single complete response (text concatenation, function call accumulation).

## Provider Registry & Model Resolution

The provider registry resolves model strings (like `openai/gpt-4o`) to concrete `model.Model` instances:

```go
// Create registry (loads models.dev catalog)
reg, err := provider.NewRegistry(ctx)

// Register adapter factories
reg.RegisterFactory("openai_compat", &openai.Factory{})
reg.RegisterFactory("anthropic", &anthropic.Factory{})

// Resolve models — the registry infers the adapter type from models.dev
openaiModel, err := reg.Resolve(ctx, "openai/gpt-4o")
anthropicModel, err := reg.Resolve(ctx, "anthropic/claude-4-opus")

// Override API key explicitly
model, err := reg.Resolve(ctx, "openai/gpt-4o", provider.WithAPIKey("sk-..."))
```

**Registry options**:
- `provider.NewRegistry(ctx)` — fetches catalog from models.dev API
- `provider.WithRegistryOffline()` — use embedded catalog fallback (no network)
- `provider.WithCustomProvider(id, info)` — add custom providers not in models.dev

**AdapterFactory interface**:
```go
type AdapterFactory interface {
    NewModel(ctx context.Context, entry *ModelEntry, opts ...Option) (model.Model, error)
}
```

Provider type inference (`modelsdev/infer.go`) maps provider IDs from models.dev to adapter types: `openai_compat`, `anthropic`.

## Skills System

Goa supports [Agent Skills](https://agentskills.io) — a standardized way to give agents specialized knowledge and workflows. A skill is a directory with a `SKILL.md` file:

```
my-skill/
├── SKILL.md          # Required: metadata + instructions
├── scripts/          # Optional: executable code
├── references/       # Optional: documentation
└── assets/           # Optional: templates, resources
```

### SKILL.md Format

```yaml
---
name: my-skill
description: What this skill does
license: MIT
compatibility: goa/v1
allowed-tools: bash git
metadata:
  version: "1.0"
---

Full skill instructions in markdown...
```

YAML frontmatter fields:
| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Lowercase + hyphens, max 64 chars, must match directory name |
| `description` | Yes | Max 1024 chars |
| `license` | No | License identifier |
| `compatibility` | No | Max 500 chars |
| `allowed-tools` | No | Space-separated tool names |
| `metadata` | No | Arbitrary key-value pairs |

### Progressive Disclosure

Skills use three levels of loading to minimize token usage:
1. **Discovery** — metadata only (~100 tokens per skill)
2. **Activation** — full skill body loaded on demand
3. **Resources** — scripts, references, and assets loaded as needed

### Using Skills

```go
reg := skill.NewRegistry(
    skill.WithSkillDirs(".agents/skills/", ".goa/skills/"),
    skill.WithRunScripts(true),
)

// Scan directories for skills
reg.Discover(ctx)

// List discovered skills (metadata only)
skills := reg.List()

// Activate a skill (loads full body)
s, err := reg.Activate(ctx, "my-skill")

// Read a resource file
content, err := reg.ReadResource(ctx, "my-skill", "references/guide.md")

// Run a script
output, err := reg.RunScript(ctx, "my-skill", "scripts/setup.sh")

// Generate prompt for injection into agent instructions
prompt := s.ToPromptXML()
```

**Default skill directories**: `.agents/skills/`, `.goa/skills/` (project-level), `~/.agents/skills/`, `~/.goa/skills/` (user-level).

### Built-in Skill Tools

These tools let agents interact with skills at runtime:
- `skilltool.ActivateTool` — activate a skill by name
- `skilltool.ResourceTool` — read a skill resource file
- `skilltool.ScriptTool` — run a skill script

## Flow & Callbacks

`flow.Flow` is the core execution loop that drives LLM agents:

```
preprocess → callLLM → postprocess → handleTools → loop
```

Each step:
1. **Preprocess** — builds conversation history from session events, collects tool declarations
2. **CallLLM** — sends request to the model (with streaming if supported)
3. **Postprocess** — runs response processors
4. **HandleTools** — executes function calls from the model response (concurrently by default)
5. **Loop** — repeats until a final response (no more function calls) or `MaxIterations` reached

`flow.Flow` fields: `Model`, `Tools`, `Instruction`, `GenerateConfig`, `MaxIterations` (default 25), `ToolTimeout` (default 120s), `RequestProcessors`, `ResponseProcessors`, `BeforeModelCallbacks`, `AfterModelCallbacks`, `OnModelErrorCallbacks`, `BeforeToolCallbacks`, `AfterToolCallbacks`, `OnToolErrorCallbacks`.

Doom loop detection: if the same tool call signature repeats 3+ consecutive times, a warning is logged. After 2 doom loop detections, the flow terminates with an error.

### Callbacks

Hooks into the execution loop for observability and control:

```go
flow := &flow.Flow{
    Model:       model,
    Tools:       []tool.Tool{weatherTool},
    Instruction: "You are a weather assistant.",

    BeforeModelCallbacks: []flow.BeforeModelCallback{
        func(ctx agent.InvocationContext, req *model.ModelRequest) (*model.ModelResponse, error) {
            return nil, nil
        },
    },

    AfterToolCallbacks: []flow.AfterToolCallback{
        func(ctx tool.Context, t tool.Tool, args map[string]any, result map[string]any, err error) (map[string]any, error) {
            return result, err
        },
    },

    OnModelErrorCallbacks: []flow.OnModelErrorCallback{
        func(ctx agent.InvocationContext, req *model.ModelRequest, err error) (*model.ModelResponse, error) {
            return nil, err
        },
    },
}
```

**Callback types**:
| Callback | Signature | When |
|----------|-----------|------|
| `BeforeModelCallback` | `(ctx, req) → (resp, error)` | Before LLM call |
| `AfterModelCallback` | `(ctx, resp, err) → (resp, error)` | After LLM call |
| `OnModelErrorCallback` | `(ctx, req, err) → (resp, error)` | On LLM error |
| `BeforeToolCallback` | `(ctx, tool, args) → (args, error)` | Before tool execution |
| `AfterToolCallback` | `(ctx, tool, args, result, err) → (result, error)` | After tool execution |
| `OnToolErrorCallback` | `(ctx, tool, args, err) → (result, error)` | On tool error |

**Request/Response processors** for advanced customization:
- `RequestProcessors` — `func(ctx, req, flow) iter.Seq2[*Event, error]` — can emit events and modify requests
- `ResponseProcessors` — `func(ctx, req, resp) error` — can modify responses

## Session & State Management

Sessions persist conversation history and state across agent runs:

```go
svc := session.InMemoryService()

resp, err := svc.Create(ctx, &session.CreateRequest{
    AppName:   "my-app",
    UserID:    "user1",
    SessionID: "session1",
    State:     map[string]any{},
})

// Read state
val, err := resp.Session.State().Get("key")

// Write state
err = resp.Session.State().Set("key", "value")

// Iterate all state
for key, val := range resp.Session.State().All() {
    fmt.Println(key, val)
}

// Append an event
err = svc.AppendEvent(ctx, resp.Session, event)
```

**`session.Session` interface**: `ID()`, `AppName()`, `UserID()`, `Events()`, `State()`, `LastUpdateTime()`

**`session.Event`** struct:
| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | Unique event ID |
| `Timestamp` | `time.Time` | Event time |
| `InvocationID` | `string` | Groups events from one Run call |
| `Branch` | `string` | Branch for parallel execution |
| `Author` | `string` | Agent name that produced the event |
| `Actions` | `EventActions` | Side effects (state delta, transfer, escalation) |
| `Partial` | `bool` | `true` for streaming chunks |
| `ModelResponse` | `*content.Content` | LLM response content |
| `Usage` | `*Usage` | Token usage |
| `FinishReason` | `FinishReason` | Why the model stopped |

**`session.EventActions`**: `StateDelta`, `ArtifactDelta`, `TransferToAgent`, `Escalate`, `SkipSummarization`

**Helper functions**:
- `session.NewEvent(invocationID)` — create a new event
- `event.Text()` — extract text from model response
- `event.IsFinalResponse()` — check if event is a final (non-partial, non-tool) response

### SQLite Session Backend

For persistent sessions, use the SQLite backend:

```go
import "github.com/codeownersnet/goa/session/sqlite"

svc, err := sqlite.NewService(ctx, sqlite.Config{
    Path: "sessions.db",  // or ":memory:" for in-memory SQLite
})
if err != nil {
    log.Fatal(err)
}
defer svc.Close()

r, err := runner.New(runner.Config{
    AppName:        "my-app",
    Agent:          myAgent,
    SessionService: svc,
})
```

`sqlite.Config` fields:
- `Path` — database file path (defaults to `:memory:`);

The SQLite backend supports all `session.Service` operations with full state persistence and event history. WAL mode is enabled for concurrent read/write performance.


## Runner

The `runner.Runner` ties agents, sessions, and services together:

```go
r, err := runner.New(runner.Config{
    AppName:        "my-app",
    Agent:          myAgent,
    SessionService: session.InMemoryService(),
    ArtifactService: artifact.InMemoryService(),   // optional
    MemoryService:  memory.InMemoryService(),       // optional
    AutoCreateSession: true,                        // auto-create sessions
})

// Run the agent
for event, err := range r.Run(ctx, "user1", "session1", userMsg, agent.RunConfig{}) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(event.Text())
}
```

`runner.Run(ctx, userID, sessionID, msg, runConfig)`:
1. Finds or creates the session
2. Appends the user message to session history
3. Runs the agent
4. Persists agent events to the session
5. Returns `iter.Seq2[*session.Event, error]`

## Memory Service

Memory provides cross-session retrieval for agents:

```go
svc := memory.InMemoryService()

// Index session content for future retrieval
err := svc.AddSessionToMemory(ctx, session)

// Search across indexed sessions
resp, err := svc.SearchMemory(ctx, memory.SearchRequest{
    AppName: "my-app",
    UserID:  "user1",
    Query:   "previous conversation about weather",
})
```

The built-in `memory.InMemoryService()` uses simple substring matching. Implement the `memory.Service` interface for vector search or other backends.

## Artifact Service

Artifacts provide versioned file storage per app/user/session:

```go
svc := artifact.InMemoryService()

// Save a file (creates a new version)
resp, err := svc.Save(ctx, &artifact.SaveRequest{
    AppName: "my-app", UserID: "user1", SessionID: "session1",
    FileName: "report.md", Data: []byte("content"),
})

// Load the latest version
resp, err := svc.Load(ctx, &artifact.LoadRequest{
    AppName: "my-app", UserID: "user1", SessionID: "session1",
    FileName: "report.md",
})

// List all artifacts
resp, err := svc.List(ctx, &artifact.ListRequest{
    AppName: "my-app", UserID: "user1", SessionID: "session1",
})

// List versions
resp, err := svc.Versions(ctx, &artifact.VersionsRequest{
    AppName: "my-app", UserID: "user1", SessionID: "session1",
    FileName: "report.md",
})

// Load a specific version
resp, err := svc.GetArtifactVersion(ctx, &artifact.GetArtifactVersionRequest{
    AppName: "my-app", UserID: "user1", SessionID: "session1",
    FileName: "report.md", Version: 1,
})

// Delete an artifact
err = svc.Delete(ctx, &artifact.DeleteRequest{
    AppName: "my-app", UserID: "user1", SessionID: "session1",
    FileName: "report.md",
})
```

`artifact.Service` interface composes: `Saver`, `Loader`, `Deleter`, `Lister`, `Versioner`, `ArtifactVersionGetter`.

## Workflow YAML & CLI

Define agents declaratively in YAML files and run them via the `goafl` CLI:

```yaml
name: code-review
description: Review code and write a report
type: sequential
mcp_servers:
  filesystem:
    command: "npx -y @modelcontextprotocol/server-filesystem /workspace"
steps:
  - name: find-todos
    model: openai/gpt-4o
    instruction: Find all TODO comments
    tools: [mcp:filesystem, grep]
  - name: review
    type: loop
    max_iterations: 3
    exit_when:
      state:
        review_complete: "true"
      timeout: 10m
    steps:
      - name: reviewer
        model: openai/gpt-4o
        instruction: Review each TODO
        tools: [read_file, exit_loop]
```

### Workflow YAML Schema

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Workflow name |
| `description` | Yes | Workflow description |
| `type` | No | `sequential` (default) or `loop` |
| `max_iterations` | No | Max loop iterations (default 10 for loop) |
| `exit_when` | No | Exit condition: `state` key-value matches and/or `timeout` duration |
| `mcp_servers` | No | Map of MCP server configs (see below) |
| `steps` | Yes | List of step definitions |

### Step Types

| Type | Fields | Description |
|------|--------|-------------|
| `llm` (default) | `model`, `instruction`, `tools`, `skills`, `generate_config` | LLM-backed step |
| `sequential` | `steps` | Runs sub-steps in order |
| `loop` | `steps`, `max_iterations`, `exit_when` | Repeats sub-steps until exit condition |

### MCP Server Configuration

Declare MCP servers at the workflow level (shared across all steps):

```yaml
mcp_servers:
  filesystem:
    command: "npx -y @modelcontextprotocol/server-filesystem /tmp"
  remote-api:
    url: "https://mcp.example.com/sse"
    headers:
      Authorization: "Bearer ${API_TOKEN}"
    connect_timeout: 10s
    tool_timeout: 120s
```

Reference MCP tools in step `tools` lists:
- **Explicit**: `filesystem_read_file` — a specific discovered tool
- **Wildcard**: `mcp:filesystem` — all tools from that server

### `goafl` CLI

```bash
# Run a workflow
goafl run <workflow-name> [prompt]

# List available workflows
goafl list

# Validate a workflow file
goafl validate <workflow-name>
```

Workflow search paths: `.goa/workflows/`, `~/.goa/workflows/`.

### Workflow API

```go
wf, err := workflow.Load(ctx, "path/to/workflow.yaml",
    workflow.WithProviderRegistry(provReg),
    workflow.WithToolRegistry(toolReg),
    workflow.WithSkillRegistry(skillReg),
)
if err != nil { log.Fatal(err) }
defer wf.Close()  // closes MCP toolset connections

agent := wf.Agent()
exitCond := wf.ExitCondition()
```

`workflow.LoadFromBytes(ctx, data, opts...)` loads from raw YAML. `workflow.ValidateFile(ctx, path)` validates structure without connecting to MCP servers.

## Configuration Options

### GenerateConfig

```go
config := &model.GenerateConfig{
    Temperature:      ptr(0.7),
    TopP:            ptr(0.9),
    MaxTokens:       2048,
    StopSequences:   []string{"\n\n"},
    ToolChoice:      model.ToolChoice{Mode: model.ToolChoiceAuto},
    ResponseSchema:  schema.Object(map[string]*schema.Schema{
        "answer": schema.String(),
    }, "answer"),
    ResponseMIMEType: "application/json",
    SystemInstruction: "You are a concise assistant.",
    ThinkingConfig: &model.ThinkingConfig{
        Enabled:         true,
        BudgetTokens:    8192,
        IncludeThoughts: true,
    },
    SafetySettings: []model.SafetySetting{
        {Category: "HARM_CATEGORY", Threshold: "BLOCK_LOW_AND_ABOVE"},
    },
    ProviderExtra: map[string]any{
        "custom_param": "value",
    },
}
```

### ToolChoice Modes

| Mode | Constant | Description |
|------|----------|-------------|
| Auto | `ToolChoiceAuto` | Model decides whether to call tools |
| None | `ToolChoiceNone` | Model will not call tools |
| Required | `ToolChoiceRequired` | Model must call a tool |
| Specific | `ToolChoiceSpecific` | Model must call a specific tool (set `Name`) |

### ThinkingConfig

```go
&model.ThinkingConfig{
    Enabled:         true,    // Enable extended thinking
    BudgetTokens:    8192,    // Token budget for thinking
    IncludeThoughts: true,    // Include thoughts in response
}
```

### Usage

```go
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
    CacheReadTokens  int
    CacheWriteTokens int
}
```

### FinishReason

| Constant | Meaning |
|----------|---------|
| `FinishReasonStop` | Model stopped naturally |
| `FinishReasonMaxTokens` | Hit max token limit |
| `FinishReasonToolCall` | Model requested a tool call |
| `FinishReasonSafety` | Stopped by safety filter |
| `FinishReasonRecursion` | Stopped due to recursion limit |
| `FinishReasonCancelled` | Cancelled by caller |
| `FinishReasonUnknown` | Unknown reason |

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                      Runner                          │
│           (orchestration, session management)         │
├─────────────────────────────────────────────────────┤
│                      Agent                           │
│  (LLMAgent | Sequential | Loop | Parallel | custom)  │
├─────────────────────────────────────────────────────┤
│                      Flow                            │
│   preprocess → callLLM → postprocess → handleTools   │
├─────────────────────────────────────────────────────┤
│                     Model                            │
│       (OpenAI | Anthropic adapter)                   │
└─────────────────────────────────────────────────────┘

Service Layer:  Session  |  Memory  |  Artifact
               (all interface-based, pluggable)

Core Types:    content/  |  model/  |  schema/
               (provider-agnostic, zero SDK imports)

Providers:     provider/openai/  |  provider/anthropic/
               (thin adapters, import SDK only here)

Declarative:   workflow/  (YAML → agent tree + MCP servers)
               skill/     (SKILL.md → agent knowledge)

Tools:         tool/bash/  |  tool/editfile/  |  tool/grep/  ...
               tool/mcptoolset/  (MCP protocol client)
               tool/plantool/    (structured plan management)
               tool/registry/    (name → factory mapping)

Utilities:     modelsdev/  |  parentmap/  |  telemetry/
```

**Key design principle**: All core types are provider-agnostic. Provider adapters only import their own SDKs.

## Examples

| Example | What It Shows | Key Packages |
|---------|--------------|--------------|
| [`hello_world`](examples/hello_world/) | Basic agent setup and usage | `provider`, `llmagent`, `runner` |
| [`multi_provider`](examples/multi_provider/) | Using multiple LLM providers | `openai`, `anthropic` factories |
| [`streaming`](examples/streaming/) | SSE streaming with partial events | `StreamingModeSSE` |
| [`tool_usage`](examples/tool_usage/) | Type-safe function tools with generics | `functiontool`, generics |
| [`skills`](examples/skills/) | Skill discovery and activation | `skill.Registry` |
| [`coding_agent`](examples/coding_agent/) | Full coding agent with tools, skills, and artifacts | All packages |
| [`coding_agent_with_plan`](examples/coding_agent_with_plan/) | Coding agent with plan management | `plantool` |

Run an example:
```bash
export OPENROUTER_API_KEY=sk-...
go run ./examples/hello_world

# Override the default model
go run ./examples/hello_world openai/gpt-4o
```

## Telemetry

The `telemetry/` package provides OpenTelemetry scaffolding (currently a stub):

```go
providers := telemetry.New()  // placeholder — returns no-op providers
```

Defines: `Providers` (TracerProvider, LoggerProvider), `StartInvokeAgentSpan`, `WrapYield`, `TraceAgentResult`.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Run `make lint` and `make test` before submitting
4. Add tests for new features
5. Follow existing code patterns:
   - Functional options for configuration
   - Interface-driven design
   - Provider adapters implement `model.Model` + `provider.AdapterFactory`
6. Open a pull request

## License

MIT
