package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"

	"github.com/buchk/TypeFerence/go/internal/mcpserver"
)

// mcpCommand serves the CLI as MCP tools over stdio (ADR-0008). Each tool
// call runs this same executable as a child process, so standard output stays
// the protocol channel and every tool behaves exactly as the command does.
//
//	typeference mcp
func mcpCommand(args []string) (int, error) {
	if len(args) > 1 {
		return 0, errors.New("typeference mcp takes no arguments")
	}
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	run := func(cliArgs []string) (string, string, int, error) {
		cmd := exec.Command(self, cliArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return stdout.String(), stderr.String(), exit.ExitCode(), nil
		}
		if err != nil {
			return "", "", 0, err
		}
		return stdout.String(), stderr.String(), 0, nil
	}
	if err := mcpserver.Serve(os.Stdin, os.Stdout, run, version); err != nil {
		return 0, err
	}
	return 0, nil
}
