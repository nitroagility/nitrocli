// Package main is the entry point for the NitroCLI binary.
package main

import (
	"os"

	"github.com/nitroagility/nitrocli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
