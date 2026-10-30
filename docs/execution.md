# Execution order and policy hooks

`flow.Flow` and `llmagent.Config` expose the same execution controls. The
`llmagent` constructor copies `ExecutionOptions`, `ResultPolicy`, limits, and
callbacks into the internal `Flow`.

## Model dispatch order

1. `RequestProcessors` build or rewrite `model.ModelRequest`.
2. `ContextPolicy` runs on the final rewritten request.
3. `RunLimits.MaxPromptBytes` is checked.
4. `BeforeModelGuards` run. A guard error is terminal and cannot be overridden by
   callbacks.
5. `BeforeModelCallbacks` run. A callback may return a replacement response and
   short-circuit the provider call.
6. The provider adapter runs. Provider-level retries happen inside the adapter;
   `SingleAttempt` disables them and `BeforeAttempt` is the attempt-level gate.
   See `docs/host-integration.md` for retry ownership.
7. `AfterModelCallbacks` run.
8. `ResponseProcessors` run.
9. Output limits are checked and a `session.Event` is emitted.
10. If the response contains tool calls, the tool phase runs.

## Tool phase order

1. Tool scheduling is selected (`sequential` or bounded parallel).
2. `BeforeToolCallbacks` may replace the arguments.
3. `BeforeToolGuards` see the final rewritten arguments. A guard error is a
   terminal denial.
4. The tool's `Process` runs under `ToolTimeout` / `MaxToolDuration`.
5. `AfterToolCallbacks` may replace the result, unless the tool returned a
   terminal outcome.
6. `ResultPolicy` applies (default: bounded in-memory truncation).
7. `MaxToolResultBytes` is checked.
8. `OnToolErrorCallbacks` run for recoverable errors only. Terminal `denied` and
   `uncertain` outcomes bypass these callbacks.

## Completion order

1. `OutputValidator` runs on the final non-partial event. Validation failure is
   never success. If `MaxValidationRepairs > 0`, a bounded repair request is
   appended and the model runs again without implicitly replaying tools.
2. `BeforeCompletionGuards` run. A guard error is a terminal denial.
3. The run completes.

## Terminal tool outcomes

Tools return ordinary errors for recoverable failures. Use:

- `tool.NewDeniedError(err)` for policy denial.
- `tool.NewUncertainError(err)` when a mutation may or may not have committed.

Both stop the run. In parallel mode, started siblings are cancelled
cooperatively; already-started external writes may still finish.

## Retry ownership

Choose one owner of retries. Either let the bundled adapter retry (default, five
attempts) or configure `SingleAttempt` and retry in the host. Do not wrap a
retrying adapter in a host retry loop unless the resulting total attempt budget is
intentional. Retries never replay after the adapter has emitted output.
