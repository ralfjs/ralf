package engine

import "runtime"

// cgoSem bounds concurrent CGo calls to runtime.NumCPU().
// CGo pins goroutines to OS threads — unbounded concurrent CGo calls cause
// unbounded OS thread creation and resource exhaustion.
var cgoSem = make(chan struct{}, runtime.NumCPU())

func acquireCGo() { cgoSem <- struct{}{} }
func releaseCGo() { <-cgoSem }
