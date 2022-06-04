package main

import (
	"flag"
	"fmt"
	"os"
)

const (
	Success          = 0
	BadConfig        = 8
	OperationError   = 32
	Usage            = 64
	PermissionDenied = 128
)

var addFlag = flag.Bool("a", false, "add a delegation for a specific user and path")
var listFlag = flag.Bool("l", false, "list user delegations established on paths")
var deleteFlag = flag.Bool("d", false, "remove a delegation for a specific user and path")
var recurseFlag = flag.Bool("r", false, "take ownership recursively")
var verboseFlag = flag.Bool("v", false, "when taking ownership, print out the actions taken")
var simulateFlag = flag.Bool("s", false, "simulate taking ownership")
var traceFlag = flag.Bool("T", false, "show trace of internal execution; requires file `/.trace` to exist")

func usage() {
	fmt.Fprintf(os.Stderr, USAGE)
}

func main() {
	flag.Parse()

	if *traceFlag {
		if set := setTrace(); !set {
			fmt.Fprintf(os.Stderr, "error: the file /.trace must exist to enable tracing\n")
			os.Exit(PermissionDenied)
		}
	}

	if *listFlag {
		if *recurseFlag || *addFlag || *deleteFlag || *simulateFlag || *verboseFlag {
			usage()
			os.Exit(Usage)
		}
		paths := flag.Args()
		if len(paths) == 0 {
			paths = []string{"."}
		}
		os.Exit(listDelegations(paths))
	}

	if *addFlag {
		if *recurseFlag || *listFlag || *deleteFlag || *simulateFlag || *verboseFlag {
			usage()
			os.Exit(Usage)
		}
		if flag.NArg() < 2 {
			usage()
			os.Exit(Usage)
		}
		os.Exit(addDelegation(flag.Args()[0], flag.Args()[1:]))
	}

	if *deleteFlag {
		if *recurseFlag || *addFlag || *listFlag || *simulateFlag || *verboseFlag {
			usage()
			os.Exit(Usage)
		}
		if flag.NArg() < 2 {
			usage()
			os.Exit(Usage)
		}
		os.Exit(deleteDelegation(flag.Args()[0], flag.Args()[1:]))
	}

	if flag.NArg() < 1 {
		usage()
		os.Exit(Usage)
	}

	os.Exit(takeOwnership(flag.Args(), *recurseFlag, *simulateFlag, *verboseFlag))
}
