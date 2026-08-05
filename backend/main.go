package main

import (
	"fmt"
	"os"

	"github.com/certvault/certvault/cli"
)

func main() {
	args := os.Args[1:]
	if cli.IsInvocation(args) {
		exitOnError(cli.Run(args, os.Stdout, os.Stderr))
		return
	}

	exitOnError(runServer(args, os.Stdout, os.Stderr))
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "certvault: %v\n", err)
		os.Exit(1)
	}
}
