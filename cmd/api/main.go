package main

import (
	"context"
	"os"

	"github.com/jeromelaurens/api-cli/internal/runner"
)

func main() {
	os.Exit(runner.NewCLI().Run(context.Background(), os.Args[1:]))
}
