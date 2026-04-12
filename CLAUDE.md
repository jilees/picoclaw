# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PicoClaw is an ultra-lightweight personal AI assistant written entirely in Go. It targets minimal hardware ($10 devices) with <10MB RAM usage and millisecond boot times. It supports 18+ messaging channels, 30+ LLM providers, voice (ASR/TTS), MCP, and a modular skills system.

## Commands

### Build

```bash
make build              # Build for current platform (runs go generate first)
make build-all          # Build for all platforms
make build-launcher     # Build Web UI launcher
make build-launcher-tui # Build TUI launcher
make install            # Build and install to ~/.local/bin
make deps               # Download and verify dependencies
go generate ./...       # Run code generation (required before builds)
```

### Test & Lint

```bash
make check              # Full pre-commit: deps + fmt + vet + test
make test               # Run all tests
make fmt                # Format code (golangci-lint)
make vet                # Static analysis
make lint               # Run linters with all build tags
make fix                # Auto-fix linting issues
go test ./pkg/agent/... # Run tests for a specific package
```

### Docker

```bash
make docker-build       # Build minimal Alpine-based image
make docker-build-full  # Build with Node.js 24 for MCP support
make docker-run         # Run gateway in Docker
```

## Architecture

PicoClaw is organized into a layered architecture centered on a `Gateway` that orchestrates all subsystems:

### Core Flow

```
CLI (cmd/picoclaw/) → Gateway (pkg/gateway/) → Agent (pkg/agent/)
                                              → Channels (pkg/channels/)
                                              → Audio (pkg/audio/)
                                              → Tools (pkg/tools/)
                                              → Cron (pkg/cron/)
```

### Key Subsystems

**`pkg/gateway/`** — Central orchestrator. Manages all channels, routes messages to the agent, handles audio I/O, cron jobs, and device state. This is the "main loop" of the running assistant.

**`pkg/agent/`** — Core stateful LLM interaction engine. Handles context management, token budgeting, summarization, event bus, hooks system, and sub-turn coordination for sub-agents. The budget/token tracking system is critical for running on constrained hardware.

**`pkg/channels/`** — 18+ platform adapters (Telegram, Discord, WhatsApp, WeChat, Matrix, IRC, Slack, DingTalk, Feishu/Lark, LINE, VK, OneBot, QQ, Teams, MaixCam, and more). Each channel implements a consistent pluggable interface.

**`pkg/providers/`** — 30+ LLM provider integrations (OpenAI, Anthropic, Google Gemini, DeepSeek, Qwen, Groq, Mistral, Azure, AWS Bedrock, Ollama, vLLM, etc.) behind a unified provider abstraction with model routing.

**`pkg/audio/`** — ASR (speech-to-text) and TTS (text-to-speech) pipelines with WebRTC support via `pion/rtp` and `pion/webrtc`.

**`pkg/tools/`** — Built-in tools: web search (DuckDuckGo, Tavily, Brave, Baidu, Perplexity, SearXNG), file operations, code execution, and scheduling.

**`pkg/mcp/`** — Native Model Context Protocol support (using `modelcontextprotocol/go-sdk`).

**`pkg/skills/`** — Modular skill discovery and loading. Skills are defined via `SKILL.md` files in the workspace directory.

**`pkg/session/`** — Session management with context budget tracking. Critical for memory-constrained targets.

**`pkg/seahorse/`** — Prompt caching optimization (cached context protocol) for reducing LLM API costs.

**`pkg/memory/`** — Long-term memory storage using a JSONL-based store.

**`pkg/bus/`** — Event bus for pub/sub communication between subsystems.

**`web/`** — Web UI with a Go backend (`web/backend/`) and a web frontend (`web/frontend/`). The launcher serves on `http://localhost:18800`.

**`cmd/picoclaw/`** — Main CLI binary. Subcommands include: agent (one-shot/interactive chat), gateway (start orchestrator), onboarding, status, model selection, auth, cron management, skills, and migration.

### Configuration

- JSON config files parsed by `pkg/config/`
- `.security.yml` for sensitive data (API keys) — kept separate from main config
- Workspace directory holds persistent state, memory, and skills
- `config/` directory contains example configuration templates

## Build Tags

The project uses Go build tags for optional features:
- `goolm` — matrix crypto optimization
- `stdjson` — standard JSON (default)
- `bedrock` — AWS Bedrock support
- `whatsapp_native` — native WhatsApp support

When running lint (`make lint`), all relevant build tags are included automatically.

## Contribution Notes

Per `CONTRIBUTING.md`, PRs must disclose AI involvement level:
- 🤖 fully AI-generated
- 🛠️ mostly AI-generated  
- 👨‍💻 mostly human-written

Run `make check` before submitting any PR. All CI checks (lint, security scan, tests) must pass.
