package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codeownersnet/goa/workflow"
)

func validateCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: goafl validate <workflow.yaml>")
		os.Exit(1)
	}
	path := resolveWorkflowPath(args[0])

	err := workflow.ValidateFile(context.Background(), path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validation error: %v\n", err)
		os.Exit(2)
	}

	fmt.Printf("valid: %s\n", path)
}
