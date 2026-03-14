package main

import (
	"os"

	"github.com/jabal/jabal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
