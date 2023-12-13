package main

import (
	"github.com/Azure/draft/test/e2e/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
