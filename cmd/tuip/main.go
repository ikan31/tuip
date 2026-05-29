package main

import (
	"fmt"
	"os"

	"github.com/tuipcli/tuip/internal/cli"
)

func main() {
	err := cli.NewRootCommand().Execute()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}
