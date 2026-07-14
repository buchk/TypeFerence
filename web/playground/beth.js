// BETH operator console (ADR-0011). The browser packs cells with the real
// eval.Pack (wasm), the human operator collects responses by copy/paste from
// real hosts, the console exports a byte-faithful run directory as .tar.gz,
// scoring happens locally with the CLI (keys stay in the operator's
// terminal), and the resulting scorecard.json renders here. This file makes
// no network requests.
"use strict";

const beth = {
  run: null,      // {files: {path: content}, manifest, cells: [{path, scenarioId, surface, prompt}]}
  stale: false,   // source edited after packing
  exampleName: null,
};

/* ---------------------------------------------------------------- pack */

// Called by app.js: on example switch (reset) and on every successful
// compile after a pack (stale marker).
function bethReset(exampleName) {
  beth.run = null;
  beth.stale = false;
  beth.exampleName = exampleName;
  bethRenderScenarioList();
  document.getElementById("beth-step-collect").hidden = true;
  document.getElementById("beth-step-export").hidden = true;
  document.getElementById("beth-pack-status").textContent = "";
  document.getElementById("beth-cells").innerHTML = "";
}

function bethMarkStale() {
  if (!beth.run || beth.stale) return;
  beth.stale = true;
  document.getElementById("beth-pack-status").textContent =
    "source changed since this pack — repack to stay faithful (pasted responses are kept until then)";
}

function bethScenarios() {
  const example = state.examples.find((e) => e.name === state.example);
  return (example && example.scenarios) || {};
}

function bethRenderScenarioList() {
  const names = Object.keys(bethScenarios()).sort();
  document.getElementById("beth-scenarios").textContent = names.length
    ? `Scenarios shipped with this example: ${names.join(", ")}`
    : "This example ships no scenarios.";
  document.getElementById("beth-pack-btn").disabled = names.length === 0;
}

function bethPack() {
  const scenarios = bethScenarios();
  const status = document.getElementById("beth-pack-status");
  const result = globalThis.TypeFerence.pack({
    files: Object.fromEntries(state.files),
    sourceName: state.example || "src",
    scenarios,
  });
  if (!result.ok) {
    status.textContent = result.error;
    return;
  }
  const manifest = JSON.parse(result.files["manifest.json"]);
  const cells = manifest.cells.map((cell) => ({
    path: cell.path,
    scenarioId: cell.scenario,
    surface: cell.surface,
    prompt: result.files[cell.path + "/PROMPT.txt"] ?? "",
  }));
  beth.run = { files: result.files, manifest, cells };
  beth.stale = false;
  status.textContent =
    `packed ${cells.length} cells (${manifest.cells.length / manifest.surfaces.length} scenarios × ` +
    `${manifest.surfaces.length} surfaces) · source ${manifest.sourceDigest.slice(0, 19)}…`;
  bethRenderCells();
  document.getElementById("beth-step-collect").hidden = false;
  document.getElementById("beth-step-export").hidden = false;
  bethRefreshExport();
}

/* -------------------------------------------------------------- collect */

function bethRenderCells() {
  const container = document.getElementById("beth-cells");
  container.innerHTML = "";
  const byScenario = new Map();
  for (const cell of beth.run.cells) {
    if (!byScenario.has(cell.scenarioId)) byScenario.set(cell.scenarioId, []);
    byScenario.get(cell.scenarioId).push(cell);
  }
  for (const [scenarioId, cells] of byScenario) {
    const group = document.createElement("div");
    group.className = "beth-scenario";
    const heading = document.createElement("h4");
    heading.textContent = scenarioId;
    group.appendChild(heading);
    const task = document.createElement("details");
    task.innerHTML = "<summary>Prompt (identical for every surface)</summary>";
    const pre = document.createElement("pre");
    pre.textContent = cells[0].prompt;
    task.appendChild(pre);
    group.appendChild(task);
    const grid = document.createElement("div");
    grid.className = "beth-cell-grid";
    for (const cell of cells) {
      const card = document.createElement("div");
      card.className = "beth-cell";
      card.dataset.path = cell.path;
      card.innerHTML = `
        <div class="cell-head">
          <span class="surface-chip">${escapeHTML(cell.surface)}</span>
          <button class="ghost copy-prompt">Copy prompt</button>
          <span class="collected" hidden>✓</span>
        </div>
        <textarea rows="4" placeholder="Paste the host's final response…" aria-label="Response for ${escapeHTML(cell.path)}"></textarea>
        <div class="cell-meta">
          <input class="host" placeholder="host (e.g. claude-code 3.7)" aria-label="Host">
          <input class="model" placeholder="model" aria-label="Model">
        </div>`;
      card.querySelector(".copy-prompt").addEventListener("click", async () => {
        await navigator.clipboard.writeText(cell.prompt);
        toast(`Prompt copied — run it on a ${cell.surface} host, then paste the response`);
      });
      card.querySelector("textarea").addEventListener("input", () => {
        card.querySelector(".collected").hidden = !card.querySelector("textarea").value.trim();
        bethRefreshExport();
      });
      grid.appendChild(card);
    }
    group.appendChild(grid);
    container.appendChild(group);
  }
}

