// TypeFerence Playground. Vanilla JS, no dependencies: the page loads the
// real Go compiler as WebAssembly (typeference.wasm), gives it an in-memory
// filesystem (memfs.js), and recompiles every Copilot plugin on every edit.
// Nothing leaves the tab.
"use strict";

/* ---------------------------------------------------------------- state */

const state = {
  files: new Map(),      // path -> content (the editable source tree)
  activePath: null,
  example: null,         // name of the example the tree started from
  examples: [],
  result: null,          // last successful compile result
  activeArtifact: null,
  activeAgent: null,
  ready: false,
  formFile: null,       // data document the Instantiate tab edits
};

const $ = (id) => document.getElementById(id);
const els = {
  exampleSelect: $("example-select"),
  reset: $("reset-btn"),
  share: $("share-btn"),
  theme: $("theme-btn"),
  fileList: $("file-list"),
  addFile: $("add-file"),
  editor: $("editor"),
  highlight: document.querySelector("#highlight code"),
  highlightPre: $("highlight"),
  diagnostics: $("diagnostics"),
  outputPane: $("output-pane"),
  artifactList: $("artifact-list"),
  artifactView: document.querySelector("#artifact-view code"),
  graph: $("graph"),
  graphLegend: $("graph-legend"),
  bundleAgent: $("bundle-agent"),
  bundleView: document.querySelector("#bundle-view code"),
  status: $("status"),
  formFile: $("form-file"),
  formFields: $("form-fields"),
  formPreview: document.querySelector("#form-preview code"),
  formHint: $("form-hint"),
};

/* ---------------------------------------------------------------- theme */

function initTheme() {
  const saved = localStorage.getItem("tf-theme");
  const preferred = matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  document.documentElement.dataset.theme = saved || preferred;
  els.theme.addEventListener("click", () => {
    const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    localStorage.setItem("tf-theme", next);
    if (state.result) renderGraph(state.result); // repaint kind colors
  });
}

/* ------------------------------------------------------------ utilities */

const escapeHTML = (s) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

function toast(message) {
  let el = $("toast");
  if (!el) {
    el = document.createElement("div");
    el.id = "toast";
    document.body.appendChild(el);
  }
  el.textContent = message;
  el.classList.add("show");
  clearTimeout(el._timer);
  el._timer = setTimeout(() => el.classList.remove("show"), 2200);
}

function setStatus(kind, html) {
  els.status.className = kind;
  els.status.innerHTML = html;
}

/* ------------------------------------------------------ syntax coloring */

