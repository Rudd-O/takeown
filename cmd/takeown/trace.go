package main

import (
	"fmt"
	"os"
	"runtime"
)

var tracing bool

// setTrace attempts to set tracing on.  If the file /.trace does not exist,
// it fails and returns false.  Otherwise it sets tracing on and returns true.
func setTrace() bool {
	if _, err := os.Stat("/.trace"); err != nil {
		return false
	}
	tracing = true
	return true
}

func trace(s string, args ...interface{}) {
	if tracing {
		pc := make([]uintptr, 10) // at least 1 entry needed
		runtime.Callers(2, pc)
		f := runtime.FuncForPC(pc[0])
		file, line := f.FileLine(pc[0])
		prefix := fmt.Sprintf("%s:%d %s: ", file, line, f.Name())
		fmt.Fprintf(os.Stderr, prefix+s+"\n", args...)
	}
}
