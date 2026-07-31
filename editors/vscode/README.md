# TypeFerence for VS Code

A thin VS Code client for the TypeFerence language server. It associates `.tfer`
files with the `tfer` language (YAML frontmatter highlighted as YAML, the body as
markdown) and launches `typeference-lsp` to provide:

- shape and workspace-composition diagnostics,
- completion of resource kinds and field names,
- go-to-definition on resource ids,
- document symbols.

## Prerequisites

Build and install the language server so it is on `PATH`:

```sh
make build-lsp        # produces bin/typeference-lsp
# put bin/ on PATH, or set the setting below
```

If it is not on `PATH`, set `"typeference.serverPath"` to its absolute location
in your VS Code settings.

## Develop / run

```sh
cd editors/vscode
npm install
```

Then open this folder in VS Code and press F5 to launch an Extension
Development Host, or package a `.vsix`:

```sh
npx @vscode/vsce package
```

## Status

The extension source is complete and self-contained; the LSP server it drives
(`internal/lsp`) is covered by Go tests. The extension itself has not been run
end-to-end in CI — build it with `npm install` and load it to try it.
