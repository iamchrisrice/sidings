package classifier

// Tier pairs a tier name with the keywords that signal it.
// Edit this file to tune the heuristic classifier — one obvious place,
// easy to find and easy to change.
type Tier struct {
	Name     string
	Keywords []string
}

// tiers is the ordered list of routing tiers, from least to most complex.
// When keyword scores are counted, the tier with the highest count wins.
// Order matters for tie-breaking in the heuristic pass.
var tiers = []Tier{
	{
		Name: "simple",
		Keywords: []string{
			"fix", "rename", "format", "typo", "comment",
			"update", "change", "remove", "delete", "move",
			"add parameter", "correct",
		},
	},
	{
		Name: "medium",
		Keywords: []string{
			"add", "write", "extend", "support", "include",
			"create a", "create a function", "create a method", "create a test",
			"create a helper", "create a handler", "create a struct",
		},
	},
	{
		Name: "complex",
		Keywords: []string{
			"refactor", "redesign", "restructure", "reorganise",
			"rewrite", "multiple files", "across the codebase",
			"extract", "migrate", "implement", "add support for", "add integration",
		},
	},
	{
		Name: "exceptional",
		Keywords: []string{
			"why is", "why does", "can't figure out", "not working",
			"performance", "deadlock", "race condition",
			"architect", "system design", "design a",
			"build a", "build me", "generate", "scaffold", "bootstrap",
			"initialise", "initialize", "from scratch",
			"new project", "new service", "new application", "new api",
			"rest api", "full tests", "layered", "architecture",
			"docker", "kubernetes", "helm", "deployment", "container",
			"distroless", "microservice", "infrastructure",
			"ci/cd", "pipeline", "terraform", "cloud",
		},
	},
}