function bethCollectedCount() {
  let n = 0;
  for (const card of document.querySelectorAll(".beth-cell")) {
    if (card.querySelector("textarea").value.trim()) n++;
  }
  return n;
}

function bethRefreshExport() {
  if (!beth.run) return;
  const collected = bethCollectedCount();
  const total = beth.run.cells.length;
  document.getElementById("beth-export-status").textContent =
    `${collected}/${total} cells collected — export includes every cell either way; ` +
    `uncollected cells score as no-response`;
  const dirName = `beth-run-${beth.exampleName || "playground"}`;
  document.getElementById("beth-score-cmd").textContent =
    `tar -xzf ${dirName}.tar.gz\n` +
    `typeference equivalence score ${dirName}          # offline: emits judge payloads\n` +
    `typeference equivalence score ${dirName} --live   # judges via ANTHROPIC_API_KEY (your terminal, your key)`;
}

/* --------------------------------------------------------------- export */

// Minimal ustar writer. mtime is fixed at 0 so identical runs produce
// byte-identical archives — the house rule applies to exports too.
function tarArchive(files) {
  const encoder = new TextEncoder();
  const blocks = [];
  const writeString = (buf, at, max, value) => {
    const bytes = encoder.encode(value);
    if (bytes.length > max) throw new Error(`tar field overflow: ${value}`);
    buf.set(bytes, at);
  };
  const writeOctal = (buf, at, width, value) => {
    writeString(buf, at, width - 1, value.toString(8).padStart(width - 1, "0"));
  };
  for (const [path, content] of files) {
    const data = typeof content === "string" ? encoder.encode(content) : content;
    let name = path, prefix = "";
    if (encoder.encode(name).length > 100) {
      const cut = path.slice(0, 155).lastIndexOf("/");
      if (cut <= 0) throw new Error(`path too long for ustar: ${path}`);
      prefix = path.slice(0, cut);
      name = path.slice(cut + 1);
      if (encoder.encode(name).length > 100 || encoder.encode(prefix).length > 155) {
        throw new Error(`path too long for ustar: ${path}`);
      }
    }
    const header = new Uint8Array(512);
    writeString(header, 0, 100, name);
    writeOctal(header, 100, 8, 0o644); // mode
    writeOctal(header, 108, 8, 0);     // uid
    writeOctal(header, 116, 8, 0);     // gid
    writeOctal(header, 124, 12, data.length);
    writeOctal(header, 136, 12, 0);    // mtime: fixed for determinism
    header.fill(0x20, 148, 156);       // checksum field counts as spaces
    header[156] = 0x30;                // typeflag '0': regular file
    writeString(header, 257, 6, "ustar");
    header[263] = 0x30; header[264] = 0x30; // version "00"
    writeString(header, 345, 155, prefix);
    let sum = 0;
    for (const byte of header) sum += byte;
    writeString(header, 148, 7, sum.toString(8).padStart(6, "0") + "\0");
    blocks.push(header, data);
    const overhang = data.length % 512;
    if (overhang) blocks.push(new Uint8Array(512 - overhang));
  }
  blocks.push(new Uint8Array(1024)); // end-of-archive
  return new Blob(blocks, { type: "application/x-tar" });
}

// Assembles the run directory (pack output plus collected responses) and
// returns { name, blob } with the gzipped tar.
async function bethBuildArchive() {
  const dirName = `beth-run-${beth.exampleName || "playground"}`;
  const files = new Map();
  for (const [path, content] of Object.entries(beth.run.files).sort((a, b) => (a[0] < b[0] ? -1 : 1))) {
    files.set(`${dirName}/${path}`, content);
  }
  for (const card of document.querySelectorAll(".beth-cell")) {
    const response = card.querySelector("textarea").value;
    if (!response.trim()) continue;
    const base = `${dirName}/${card.dataset.path}`;
    files.set(`${base}/response.md`, response.endsWith("\n") ? response : response + "\n");
    const host = card.querySelector(".host").value.trim();
    const model = card.querySelector(".model").value.trim();
    if (host || model) {
      const runtime = {};
      if (host) runtime.host = host;
      if (model) runtime.model = model;
      files.set(`${base}/runtime.json`, JSON.stringify(runtime) + "\n");
    }
  }
  const blob = await new Response(
    tarArchive(files).stream().pipeThrough(new CompressionStream("gzip")),
  ).blob();
  return { name: `${dirName}.tar.gz`, blob, fileCount: files.size };
}

