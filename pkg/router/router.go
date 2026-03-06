// Package router maps classification tiers to execution backends.
// The routing table is a plain map — easy to find, easy to edit.
package router

// Router maps a tier string to a routing Decision.
type Router interface {
	Route(tier string) (Decision, error)
}

// Decision is the selected backend and model for a task.
type Decision struct {
	Backend string // "ollama" or "claude"
	Model   string
}

// defaultRoutes is the hardcoded fallback table used when no config file exists.
// Edit here to change the default routing behaviour.
var defaultRoutes = map[string]Decision{
	"simple":      {Backend: "ollama", Model: "qwen3.5:0.8b"},
	"medium":      {Backend: "ollama", Model: "qwen3.5:9b"},
	"complex":     {Backend: "ollama", Model: "qwen2.5-coder:32b"},
	"exceptional": {Backend: "claude", Model: "sonnet"},
}

type tableRouter struct {
	table map[string]Decision
}

// New creates a Router using the provided routing table.
// Use LoadConfig to build the table from config file + defaults.
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
