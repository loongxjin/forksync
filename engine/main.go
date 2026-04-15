package main

import (
	"os"
	"github.com/loongxjin/forksync/engine/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
