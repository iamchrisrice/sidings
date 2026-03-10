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

## Routing tiers

| Tier | Model | Examples |
|---|---|---|
| `simple` | `qwen3.5:0.8b` (local) | Fix typo, rename variable, add comment |
| `medium` | `qwen3.5:9b` (local) | Write tests, add a function, small refactor |
| `complex` | `qwen3-coder` (local) | Implement feature, multi-file refactor |
| `exceptional` | Claude Sonnet (Anthropic API) | System design, deep debugging, greenfield projects |

Local tiers run through Claude Code pointed at Ollama — free, private, no API calls.
Exceptional tier runs through Claude Code using your Anthropic subscription.

## How classification works

### `sidings task classify`

Classifies a coding task as `simple`, `medium`, `complex`, or `exceptional` using `qwen3.5:0.8b` running locally via Ollama. Returns a single word — no heuristics, no keyword lists.

```bash
echo "rename this variable" | sidings task classify
# {"task_id": "abc123", "content": "rename this variable", "tier": "simple", "method": "llm"}
```

If Ollama is unavailable, defaults to `exceptional`.

Tier definitions:
- `simple` — single-line changes, typos, renames, adding a comment
- `medium` — adding a function, writing a test, small self-contained change
- `complex` — multi-file changes, refactoring, implementing a feature
- `exceptional` — greenfield projects, system design, deep debugging, infrastructure

## Known behaviour

**All tiers use Claude Code.**
Claude Code handles context gathering, file writing, and permissions for every tier. For local tiers it is pointed at Ollama via `ANTHROPIC_BASE_URL`. For exceptional tasks it uses Anthropic's API directly. The pipeline behaviour is identical regardless of which model runs the task.

**Classification uses a local LLM.**
Every task is classified by `qwen3.5:0.8b` running locally via Ollama. This adds ~1-3 seconds per task. If Ollama is unavailable, all tasks default to `exceptional` and route to Claude Code via the Anthropic API.

**Greenfield project creation always routes to exceptional.**
Tasks like "create a REST API" or "scaffold a new service" always route to Claude Sonnet. Local models via Ollama handle targeted edits well but struggle with large multi-file creation in a single session.

**`.claude/settings.json` is created automatically.**
On first run in a new project directory, `sidings task dispatch` creates `.claude/settings.json` with sandbox mode enabled. If the file already exists with conflicting settings, sidings exits with a clear error rather than overwriting your configuration.

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
git clone https://github.com/you/sidings
cd sidings
make install
```

Installs to `~/.local/`:
- Libexec binaries → `~/.local/libexec/sidings/`
- `sidings` wrapper → `~/.local/bin/sidings`

Make sure `~/.local/bin` is on your PATH:
```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Shell completion

```bash
sidings completion install
```

Detects your shell automatically and installs completion. Supports bash, zsh, and fish.

**Prerequisites:**
- Go 1.21+
- [Ollama](https://ollama.com) installed and running
- [Claude Code](https://claude.ai/code) installed and authenticated (`claude login`)
- `git` — sidings uses git for context gathering; run `git init` in your project before using sidings

Pull the required models:

```bash
ollama pull qwen3.5:0.8b   # classifier + simple tasks
ollama pull qwen3.5:9b     # medium tasks
ollama pull qwen3-coder    # complex tasks
export OLLAMA_MAX_LOADED_MODELS=3
```

No Anthropic API key needed — Claude Code handles auth with your existing Claude subscription.

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

## Build status

- [x] `pkg/pipe` — shared NDJSON types
- [x] `pkg/telemetry` — shared socket emitter
- [x] `pkg/tty` — terminal input helpers
- [x] `sidings task classify`
- [x] `sidings task route`
- [x] `sidings task dispatch`
- [x] `sidings` wrapper with shell completion
- [ ] `sidings monitor`
- [ ] `sidings task decompose` + `sidings task merge`

## Name

*Sidings* are the tracks where rolling stock waits before being routed onwards. Tasks flow into the system, wait in the sidings, and are dispatched to the right destination. A distinctly British railway term for a very Unix idea.
