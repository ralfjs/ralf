package engine

import "runtime"

// cgoSem bounds concurrent CGo calls to runtime.NumCPU().
// CGo pins goroutines to OS threads — unbounded concurrent CGo calls cause
// unbounded OS thread creation and resource exhaustion.
var cgoSem = make(chan struct{}, runtime.NumCPU())

func acquireCGo() { cgoSem <- struct{}{} }
func releaseCGo() { <-cgoSem }

// AcquireCGo reserves a slot in the engine's CGo concurrency limiter.
// Exposed so other packages that make tree-sitter CGo calls outside the
// engine's lint path (e.g. internal/lsp's parse cache) share the same
// NumCPU-bounded thread budget.
//
// Every AcquireCGo must be paired with ReleaseCGo, typically via defer.
func AcquireCGo() { acquireCGo() }

// ReleaseCGo releases a slot previously acquired by AcquireCGo.
func ReleaseCGo() { releaseCGo() }
