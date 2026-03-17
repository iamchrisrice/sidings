# sidings

A composable ecosystem of small Unix CLI tools for intelligent LLM task routing. Tasks wait in the sidings until they're routed to the right backend — local Ollama models or Claude — based on complexity and cost.

Built in Go. Designed for Apple Silicon. Inspired by fifty years of Unix philosophy.

```bash
echo "refactor the auth module" \
  | sidings task classify \
  | sidings task route \
  | sidings task dispatch
```

## Design philosophy

**Small tools that do one thing.**
Every tool in sidings has a single, clearly defined responsibility. It does that one thing well and nothing else. Complexity lives in the composition, not inside any single binary.

**Pipes as the interface.**
Tools communicate via stdin and stdout using newline-delimited JSON (NDJSON). Any tool can be inserted, removed, or replaced without touching the others. The pipeline is the program.

**stderr for errors only.**
stdout is sacred — it carries data between tools and must stay clean. stderr is reserved for actual errors. Progress and observability flow through a telemetry socket, not the standard streams.

**Parallelism from the shell.**
Sidings doesn't implement a parallel execution framework. It doesn't need to — the shell already has one. `xargs -P` and shell backgrounding compose naturally with NDJSON output to give you parallel multi-agent pipelines without any coordination infrastructure.

**Earn complexity before building it.**
Start with three tools and a pipe. Add the next tool when you feel the limitation that motivates it. The simplest thing that works is always the right first version.

**Observable by default.**
Every tool emits structured telemetry events to a Unix socket when `sidings monitor` is running. No configuration needed — the presence of the socket is the signal. When nothing is listening, the tools are completely silent.

**Local first.**
The default routing preference is always local. Free, private, and fast enough for most tasks. Cloud models are a deliberate upgrade for tasks that genuinely need them — not the default.

## How it works

```
Task arrives
    ↓
sidings task classify   → what kind of task is this?
    ↓
sidings task route      → which model should handle it?
    ↓
sidings task dispatch   → launch Claude Code with the right model and environment
```

For local tiers (simple/medium/complex), Claude Code is pointed at Ollama via environment variables. For exceptional tasks, Claude Code uses Anthropic's API directly. The same Claude Code session handles context gathering, file writing, and permissions in both cases.

For parallel workloads — the shell does the work:

```bash
echo "build a notifications system" \
  | sidings task decompose \
  | xargs -P 4 -I {} sh -c 'echo "{}" | sidings task classify | sidings task route | sidings task dispatch' \
  | sidings task merge
```

## Prerequisites

