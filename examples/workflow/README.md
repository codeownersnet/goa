# Workflow Agent Examples

This directory contains examples demonstrating Goa's built-in workflow agents. Workflow agents orchestrate multiple sub-agents using predefined execution patterns — no LLM reasoning is involved in the orchestration itself.

## Examples

### 1. Sequential Agent (`sequential/`)

Runs two or more sub-agents in strict order. The text output of each agent is passed as the user message to the next agent.

```bash
go run ./examples/workflow/sequential
```

### 2. Sequential Code Pipeline (`sequential_code/`)

A 3-stage LLM-powered pipeline: **Code Writer → Reviewer → Refactorer**. Each stage is an `llmagent` that receives the previous agent's output as input. Requires `SYNTHETIC_API_KEY` env var.

```bash
go run ./examples/workflow/sequential_code
```

### 3. Parallel Agent (`parallel/`)

Runs multiple sub-agents concurrently in goroutines. Events from all sub-agents are collected and yielded after all complete.

```bash
go run ./examples/workflow/parallel
```

### 4. Loop Agent (`loop/`)

Repeats a sub-agent for a configured number of iterations (default 10). Each iteration's branch is named `loop-demo/iter{N}/{agentName}`.

```bash
go run ./examples/workflow/loop
```

### 5. Loop Agent with LLM (`loop_llm/`)

An LLM-powered agent that iteratively refines text. The agent can call the `exit_loop` tool to terminate the loop early when satisfied. Requires `SYNTHETIC_API_KEY` env var.

```bash
go run ./examples/workflow/loop_llm
```
