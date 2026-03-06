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
sidings task route      → which backend should handle it?
    ↓
sidings task dispatch   → build prompt with project context, execute, emit result
```

For parallel workloads — the shell does the work:

```bash
echo "build a notifications system" \
  | sidings task decompose \
  | xargs -P 4 -I {} sh -c 'echo "{}" | sidings task classify | sidings task route | sidings task dispatch' \
  | sidings task merge
```

## Routing tiers

| Tier | Backend | Examples |
|---|---|---|
| `simple` | Ollama `qwen3.5:0.8b` | Fix typo, rename variable, add comment |
| `medium` | Ollama `qwen3.5:9b` | Write tests, add a function, small refactor |
| `complex` | Ollama `qwen2.5-coder:32b` | Implement feature, multi-file refactor |
| `exceptional` | Claude Sonnet | System design, deep debugging |

## Pipe format

Tools communicate via NDJSON — one JSON object per line. Each tool reads from stdin, enriches the object, and passes it downstream:

```json
{"task_id": "abc123", "content": "refactor the auth module"}
```
After `sidings task classify`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex"}
```
After `sidings task route`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex", "route": {"backend": "ollama", "model": "qwen2.5-coder:32b"}}
```
After `sidings task dispatch`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex", "route": {"backend": "ollama", "model": "qwen2.5-coder:32b"}, "result": "...", "duration_ms": 4200, "status": "complete"}
```

Plain text input is accepted anywhere — tools wrap it into NDJSON automatically.

## Installation

```bash
git clone https://github.com/iamchrisrice/sidings
cd sidings
make install
sidings completion install
```

**Prerequisites:**
- [Ollama](https://ollama.com) installed and running
- [Claude Code](https://claude.ai/code) installed and logged in

```bash
ollama pull qwen3.5:0.8b
ollama pull qwen3.5:9b
ollama pull qwen2.5-coder:32b
export OLLAMA_MAX_LOADED_MODELS=3
```

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
    executor/       # Ollama and Claude Code backends
    prompt/         # project context gathering and prompt construction
    pipe/           # shared NDJSON types
    telemetry/      # Unix socket event emitter
  Makefile
  go.mod
  README.md
```

## Build status

- [ ] `pkg/pipe` — shared NDJSON types
- [ ] `pkg/telemetry` — shared socket emitter
- [ ] `sidings task classify`
- [ ] `sidings task route`
- [ ] `sidings task dispatch`
- [ ] `sidings monitor`
- [ ] `sidings task decompose` + `sidings task merge`
- [ ] `sidings` wrapper with shell completion

## Name

*Sidings* are the tracks where rolling stock waits before being routed onwards. Tasks flow into the system, wait in the sidings, and are dispatched to the right destination. A distinctly British railway term for a very Unix idea.
