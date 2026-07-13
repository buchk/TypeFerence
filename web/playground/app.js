// TypeFerence Playground. Vanilla JS, no dependencies: the page loads the
// real Go compiler as WebAssembly (typeference.wasm), gives it an in-memory
// filesystem (memfs.js), and recompiles on every edit. Nothing leaves the tab.
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
};

const $ = (id) => document.getElementById(id);
const els = {
  exampleSelect: $("example-select"),
  reset: $("reset-btn"),
  emitArd: $("emit-ard"),
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
  if (path.endsWith(".yaml") || path.endsWith(".yml")) return highlightYAML(text);
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
    const path = prompt("New file path (e.g. skills/triage.skill.yaml):");
    if (!path || state.files.has(path)) return;
    const clean = path.replace(/^\/+/, "");
    const template = clean.endsWith(".md")
      ? "# Notes\n"
      : "schemaVersion: 3\nkind: skill\nid: acme/skills/new-skill@1.0.0\nbinds: acme/capabilities/change-me@1.0.0\ndescription: Describe the implementation.\ninstructions: |\n  Say what this skill does.\n";
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

function compileNow() {
  if (!state.ready) return;
  const started = performance.now();
  const example = state.examples.find((e) => e.name === state.example);
  const request = {
    files: Object.fromEntries(state.files),
    target: "all",
    emitArd: els.emitArd.checked,
    publisherDomain: example?.publisherDomain || "playground.example",
    sourceName: state.example || "src",
  };
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
    return;
  }

  els.diagnostics.hidden = true;
  els.outputPane.classList.remove("stale");
  state.result = result;
  const fileCount = Object.keys(result.files).length;
  const digest = result.hash.replace(/^sha256:/, "").slice(0, 16);
  const emitted = result.agents.filter((a) => a.emit).length;
  setStatus("ok",
    `${emitted} agent${emitted === 1 ? "" : "s"} · ${fileCount} artifacts · ` +
    `<span class="digest">SHA-256 <b>${digest}…</b></span> · ${elapsed} ms · ` +
    `same bytes the CLI produces`);
  renderArtifacts(result);
  renderGraph(result);
  renderBundle(result);
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
    state.activeArtifact = paths.find((p) => p.endsWith("AGENTS.md")) ?? paths[0];
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

const KIND_COLORS = { agent: "--kind-agent", profile: "--kind-profile", skill: "--kind-skill", capability: "--kind-capability", interface: "--kind-interface" };
const EDGE_KIND_COLOR = { embeds: "--text-dim", satisfies: "--kind-interface", skill: "--kind-skill", binds: "--kind-skill", capability: "--kind-capability", requires: "--kind-capability" };

const shortName = (id) => {
  const noVersion = id.split("@")[0];
  return noVersion.slice(noVersion.lastIndexOf("/") + 1);
};

function renderGraph(result) {
  const graph = result.graph || { nodes: [], edges: [] };
  const nodes = graph.nodes.map((n) => ({ ...n }));
  const edges = graph.edges.map((e) => ({ ...e }));
  // Structural interface satisfaction is computed, not declared; show it.
  for (const agent of result.agents || []) {
    for (const iface of agent.satisfies || []) {
      edges.push({ from: agent.id, to: iface, kind: "satisfies" });
    }
  }

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

  const KIND_ORDER = { agent: 0, profile: 1, interface: 2, skill: 3, capability: 4 };
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
    `<span><span class="edge-sample" style="background:repeating-linear-gradient(90deg,${color("--kind-interface")} 0 5px,transparent 5px 9px)"></span>satisfies (structural)</span>`;
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
  els.emitArd.addEventListener("change", scheduleCompile);
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
