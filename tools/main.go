// Command tools is the harness toolbox: one binary, subcommands.
package main

import (
	"fmt"
	"os"
)

var subcommands = map[string]func(args []string) error{
	"bot":        runBot,
	"rcon":       runRcon,
	"gen-matrix": runGenMatrix,
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	run, ok := subcommands[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tools <bot|rcon|gen-matrix> [args]")
}
