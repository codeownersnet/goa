# Host-owned execution and durable recovery

Goa supplies execution mechanisms and extension contracts. The host application
owns authentication, trusted tool registration, credentials, endpoint policy,
durable persistence, operation reconciliation, and protected state publication.
Goa does not promise exactly-once remote execution.

## Supported host-managed path

A host can use Goa without the convenience runner:

1. Implement `session.Service` or provide an authorized session implementation
   that loads only the history the current caller may see.
2. Construct an `agent.InvocationContext` with the host-selected agent, session,
   invocation identity, and `agent.RunConfig`.
3. Run the agent with `agent.Run(ctx)` and consume `iter.Seq2[*session.Event, error]`.
4. Persist non-partial events before starting the next step that depends on them.
5. Treat partial events as display-only deltas. Never persist them as completed
   answers.
6. Persist stream, output, and prompt limits in host state using `flow.RunBudget`.
   Pass the same budget pointer to resumed or transferred invocations.

The convenience runner uses the same `Flow` implementation. It is a convenience
layer for lookup, session creation, append, and user-event insertion, not a second
execution engine.

## Event commit contract

- Each yielded non-partial event must be committed before the host performs work
  that depends on it.
- A failed mandatory append stops execution. Observers may follow a different,
  explicitly documented failure policy, but they must not turn a failed mandatory
  write into success.
- The terminal response is persisted exactly once. Partial deltas are not stored.
- An interrupted stream must surface an error and must not be stored as an ordinary
  completed answer.

## Operations and reconciliation

- Provider tool-call IDs are not host operation identities. Use an opaque host
  operation reference for reconciliation.
- Identical tool arguments can represent separate legitimate operations. Goa does
  not deduplicate tool calls.
- If a tool reports `tool.NewUncertainError`, the run stops and no later tool is
  scheduled. The host decides whether to retry, reconcile, or fail.
- If a tool reports `tool.NewDeniedError`, the run stops and generic tool-error
  callbacks cannot convert the denial into a successful result.
- Keep unresolved-operation state outside prompt history. Trimming context must not
  erase the fact that an operation is unresolved.
- Do not finalize success while a required operation remains unresolved.
- Cancellation or timeout does not imply that an external side effect was rolled
  back.

## Budgets and replay

`flow.RunLimits.Budget` is a pointer so a host can carry consumed model turns,
tool calls, and output bytes across restarts or agent transfers. A resumed
invocation must not silently receive a fresh budget.

Replay records, authorized history, and operation references are host state.
Goa only supplies the execution hooks and budget accounting.

## Concurrency and ownership

- Agent definitions, instructions, tool declarations, and configured callbacks
  are shared and must be treated as read-only during a run.
- Session and run-local state belong to one logical run. Do not share mutable
  session state between concurrent runs.
- Host callbacks must not rely on Goa to serialize access to shared host state.
- The host owns credential storage, endpoint allowlists, and authorization checks.
  Goa guards are execution-time hooks, not a replacement for authorization at the
  authoritative tool or provider boundary.
- Provider adapters expose `DisableRedirects`, `CheckRedirect`, `BeforeAttempt`,
  `MaxResponseBytes`, `MaxErrorBodyBytes`, and `StreamIdleTimeout` so transport
  policy can live in the host without replacing the adapter.
