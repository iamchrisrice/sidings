// Package executor provides execution backends for sidings tasks.
package executor

import "github.com/iamchrisrice/sidings/pkg/pipe"

// Executor executes a routed task and returns the result.
type Executor interface {
	Execute(task pipe.Task, verbose bool) (Result, error)
}

// Result holds the outcome of task execution.
type Result struct {
	FilesWritten []string // paths of files created or modified
	Output       string   // plain-text model output (when no file blocks found)
}
