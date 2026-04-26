package main

import (
	"embed"
	"fmt"
	"os"
	"time"

	sdkagent "github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/terminal/cli"
)

//go:embed all:.agents
var miniagentResources embed.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	sessionsDir, err := cli.HomeSessionDir(".miniagent/sessions")
	if err != nil {
		return err
	}
	cmd := cli.NewCommand(cli.CommandConfig{
		Name:                  "miniagent",
		Use:                   "miniagent [task]",
		Short:                 "A minimal agentic CLI with tools",
		Long:                  "miniagent runs an autonomous agent loop: model -> tools -> model -> ...\nWith no arguments it starts an interactive REPL.\nWith positional arguments it runs the task once and exits.",
		Resources:             cli.EmbeddedResources(miniagentResources, ".agents"),
		DefaultSessionsDir:    sessionsDir,
		CacheKeyPrefix:        "miniagent:",
		Prompt:                "> ",
		DefaultInference:      sdkagent.DefaultInferenceOptions(),
		DefaultMaxSteps:       30,
		DefaultToolTimeout:    30 * time.Second,
		ApplyDefaultInference: true,
		ApplyDefaultMaxSteps:  true,
	})
	return cmd.Execute()
}
