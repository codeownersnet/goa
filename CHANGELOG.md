# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-10-30

First public release of Goa, a multi-provider Go agent framework.

### Added

- Provider-agnostic core types in `content/`, `model/`, and `schema/`, with no provider SDK imports.
- models.dev catalog integration covering 116 providers and 2000+ models, with registry and adapter factory in `provider/`.
- OpenAI Chat Completions adapter supporting 88+ providers, and an Anthropic Messages API adapter.
- Agent types: base `llmagent`, plus `sequentialagent`, `loopagent`, and `parallelagent` workflows.
- Execution loop with preprocessing, LLM calls, postprocessing, and tool handling in `flow/`.
- Streaming APIs built on `iter.Seq2`, including durable execution and stream controls.
- Runner tying agents to sessions and services.
- Session services with in-memory and SQLite backends; memory and artifact services.
- Tool system with `AgentTool`, `Bash`, `Diff`, `EditFile`, `ExitLoopTool`, function tools, `Git`, `GitHub`, `Glob`, `Grep`, `ListDir`, `Plan`, `ReadFile`, `Truncate`, and `WriteFile`.
- MCP toolset using MCP SDK v1.8 capabilities, over stdio and Streamable HTTP transports.
- Skill loading, validation, and prompt generation, with `ActivateTool`, `ResourceTool`, and `ScriptTool`.
- Workflow YAML parsing, validation, and resolution to agent trees with MCP servers.
- `goafl` CLI for running, listing, and validating workflow YAML files.
- Embedded models.dev catalog snapshot with a regeneration tool.

### Changed

- Upgraded to Go 1.27 and refreshed dependencies.
- Resolved framework review technical debt and README discrepancies.

### Build

- Added GitHub Actions CI running `make ci` on pushes and pull requests to `main`.
- Added container image and release workflow: merging a `release/<version>` PR tags the release, pushes `ghcr.io/codeownersnet/goa/goafl`, and creates a GitHub release.

[Unreleased]: https://github.com/codeownersnet/goa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/codeownersnet/goa/releases/tag/v0.1.0