async function bethExport() {
  if (!beth.run) return;
  const archive = await bethBuildArchive();
  const url = URL.createObjectURL(archive.blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = archive.name;
  anchor.click();
  URL.revokeObjectURL(url);
  toast(`Exported ${archive.fileCount} files — score it locally`);
}

/* ------------------------------------------------------------ scorecard */

function bethRenderScorecard(card) {
  const el = document.getElementById("beth-scorecard");
  if (typeof card !== "object" || card === null || !Array.isArray(card.adherence)) {
    el.innerHTML = `<p class="beth-error">Not a BETH scorecard.json (missing adherence array).</p>`;
    return;
  }
  const parts = [];
  parts.push(`<div class="beth-banner ${card.passed ? "pass" : "fail"}">` +
    `${card.passed ? "PASSED" : "NOT PASSED"} — ${card.cells.judged}/${card.cells.total} cells judged` +
    `${card.judgeModel ? ` · judge: ${escapeHTML(card.judgeModel)}` : ""}</div>`);

  parts.push(`<div class="beth-metrics">`);
  for (const row of card.adherence) {
    const ratio = row.judged ? row.passed / row.judged : 0;
    parts.push(`<div class="beth-metric"><span class="label">${escapeHTML(row.surface)}</span>` +
      `<span class="bar"><span class="fill" style="width:${Math.round(ratio * 100)}%"></span></span>` +
      `<span class="value">${row.passed}/${row.judged}</span></div>`);
  }
  const agreement = card.agreement || { agreed: 0, comparable: 0 };
  parts.push(`<div class="beth-metric"><span class="label">agreement</span>` +
    `<span class="bar"><span class="fill" style="width:${agreement.comparable ? Math.round(agreement.agreed / agreement.comparable * 100) : 0}%"></span></span>` +
    `<span class="value">${agreement.agreed}/${agreement.comparable}</span></div>`);
  parts.push(`</div>`);

  for (const scenario of card.scenarios || []) {
    const surfaces = (scenario.cells || []).map((c) => c.surface);
    parts.push(`<div class="beth-matrix"><h4>${escapeHTML(scenario.id)}</h4><table><thead><tr><th></th>`);
    for (const cell of scenario.cells || []) {
      parts.push(`<th title="${escapeHTML(cell.status)}${cell.host ? " · " + escapeHTML(cell.host) : ""}">` +
        `${escapeHTML(cell.surface)}${cell.status !== "judged" ? "*" : ""}</th>`);
    }
    parts.push(`</tr></thead><tbody>`);
    for (const item of scenario.rubric || []) {
      parts.push(`<tr><td class="rubric-id">${escapeHTML(item.id)}</td>`);
      const bySurface = new Map((item.verdicts || []).map((v) => [v.surface, v]));
      for (const surface of surfaces) {
        const verdict = bySurface.get(surface);
        if (!verdict) parts.push(`<td class="na">—</td>`);
        else parts.push(`<td class="${verdict.passed ? "pass" : "fail"}" title="${escapeHTML(verdict.reasoning || "")}">` +
          `${verdict.passed ? "✓" : "✗"}</td>`);
      }
      parts.push(`</tr>`);
    }
    parts.push(`</tbody></table></div>`);
  }

  if ((card.divergences || []).length) {
    parts.push(`<div class="beth-divergences"><h4>Divergences</h4>`);
    for (const divergence of card.divergences) {
      parts.push(`<div class="divergence"><b>${escapeHTML(divergence.scenario)}</b> · ` +
        `${escapeHTML(divergence.rubricItem)}<ul>`);
      for (const verdict of divergence.verdicts || []) {
        parts.push(`<li><span class="${verdict.passed ? "pass" : "fail"}">${verdict.passed ? "✓" : "✗"} ` +
          `${escapeHTML(verdict.surface)}</span> — ${escapeHTML(verdict.reasoning || "")}</li>`);
      }
      parts.push(`</ul></div>`);
    }
    parts.push(`</div>`);
  } else {
    parts.push(`<p class="hint">No divergences among comparable rubric items.</p>`);
  }
  parts.push(`<p class="hint">A scorecard is one observation per surface at one point in time, not a proof of behavioral equivalence (ADR-0009). Cells marked * were not judged.</p>`);
  el.innerHTML = parts.join("");
}

function bethReadScorecard(file) {
  const reader = new FileReader();
  reader.onload = () => {
    try {
      bethRenderScorecard(JSON.parse(reader.result));
    } catch {
      document.getElementById("beth-scorecard").innerHTML =
        `<p class="beth-error">Could not parse that file as JSON.</p>`;
    }
  };
  reader.readAsText(file);
}

/* ----------------------------------------------------------------- init */

function initBeth() {
  document.getElementById("beth-pack-btn").addEventListener("click", bethPack);
  document.getElementById("beth-export-btn").addEventListener("click", bethExport);
  const drop = document.getElementById("beth-drop");
  const fileInput = document.getElementById("beth-scorecard-file");
  drop.addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", () => {
    if (fileInput.files[0]) bethReadScorecard(fileInput.files[0]);
  });
  drop.addEventListener("dragover", (e) => { e.preventDefault(); drop.classList.add("over"); });
  drop.addEventListener("dragleave", () => drop.classList.remove("over"));
  drop.addEventListener("drop", (e) => {
    e.preventDefault();
    drop.classList.remove("over");
    if (e.dataTransfer.files[0]) bethReadScorecard(e.dataTransfer.files[0]);
  });
}
