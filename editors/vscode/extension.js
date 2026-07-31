// Minimal VS Code client for the TypeFerence language server. It launches
// `typeference-lsp` (which must be on PATH) and drives it over stdio for .tfer
// sources: diagnostics, completion, go-to-definition, and document symbols.
const { workspace, window } = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

function activate(context) {
  const command = workspace.getConfiguration("typeference").get("serverPath") || "typeference-lsp";
  const server = { command, transport: TransportKind.stdio };
  const serverOptions = { run: server, debug: server };
  const clientOptions = {
    documentSelector: [{ scheme: "file", language: "tfer" }],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/*.{tfer,yaml}"),
    },
  };
  client = new LanguageClient("typeference", "TypeFerence", serverOptions, clientOptions);
  client.start().catch((err) => {
    window.showErrorMessage(
      "TypeFerence: could not start typeference-lsp (" + command + "). Is it on PATH? " + err.message,
    );
  });
  context.subscriptions.push({ dispose: () => client && client.stop() });
}

function deactivate() {
  return client ? client.stop() : undefined;
}

module.exports = { activate, deactivate };