function highlightYAML(text) {
  return text.split("\n").map((line) => {
    const comment = line.match(/^(\s*)(#.*)$/);
    if (comment) return escapeHTML(comment[1]) + `<span class="tok-comment">${escapeHTML(comment[2])}</span>`;
    const kv = line.match(/^(\s*(?:-\s+)?)([A-Za-z0-9._-]+)(:)( .*|$)/);
    if (kv) {
      const [, indent, key, colon, rest] = kv;
      return `<span class="tok-punct">${escapeHTML(indent)}</span>` +
        `<span class="tok-key">${escapeHTML(key)}</span>` +
        `<span class="tok-punct">${colon}</span>` +
        highlightYAMLValue(rest);
    }
    const item = line.match(/^(\s*-\s+)(.*)$/);
    if (item) return `<span class="tok-punct">${escapeHTML(item[1])}</span>` + highlightYAMLValue(item[2]);
    return escapeHTML(line);
  }).join("\n");
}

function highlightYAMLValue(value) {
  const trimmed = value.trim();
  if (/^['"].*['"]$/.test(trimmed) || /^[|>]-?$/.test(trimmed)) {
    return `<span class="tok-str">${escapeHTML(value)}</span>`;
  }
  if (/^-?\d+(\.\d+)?$/.test(trimmed) || trimmed === "true" || trimmed === "false") {
    return `<span class="tok-num">${escapeHTML(value)}</span>`;
  }
  return escapeHTML(value);
}

function highlightMarkdown(text) {
  return text.split("\n").map((line) => {
    if (/^#{1,6}\s/.test(line)) return `<span class="tok-head">${escapeHTML(line)}</span>`;
    return escapeHTML(line).replace(/`[^`]+`/g, (m) => `<span class="tok-str">${m}</span>`);
  }).join("\n");
}

function highlightJSON(text) {
  let html = "";
  const re = /("(?:[^"\\]|\\.)*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|([{}\[\],:])/g;
  let last = 0, m;
  while ((m = re.exec(text)) !== null) {
    html += escapeHTML(text.slice(last, m.index));
    if (m[1] !== undefined) {
      html += `<span class="${m[2] ? "tok-key" : "tok-str"}">${escapeHTML(m[1])}</span>${m[2] || ""}`;
    } else if (m[3] !== undefined) {
      html += `<span class="tok-num">${escapeHTML(m[3])}</span>`;
    } else {
      html += `<span class="tok-punct">${escapeHTML(m[4])}</span>`;
    }
    last = m.index + m[0].length;
  }
  return html + escapeHTML(text.slice(last));
}

function highlightFor(path, text) {
  if (path.endsWith(".yaml") || path.endsWith(".yml") || path.endsWith(".tfer")) return highlightYAML(text);
  if (path.endsWith(".md")) return highlightMarkdown(text);
  if (path.endsWith(".json")) return highlightJSON(text);
  return escapeHTML(text);
}

/* --------------------------------------------------------------- editor */

function openFile(path) {
  state.activePath = path;
  els.editor.value = state.files.get(path) ?? "";
  refreshHighlight();
  els.editor.scrollTop = 0;
  renderFileList();
}

function refreshHighlight() {
  // Trailing newline keeps the pre's height in sync with the textarea.
  els.highlight.innerHTML = highlightFor(state.activePath || "", els.editor.value) + "\n";
}

function initEditor() {
  els.editor.addEventListener("input", () => {
    if (state.activePath) state.files.set(state.activePath, els.editor.value);
    refreshHighlight();
    scheduleCompile();
  });
  els.editor.addEventListener("scroll", () => {
    els.highlightPre.scrollTop = els.editor.scrollTop;
    els.highlightPre.scrollLeft = els.editor.scrollLeft;
  });
  els.editor.addEventListener("keydown", (e) => {
    if (e.key === "Tab") {
      e.preventDefault();
      const { selectionStart: start, selectionEnd: end, value } = els.editor;
      els.editor.value = value.slice(0, start) + "  " + value.slice(end);
      els.editor.selectionStart = els.editor.selectionEnd = start + 2;
      els.editor.dispatchEvent(new Event("input"));
    }
  });
}

/* ------------------------------------------------------------ file pane */

function renderFileList() {
  els.fileList.innerHTML = "";
  const paths = [...state.files.keys()].sort();
  let currentDir = null;
  for (const path of paths) {
    const slash = path.lastIndexOf("/");
    const dir = slash < 0 ? "" : path.slice(0, slash);
    if (dir !== currentDir) {
      currentDir = dir;
      if (dir !== "") {
        const dirEl = document.createElement("li");
        dirEl.className = "dir";
        dirEl.textContent = dir + "/";
        els.fileList.appendChild(dirEl);
      }
    }
    const li = document.createElement("li");
    li.title = path;
    if (path === state.activePath) li.classList.add("active");
    const name = document.createElement("span");
    name.textContent = slash < 0 ? path : path.slice(slash + 1);
    li.appendChild(name);
    const del = document.createElement("button");
    del.className = "del";
    del.textContent = "✕";
    del.title = `Delete ${path}`;
    del.addEventListener("click", (e) => {
      e.stopPropagation();
      state.files.delete(path);
      if (state.activePath === path) openFile(paths.find((p) => p !== path) ?? null);
      renderFileList();
      scheduleCompile();
    });
    li.appendChild(del);
    li.addEventListener("click", () => openFile(path));
    els.fileList.appendChild(li);
  }
}

function initFilePane() {
  els.addFile.addEventListener("click", () => {
    const path = prompt("New file path (e.g. skills/triage.skill.tfer):");
    if (!path || state.files.has(path)) return;
    const clean = path.replace(/^\/+/, "");
    // The file name decides the kind; link the new document from a plugin
    // (or from something a plugin ships) so it becomes part of the build.
    const template = clean.endsWith(".md")
      ? "# Notes\n"
      : "---\ndescription: Say what this does and when to use it.\n---\nWrite the instructions here.\n";
    state.files.set(clean, template);
    openFile(clean);
    scheduleCompile();
  });
}

/* -------------------------------------------------------------- compile */

let compileTimer = null;
function scheduleCompile() {
  clearTimeout(compileTimer);
  compileTimer = setTimeout(compileNow, 300);
}

// buildRequest splits the editable tree into the package being built and its
// dependency packages. A marketplace example prefixes every path with its
// package directory; a single-package example does not.
function buildRequest(example) {
  const request = { files: {}, packages: {}, sourceName: state.example || "src" };
  const rootDir = example?.root || "";
  const packages = example?.packages || [];
  for (const [path, content] of state.files) {
    if (!rootDir) {
      request.files[path] = content;
      continue;
    }
    const slash = path.indexOf("/");
    const dir = slash < 0 ? "" : path.slice(0, slash);
    const rest = slash < 0 ? path : path.slice(slash + 1);
    if (dir === rootDir) request.files[rest] = content;
    else if (packages.includes(dir)) (request.packages[dir] = request.packages[dir] || {})[rest] = content;
  }
  return request;
}

function compileNow() {
  if (!state.ready) return;
  const started = performance.now();
  const example = state.examples.find((e) => e.name === state.example);
  const request = buildRequest(example);
  let result;
  try {
    result = globalThis.TypeFerence.compile(request);
  } catch (err) {
    result = { ok: false, error: String(err) };
  }
  const elapsed = Math.max(1, Math.round(performance.now() - started));

  if (!result || !result.ok) {
    const message = result && result.error ? result.error : "compiler returned no result";
    els.diagnostics.hidden = false;
    els.diagnostics.textContent = message;
    els.outputPane.classList.add("stale");
    setStatus("error", `compile failed · ${elapsed} ms — details under the editor`);
    if (result && result.graph) renderGraph(result);
    if (result && result.contextTypes) renderForm(result);
    return;
  }

  els.diagnostics.hidden = true;
  els.outputPane.classList.remove("stale");
  state.result = result;
  const fileCount = Object.keys(result.files).length;
  const digest = result.hash.replace(/^sha256:/, "").slice(0, 16);
  const emitted = result.agents.length;
  setStatus("ok",
    `${emitted} agent${emitted === 1 ? "" : "s"} · ${fileCount} artifacts · ` +
    `<span class="digest">SHA-256 <b>${digest}…</b></span> · ${elapsed} ms · ` +
    `same bytes the CLI produces`);
  renderArtifacts(result);
  renderGraph(result);
  renderBundle(result);
  renderForm(result);
}

/* ------------------------------------------------------------ artifacts */

function renderArtifacts(result) {
  const paths = Object.keys(result.files).sort();
  els.artifactList.innerHTML = "";
  let currentDir = null;
  for (const path of paths) {
    const slash = path.lastIndexOf("/");
    const dir = slash < 0 ? "" : path.slice(0, slash);
    if (dir !== currentDir) {
      currentDir = dir;
      if (dir !== "") {
        const dirEl = document.createElement("div");
        dirEl.className = "dir";
        dirEl.textContent = dir + "/";
        els.artifactList.appendChild(dirEl);
      }
    }
    const el = document.createElement("div");
    el.className = "file";
    el.title = path;
    el.textContent = slash < 0 ? path : path.slice(slash + 1);
    el.addEventListener("click", () => openArtifact(path));
    el.dataset.path = path;
    els.artifactList.appendChild(el);
  }
  if (!state.activeArtifact || !result.files[state.activeArtifact]) {
    state.activeArtifact = paths.find((p) => p.endsWith(".agent.md")) ?? paths.find((p) => p.endsWith("SKILL.md")) ?? paths[0];
  }
  openArtifact(state.activeArtifact);
}

function openArtifact(path) {
  if (!state.result || !(path in state.result.files)) return;
  state.activeArtifact = path;
  els.artifactView.innerHTML = highlightFor(path, state.result.files[path]);
  for (const el of els.artifactList.querySelectorAll(".file")) {
    el.classList.toggle("active", el.dataset.path === path);
  }
}

/* ---------------------------------------------------------------- graph */

const KIND_COLORS = { plugin: "--kind-plugin", agent: "--kind-agent", profile: "--kind-profile", skill: "--kind-skill", capability: "--kind-capability", server: "--kind-interface", contextType: "--kind-capability", context: "--text-dim", rule: "--kind-native", command: "--kind-native", hook: "--kind-native", lsp: "--kind-native" };
const EDGE_KIND_COLOR = { embeds: "--text-dim", skill: "--kind-skill", binds: "--kind-skill", extends: "--kind-skill", ships: "--kind-plugin", capability: "--kind-capability", parameter: "--kind-capability", with: "--kind-agent", context: "--text-dim", server: "--kind-interface", contextType: "--kind-capability", rule: "--kind-native", command: "--kind-native", hook: "--kind-native" };

const shortName = (id) => {
  const noVersion = id.split("@")[0];
  return noVersion.slice(noVersion.lastIndexOf("/") + 1);
};

function renderGraph(result) {
  const graph = result.graph || { nodes: [], edges: [] };
  const nodes = graph.nodes.map((n) => ({ ...n }));
  const edges = graph.edges.map((e) => ({ ...e }));

  const byId = new Map(nodes.map((n) => [n.id, n]));
  const validEdges = edges.filter((e) => byId.has(e.from) && byId.has(e.to));

  // Longest-path layering, top-down. Guard against cycles (they are a
  // compile error, but the graph still renders while one exists).
  const depth = new Map(nodes.map((n) => [n.id, 0]));
  for (let pass = 0; pass < nodes.length + 1; pass++) {
    let changed = false;
    for (const e of validEdges) {
      const want = depth.get(e.from) + 1;
      if (want > depth.get(e.to) && want <= nodes.length) {
        depth.set(e.to, want);
        changed = true;
      }
    }
    if (!changed) break;
  }

  const KIND_ORDER = { plugin: 0, agent: 1, profile: 2, skill: 3, capability: 4, server: 5, context: 6, contextType: 7 };
  const rows = [];
  for (const node of nodes) {
    const d = depth.get(node.id);
    (rows[d] = rows[d] || []).push(node);
  }
  for (const row of rows) {
    if (row) row.sort((a, b) => (KIND_ORDER[a.kind] - KIND_ORDER[b.kind]) || a.id.localeCompare(b.id));
  }

  const css = getComputedStyle(document.documentElement);
  const color = (v) => css.getPropertyValue(v).trim();
  const charW = 7, padX = 12, nodeH = 40, rowGap = 104, gap = 22;
  const pos = new Map();
  let maxRowWidth = 0;
  rows.forEach((row) => {
    if (!row) return;
    const width = row.reduce((w, n) => w + Math.max(90, shortName(n.id).length * charW + padX * 2) + gap, -gap);
    maxRowWidth = Math.max(maxRowWidth, width);
  });
  const svgWidth = Math.max(560, maxRowWidth + 48);
  rows.forEach((row, d) => {
    if (!row) return;
    const rowWidth = row.reduce((w, n) => w + Math.max(90, shortName(n.id).length * charW + padX * 2) + gap, -gap);
    let x = (svgWidth - rowWidth) / 2;
    for (const node of row) {
      const w = Math.max(90, shortName(node.id).length * charW + padX * 2);
      pos.set(node.id, { x, y: 36 + d * rowGap, w, h: nodeH });
      x += w + gap;
    }
  });
  const svgHeight = 36 + rows.length * rowGap + 20;

  const svgParts = [];
  for (const e of validEdges) {
    const a = pos.get(e.from), b = pos.get(e.to);
    if (!a || !b) continue;
    const x1 = a.x + a.w / 2, y1 = a.y + a.h;
    const x2 = b.x + b.w / 2, y2 = b.y;
    const stroke = color(EDGE_KIND_COLOR[e.kind] || "--text-dim");
    svgParts.push(
      `<path class="edge ${e.kind}" stroke="${stroke}" d="M ${x1} ${y1} C ${x1} ${y1 + 46}, ${x2} ${y2 - 46}, ${x2} ${y2}">` +
      `<title>${escapeHTML(`${e.from} ${e.kind} ${e.to}`)}</title></path>`,
    );
  }
  for (const node of nodes) {
    const p = pos.get(node.id);
    const stroke = color(KIND_COLORS[node.kind] || "--text-dim");
    svgParts.push(
      `<g class="node"><title>${escapeHTML(node.id)}</title>` +
      `<rect x="${p.x}" y="${p.y}" width="${p.w}" height="${p.h}" stroke="${stroke}"></rect>` +
      `<text x="${p.x + p.w / 2}" y="${p.y + 17}" text-anchor="middle" class="kind-label">${node.kind}</text>` +
      `<text x="${p.x + p.w / 2}" y="${p.y + 31}" text-anchor="middle">${escapeHTML(shortName(node.id))}</text>` +
      `</g>`,
    );
  }
  els.graph.setAttribute("viewBox", `0 0 ${svgWidth} ${svgHeight}`);
  els.graph.setAttribute("width", svgWidth);
  els.graph.setAttribute("height", svgHeight);
  els.graph.innerHTML = svgParts.join("");

  els.graphLegend.innerHTML =
    Object.entries(KIND_COLORS).map(([kind, v]) =>
      `<span><span class="swatch" style="background:${color(v)}"></span>${kind}</span>`).join("") +
    `<span><span class="edge-sample" style="background:${color("--text-dim")}"></span>embeds</span>` +
    `<span><span class="edge-sample" style="background:${color("--kind-agent")}"></span>with (binds data)</span>`;
}

/* --------------------------------------------------------------- bundle */

function renderBundle(result) {
  const agents = result.agents || [];
  els.bundleAgent.innerHTML = "";
  for (const agent of agents) {
    const opt = document.createElement("option");
    opt.value = agent.id;
    opt.textContent = agent.id;
    els.bundleAgent.appendChild(opt);
  }
  if (!agents.some((a) => a.id === state.activeAgent)) {
    state.activeAgent = agents.length ? agents[0].id : null;
  }
  if (state.activeAgent) els.bundleAgent.value = state.activeAgent;
  showBundle();
}

function showBundle() {
  const agent = (state.result?.agents || []).find((a) => a.id === state.activeAgent);
  els.bundleView.innerHTML = agent ? highlightJSON(agent.bundle) : "";
}

/* ----------------------------------------------------------------- tabs */

function initTabs() {
  for (const button of document.querySelectorAll(".tabs button")) {
    button.addEventListener("click", () => {
      for (const b of document.querySelectorAll(".tabs button")) b.classList.toggle("active", b === button);
      for (const tab of document.querySelectorAll(".tab")) {
        tab.classList.toggle("active", tab.id === "tab-" + button.dataset.tab);
      }
    });
  }
  els.bundleAgent.addEventListener("change", () => {
    state.activeAgent = els.bundleAgent.value;
    showBundle();
  });
}

/* ------------------------------------------------------------- examples */

function loadExample(name) {
  const example = state.examples.find((e) => e.name === name);
  if (!example) return;
  state.example = name;
  state.files = new Map(Object.entries(example.files));
  state.activeArtifact = null;
  state.activeAgent = null;
  state.formFile = example.form || null;
  const paths = [...state.files.keys()].sort();
  openFile(paths.find((p) => p.includes("agent")) ?? paths[0]);
  scheduleCompile();
}

function initExamples() {
  for (const example of state.examples) {
    const opt = document.createElement("option");
    opt.value = example.name;
    opt.textContent = example.title;
    opt.title = example.description;
    els.exampleSelect.appendChild(opt);
  }
  els.exampleSelect.addEventListener("change", () => loadExample(els.exampleSelect.value));
  els.reset.addEventListener("click", () => loadExample(els.exampleSelect.value));
}

/* ----------------------------------------------------- instantiate form */

// The Instantiate tab is a form generated from a context type (ADR-0036):
// editing it rewrites one data document, which recompiles every instance
// that binds it. The form knows no TypeFerence rules; the compiler is still
// the validator.

// The form starts from the compiler's own reading of each data document
// (result.data), never from a hand-written parser, so every value the
// grammar allows, including multiline text, round-trips.

// packageNameFor reads the package name a directory's manifest declares.
function packageNameFor(dir) {
  const manifest = state.files.get(dir ? dir + "/typeference.tfer" : "typeference.tfer") || "";
  const m = manifest.match(/^name: (.*)$/m);
  return m ? m[1].trim() : null;
}

// locate maps an editable path to the package that owns it and the path
// within that package.
function locate(path) {
  const example = state.examples.find((e) => e.name === state.example);
  if (!example?.root) return { pkg: packageNameFor(""), rel: path };
  const slash = path.indexOf("/");
  return { pkg: packageNameFor(path.slice(0, slash)), rel: path.slice(slash + 1) };
}

function dataEntryFor(path, result) {
  const { pkg, rel } = locate(path);
  return (result.data || []).find((e) => e.package === pkg && e.path === rel) || null;
}

// scalarText writes a value as a plain scalar when that is unambiguous for
// a string field, and as a double-quoted string (JSON escapes, so line
// breaks become \n) otherwise.
function scalarText(value, type) {
  if (type === "boolean" || type === "integer") return value;
  const plainSafe = /^[A-Za-z0-9_./(][^#:\r\n]*$/.test(value) && value === value.trim() &&
    !["null", "~", "[]", "{}"].includes(value);
  return plainSafe ? value : JSON.stringify(value);
}

function dataDocument(typeRef, fields, values) {
  const lines = ["---", "contextType: " + typeRef, "values:"];
  for (const field of fields) {
    const value = values[field.name];
    if (field.type === "list<string>") {
      const items = (value || []).filter((v) => v !== "");
      if (!items.length) continue;
      lines.push(`  ${field.name}:`);
      for (const item of items) lines.push("    - " + scalarText(item, "string"));
      continue;
    }
    if (value === undefined || value === "") continue;
    lines.push(`  ${field.name}: ${scalarText(String(value), field.type)}`);
  }
  lines.push("---", "");
  return lines.join("\n");
}

function dataFiles() {
  return [...state.files.keys()].filter((p) => p.endsWith(".context.tfer") && /^contextType: /m.test(state.files.get(p))).sort();
}

// contextTypeRef reads the contextType reference as written, so the
// regenerated data document keeps the author's spelling of it.
function contextTypeRef(path) {
  const text = state.files.get(path) || "";
  return ((text.match(/^contextType: (.*)$/m) || [])[1] || "").trim();
}

function renderForm(result) {
  const files = dataFiles();
  els.formFile.innerHTML = "";
  for (const path of files) {
    const opt = document.createElement("option");
    opt.value = path;
    opt.textContent = path;
    els.formFile.appendChild(opt);
  }
  if (!files.includes(state.formFile)) state.formFile = files[0] || null;
  if (!state.formFile) {
    els.formFields.innerHTML = "";
    els.formPreview.textContent = "";
    els.formHint.textContent = "This example has no data documents to instantiate.";
    return;
  }
  els.formFile.value = state.formFile;
  const entry = dataEntryFor(state.formFile, result);
  const shape = entry ? (result.contextTypes || []).find((t) => t.id === entry.contextType) : null;
  const ref = contextTypeRef(state.formFile);
  if (!entry || !shape) {
    els.formFields.innerHTML = "";
    els.formFields.dataset.signature = "";
    els.formHint.textContent = "The compiler could not read this data document or its context type; fix the diagnostics first.";
    return;
  }
  // Re-render only when the shape or file changes, so typing keeps focus.
  const signature = state.formFile + "|" + JSON.stringify(shape);
  els.formPreview.innerHTML = highlightYAML(state.files.get(state.formFile) || "");
  if (els.formFields.dataset.signature === signature) return;
  els.formFields.dataset.signature = signature;
  els.formHint.innerHTML = `A form generated from <b>${escapeHTML(shape.displayName || shape.path)}</b>` +
    (shape.description ? ` — ${escapeHTML(shape.description)}` : "") +
    (shape.instanceName ? `. The <code>${escapeHTML(shape.instanceName)}</code> field names this team's skill instances.` : ".");
  const values = entry.values || {};
  els.formFields.innerHTML = "";
  for (const field of shape.fields) {
    const row = document.createElement("label");
    row.className = "form-row";
    const title = document.createElement("span");
    title.className = "form-label";
    title.textContent = (field.displayName || field.name) + (field.required ? " *" : "");
    row.appendChild(title);
    let input;
    if (field.choices && field.choices.length) {
      input = document.createElement("select");
      if (!field.required) input.appendChild(new Option(field.default ? `(default: ${field.default})` : "(none)", ""));
      for (const choice of field.choices) input.appendChild(new Option(choice, choice));
      input.value = values[field.name] ?? "";
    } else if (field.type === "boolean") {
      input = document.createElement("select");
      input.appendChild(new Option(field.default ? `(default: ${field.default})` : "(none)", ""));
      input.appendChild(new Option("true", "true"));
      input.appendChild(new Option("false", "false"));
      input.value = values[field.name] ?? "";
    } else if (field.type === "text" || field.type === "list<string>") {
      input = document.createElement("textarea");
      input.rows = 3;
      const value = values[field.name];
      input.value = Array.isArray(value) ? value.join("\n") : (value ?? "");
      if (field.type === "list<string>") input.placeholder = "one item per line";
    } else {
      input = document.createElement("input");
      input.type = field.type === "integer" ? "number" : "text";
      input.value = values[field.name] ?? "";
      if (field.default !== undefined) input.placeholder = `default: ${field.default}`;
    }
    input.dataset.field = field.name;
    input.addEventListener("input", () => updateFromForm(ref, shape));
    row.appendChild(input);
    if (field.description) {
      const help = document.createElement("span");
      help.className = "form-help";
      help.textContent = field.description;
      row.appendChild(help);
    }
    els.formFields.appendChild(row);
  }
}

function updateFromForm(ref, shape) {
  const values = {};
  for (const input of els.formFields.querySelectorAll("[data-field]")) {
    const field = shape.fields.find((f) => f.name === input.dataset.field);
    values[field.name] = field.type === "list<string>" ? input.value.split("\n").map((v) => v.trim()) : input.value;
  }
  const text = dataDocument(ref, shape.fields, values);
  state.files.set(state.formFile, text);
  if (state.activePath === state.formFile) {
    els.editor.value = text;
    refreshHighlight();
  }
  els.formPreview.innerHTML = highlightYAML(text);
  scheduleCompile();
}

function initForm() {
  els.formFile.addEventListener("change", () => {
    state.formFile = els.formFile.value;
    els.formFields.dataset.signature = "";
    if (state.result) renderForm(state.result);
  });
}

/* ---------------------------------------------------------------- share */

const b64url = {
  encode: (buf) => btoa(String.fromCharCode(...new Uint8Array(buf)))
    .replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""),
  decode: (s) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0)),
};

async function shareLink() {
  const payload = JSON.stringify({ example: state.example, files: Object.fromEntries(state.files) });
  const stream = new Blob([payload]).stream().pipeThrough(new CompressionStream("gzip"));
  const compressed = await new Response(stream).arrayBuffer();
  const url = new URL(location.href);
  url.hash = "code=" + b64url.encode(compressed);
  history.replaceState(null, "", url);
  try {
    await navigator.clipboard.writeText(url.href);
    toast("Link copied to clipboard");
  } catch {
    toast("Link is in the address bar");
  }
}

async function restoreFromHash() {
  const match = location.hash.match(/^#code=(.+)$/);
  if (!match) return false;
  try {
    const stream = new Blob([b64url.decode(match[1])]).stream()
      .pipeThrough(new DecompressionStream("gzip"));
    const payload = JSON.parse(await new Response(stream).text());
    state.example = payload.example ?? state.examples[0]?.name;
    if (state.examples.some((e) => e.name === state.example)) {
      els.exampleSelect.value = state.example;
    }
    state.files = new Map(Object.entries(payload.files));
    state.formFile = state.examples.find((e) => e.name === state.example)?.form || null;
    const paths = [...state.files.keys()].sort();
    openFile(paths.find((p) => p.includes("agent")) ?? paths[0]);
    return true;
  } catch {
    return false;
  }
}

/* ----------------------------------------------------------------- boot */

async function loadCompiler() {
  const go = new Go();
  const response = fetch("typeference.wasm");
  let instance;
  try {
    ({ instance } = await WebAssembly.instantiateStreaming(response, go.importObject));
  } catch {
    // Some static hosts serve wasm with a generic MIME type.
    const bytes = await (await fetch("typeference.wasm")).arrayBuffer();
    ({ instance } = await WebAssembly.instantiate(bytes, go.importObject));
  }
  go.run(instance); // resolves only on exit; main blocks forever
  for (let i = 0; i < 200 && !globalThis.TypeFerence; i++) {
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  if (!globalThis.TypeFerence) throw new Error("compiler did not initialize");
}

async function boot() {
  initTheme();
  initEditor();
  initFilePane();
  initTabs();
  initForm();
  els.share.addEventListener("click", shareLink);

  try {
    const [examples] = await Promise.all([
      fetch("examples.json").then((r) => {
        if (!r.ok) throw new Error(`examples.json: HTTP ${r.status}`);
        return r.json();
      }),
      loadCompiler(),
    ]);
    state.examples = examples.examples;
  } catch (err) {
    setStatus("error", `failed to load: ${escapeHTML(String(err))}`);
    return;
  }

  initExamples();
  state.ready = true;
  const restored = await restoreFromHash();
  if (!restored) loadExample(state.examples[0].name);
  else scheduleCompile();
}

boot();