- Go 1.21+
- [Ollama](https://ollama.com) installed and running locally
- [Claude Code](https://claude.ai/code) installed and authenticated (`claude login`)
- `git` — sidings uses `git diff` to detect files written; initialise your project with `git init` before using sidings

Pull the required models:

```bash
ollama pull qwen3.5:9b
ollama pull qwen3-coder
export OLLAMA_MAX_LOADED_MODELS=2
```

No Anthropic API key is required for simple/medium/complex tasks — these run entirely locally via Ollama. The `exceptional` tier uses your existing Claude Code subscription.

## Routing tiers

| Tier | Model | Where |
|------|-------|-------|
| simple | qwen3.5:9b | local (Ollama) |
| medium | qwen3.5:9b | local (Ollama) |
| complex | qwen3-coder | local (Ollama) |
| exceptional | Claude (claude-sonnet) | Anthropic API |

## How classification works

Every task is classified by `qwen3.5:9b` running locally via Ollama. There are no keyword lists or heuristics — the model reads the task and returns a single tier word.

Classification is deterministic (`temperature: 0`) and fast (typically under 3 seconds).

If Ollama is unavailable, classification falls back to `exceptional` so tasks still complete via the Anthropic API rather than failing silently.

To inspect classification:

```bash
echo "your task" | sidings task classify | jq '{tier, method}'
```

- `method: llm` — classified by local model (normal)
- `method: fallback` — Ollama unavailable, defaulted to exceptional

## Dispatcher behaviour

`sidings task dispatch` runs Claude Code as a subprocess. Claude Code's own output (tool use steps, progress lines) is redirected to stderr — only the final NDJSON result line is written to stdout.

This means:
- Piping works cleanly — only structured output flows downstream
- `2>/dev/null` suppresses all Claude Code chatter
- `2>dispatch.log` captures the full session log for debugging

Files written by Claude Code are detected via `git diff` before and after execution, so the NDJSON result always includes an accurate `files_written` count regardless of which tier handled the task.

`.claude/settings.json` is created automatically on first run in a new project directory with sandbox mode enabled. If the file already exists with conflicting settings, sidings exits with a clear error rather than silently overwriting your configuration.

## Pipe format

Tools communicate via NDJSON — one JSON object per line. Each tool reads from stdin, enriches the object, and passes it downstream:

```json
{"task_id": "abc123", "content": "refactor the auth module"}
```
After `sidings task classify`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex", "method": "llm"}
```
After `sidings task route`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex", "route": {"model": "qwen3-coder"}}
```
After `sidings task dispatch`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex", "route": {"model": "qwen3-coder"}, "files_written": ["pkg/auth/auth.go"], "duration_ms": 4200, "status": "complete"}
```

Plain text input is accepted anywhere — tools wrap it into NDJSON automatically.

## Installation

```bash
git clone https://github.com/iamchrisrice/sidings
cd sidings
make install
```

Installs to:
- `~/.local/bin/sidings` — the main wrapper
- `~/.local/libexec/sidings/` — internal binaries (task-classify, task-route, task-dispatch)

Add `~/.local/bin` to your PATH if not already present.

### Shell completion

```bash
sidings completion install
```

Detects your shell automatically and installs completion. Supports bash, zsh, and fish.

## Tips

**Suppress Claude Code output:**
```bash
echo "fix the typo in README.md" \
  | sidings task classify \
  | sidings task route \
  | sidings task dispatch 2>/dev/null
```

**Log Claude Code output for debugging:**
```bash
echo "add error handling to main.go" \
  | sidings task classify \
  | sidings task route \
  | sidings task dispatch 2>session.log
```

**Run from your project root** — sidings gathers context from the current working directory.

**Initialise git before using sidings** — sidings uses `git diff` to track which files were written. Without a git repo, file tracking won't work.

**`exceptional` tier tasks use the Anthropic API** — ensure Claude Code is authenticated (`claude login`) before running tasks that might route to exceptional.

## Build status

- [x] `pkg/pipe` — shared NDJSON types
- [x] `pkg/telemetry` — socket emitter
- [x] `pkg/tty` — /dev/tty reader for confirmation prompts
- [x] `pkg/classifier` — LLM-only classifier (qwen3.5:9b)
- [x] `pkg/router` — tier-to-model mapping
- [x] `pkg/executor/claude.go` — single executor for all tiers
- [x] `cmd/internal/task-classify`
- [x] `cmd/internal/task-route`
- [x] `cmd/internal/task-dispatch`
- [x] `cmd/sidings` — wrapper with shell completion
- [ ] `sidings monitor`
- [ ] `sidings task decompose` + `sidings task merge`

## Project structure

```
sidings/
  cmd/
    sidings/                  # public wrapper binary with shell completion
    internal/                 # libexec binaries — not public interface
      task-classify/
      task-route/
      task-dispatch/
      monitor/
      task-decompose/
      task-merge/
  pkg/
    classifier/     # classification logic and tier definitions
    router/         # routing table and decision logic
    executor/       # Claude Code executor (all tiers)
    pipe/           # shared NDJSON types
    telemetry/      # Unix socket event emitter
    tty/            # terminal input helpers
  Makefile
  go.mod
  README.md
```

## Name

*Sidings* are the tracks where rolling stock waits before being routed onwards. Tasks flow into the system, wait in the sidings, and are dispatched to the right destination. A distinctly British railway term for a very Unix idea.
