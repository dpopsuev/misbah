package main

import (
	"os"

	"github.com/dpopsuev/misbah/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
