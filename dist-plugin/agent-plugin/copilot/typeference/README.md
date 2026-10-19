# typeference plugin

Author, extend, and check TypeFerence packages from an agent. The plugin holds
one skill, `edit-typeference`, and one MCP server, `typeference-compiler`,
which serves the typeference CLI as tools (`typeference mcp`).

The server runs the `typeference` command, so the compiler must be on PATH.
With Go 1.24 or later:

```sh
go install github.com/buchk/TypeFerence/go/cmd/typeference@main
```

or clone https://github.com/buchk/TypeFerence and run `make build-go`.

This plugin is generated from `agents/typeference` in that repository; do not
edit it here.
