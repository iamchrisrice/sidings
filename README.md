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

## Known behaviour

**Greenfield project creation always routes to exceptional.**
Tasks like "create a REST API" or "scaffold a new service" always route to Claude Code. Local models cannot reliably produce multiple complete files in a single shot. This is by design — Claude Code handles multi-file creation iteratively, which works far better than a single-shot prompt to a local model.

**Classification improves with specificity.**
"Create a function to validate emails" routes correctly to medium. "Create a REST API with full tests" routes correctly to exceptional. Vague short tasks are more likely to misclassify — the more specific the task, the better the classification.

**Classifier keyword tuning.**
The heuristic keyword lists live in `pkg/classifier/tiers.go` and are easy to edit. If you notice consistent misclassifications for your workflow, adding keywords there is the fastest fix. The `method` and `matched` fields in classify output show exactly which keywords fired:

```bash
echo "your task" | sidings task classify | jq '{tier, method, matched}'
```

**`.claude/settings.json` is created automatically.**
On first run in a new project directory, `sidings task dispatch` creates `.claude/settings.json` with sandbox mode enabled. This allows Claude Code to run autonomously within the project directory without permission prompts. If the file already exists with conflicting settings, sidings will exit with a clear error rather than overwriting your configuration.

**Ollama models respond in `<file>` blocks.**
When dispatching to local models, sidings instructs them to respond using XML-style file blocks. Most of the time this works. Occasionally a model responds in prose instead — when this happens the response is stored in the `result` field of the NDJSON output and no files are written. Use `--dry-run` to inspect the prompt if you see unexpected prose responses.

## Pipe format

Tools communicate via NDJSON — one JSON object per line. Each tool reads from stdin, enriches the object, and passes it downstream:

```json
{"task_id": "abc123", "content": "refactor the auth module"}
```
After `sidings task classify`:
```json
{"task_id": "abc123", "content": "refactor the auth module", "tier": "complex", "method": "heuristic", "matched": ["refactor"]}
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

## Diagnosing misclassifications

The `method` and `matched` fields show exactly how a classification was reached:

```bash
echo "your task" | sidings task classify | jq '{tier, method, matched}'
# {"tier": "medium", "method": "heuristic", "matched": ["create"]}
```

- `method: heuristic` — keyword list won outright
- `method: llm` — ambiguous heuristics, LLM fallback made the call
- `method: fallback` — LLM unavailable, defaulted to medium

If the wrong keywords are firing, edit `pkg/classifier/tiers.go` and rebuild.

## Installation

```bash
git clone https://github.com/you/sidings
cd sidings
make install
```

Installs libexec binaries to `~/.local/libexec/sidings/` and the `sidings` wrapper to `~/.local/bin/`.

Until the `sidings` wrapper is built, call the libexec binaries directly:

```bash
echo "refactor the auth module" \
  | ~/.local/libexec/sidings/task-classify \
  | ~/.local/libexec/sidings/task-route \
  | ~/.local/libexec/sidings/task-dispatch
```

**Prerequisites:**
- Go 1.21+
- [Ollama](https://ollama.com) installed and running
- [Claude Code](https://claude.ai/code) installed and authenticated (`claude login`)
- `git` — sidings uses git for context gathering; run `git init` in your project before using sidings

Pull the required models:

```bash
ollama pull qwen3.5:0.8b
ollama pull qwen3.5:9b
ollama pull qwen2.5-coder:32b
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
    executor/       # Ollama and Claude Code backends
    prompt/         # project context gathering and prompt construction
    pipe/           # shared NDJSON types
    telemetry/      # Unix socket event emitter
  Makefile
  go.mod
  README.md
```

## Build status

- [x] `pkg/pipe` — shared NDJSON types
- [x] `pkg/telemetry` — shared socket emitter
- [x] `sidings task classify`
- [x] `sidings task route`
- [x] `sidings task dispatch`
- [ ] `sidings monitor`
- [ ] `sidings task decompose` + `sidings task merge`
- [ ] `sidings` wrapper with shell completion

## Name

*Sidings* are the tracks where rolling stock waits before being routed onwards. Tasks flow into the system, wait in the sidings, and are dispatched to the right destination. A distinctly British railway term for a very Unix idea.
