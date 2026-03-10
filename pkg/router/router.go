// Package router maps classification tiers to execution models.
// The routing table is a plain map — easy to find, easy to edit.
package router

// Router maps a tier string to a routing Decision.
type Router interface {
	Route(tier string) (Decision, error)
}

// Decision is the selected model for a task.
// All tasks are executed via Claude Code — the model determines which backend
// Claude Code uses (Ollama for local tiers, Anthropic API for exceptional).
type Decision struct {
	Model string
}

// defaultRoutes is the hardcoded fallback table used when no config file exists.
var defaultRoutes = map[string]Decision{
	"simple":      {Model: "qwen3.5:0.8b"},
	"medium":      {Model: "qwen3-coder"},
	"complex":     {Model: "qwen3-coder"},
	"exceptional": {Model: ""}, // empty = Claude Code default (Sonnet)
}

type tableRouter struct {
	table map[string]Decision
}

// New creates a Router using the provided routing table.
func New(table map[string]Decision) Router {
	return &tableRouter{table: table}
}

// Route looks up tier in the routing table.
// Unknown tiers fall back to the medium route — never fatal.
func (r *tableRouter) Route(tier string) (Decision, error) {
	if d, ok := r.table[tier]; ok {
		return d, nil
	}
	return r.table["medium"], nil
}
