/* Setup wizard (ADR-0028): collects a small AnswerSet, runs the same Go
 * generator as `typeference init` through the wasm bridge, and offers the
 * exit ramp: download the tree plus answers.json, or copy the exact
 * `typeference init --answers answers.json --verify <digest>` command.
 *
 * Presentation layer only. All semantics come from the generator/compiler;
 * this file never invents TypeFerence content. */

function initWizard({ getTypeFerence, loadFiles }) {
  const els = {
    open: document.getElementById("wizard-open"),
    panel: document.getElementById("wizard-panel"),
    close: document.getElementById("wizard-close"),
    org: document.getElementById("wiz-org"),
    version: document.getElementById("wiz-version"),
    norms: document.getElementById("wiz-norms"),
    level: document.getElementById("wiz-level"),
    agent: document.getElementById("wiz-agent"),
    run: document.getElementById("wiz-run"),
    result: document.getElementById("wiz-result"),
    downloadTree: document.getElementById("wiz-download-tree"),
    downloadAnswers: document.getElementById("wiz-download-answers"),
    copyCmd: document.getElementById("wiz-copy-cmd"),
  };
  if (!els.open) return;

  let lastResult = null;

  const show = () => {
    els.panel.hidden = false;
    els.open.disabled = true;
  };
  const hide = () => {
    els.panel.hidden = true;
    els.open.disabled = false;
  };
  els.open.addEventListener("click", show);
  els.close.addEventListener("click", hide);

  function collectAnswerSet() {
    const org = (els.org.value || "").trim().toLowerCase().replace(/[^a-z0-9-]/g, "") || "my-org";
    const version = (els.version.value || "1.0.0").trim();
    const normTexts = els.norms.value
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean)
      .slice(0, 5);
    const levels = (els.level.value || "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .slice(0, 3)
      .map((name) => ({ name }));
    const agent = { name: (els.agent.value || "my-agent").trim() };
    return {
      schemaVersion: 1,
      organization: { name: org, version },
      norms: normTexts.map((text) => ({ text })),
      levels,
      agent,
      targets: { hosts: ["neutral"] },
    };
  }

  function slug(text) {
    return (
      text
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "")
        .slice(0, 40)
        .replace(/-+$/g, "") || "norm"
    );
  }

  async function runWizard() {
    const tf = getTypeFerence();
    if (!tf || typeof tf.scaffold !== "function") {
      renderError("The compiler bridge is still loading. Try again in a moment.");
      return;
    }
    const answerSet = collectAnswerSet();
    if (answerSet.norms.length === 0) {
      renderError("Write at least one norm statement — that's the whole point.");
      return;
    }
    let response;
    try {
      response = tf.scaffold({ answerSet: JSON.stringify(answerSet, null, 2) });
    } catch (err) {
      renderError(String(err));
      return;
    }
    if (!response || !response.ok) {
      renderError((response && response.error) || "Generation failed.");
      return;
    }
    lastResult = { response, answerSet };
    renderSuccess(response);
  }

  function renderError(message) {
    els.result.hidden = false;
    els.result.innerHTML = "";
    const p = document.createElement("p");
    p.className = "wiz-error";
    p.textContent = message;
    els.result.appendChild(p);
  }

  function renderSuccess(response) {
    const { files, digest, manifest } = response;
    const paths = Object.keys(files).sort();
    const normPaths = paths.filter((p) => p.startsWith("context/norm-"));

    els.result.hidden = false;
    els.result.innerHTML = "";

    const summary = document.createElement("p");
    summary.innerHTML =
      `<strong>${paths.length} files</strong> generated · digest <code>${digest.slice(0, 23)}…</code>` +
      `<br>Generator ${manifest.generatorVersion} · answer schema ${manifest.schemaVersion}`;
    els.result.appendChild(summary);

    // The teaching moment: your prose became typed context resources.
    if (normPaths.length > 0) {
      const teach = document.createElement("p");
      teach.className = "wiz-teach";
      teach.textContent =
        "Your norm statements are now context resources of the declared type " +
        "context-types/norms — ordinary prose carried by a named, versioned, " +
        "addressable type. That is the entire v5 idea.";
      els.result.appendChild(teach);
    }

    const treeList = document.createElement("ul");
    treeList.className = "wiz-tree";
    for (const p of paths) {
      const li = document.createElement("li");
      li.textContent = p + (normPaths.includes(p) ? "  ← from your answer" : "");
      treeList.appendChild(li);
    }
    els.result.appendChild(treeList);

    els.downloadTree.hidden = false;
    els.downloadAnswers.hidden = false;
    els.copyCmd.hidden = false;

    els.downloadTree.onclick = () => downloadTar(files);
    els.downloadAnswers.onclick = () => {
      const blob = new Blob([JSON.stringify(lastResult.answerSet, null, 2)], {
        type: "application/json",
      });
      triggerDownload(blob, "answers.json");
    };
    els.copyCmd.onclick = () => {
      navigator.clipboard
        .writeText(
          `typeference init --answers answers.json --verify ${digest}`
        )
        .then(() => {
          els.copyCmd.textContent = "Copied ✓";
          setTimeout(() => (els.copyCmd.textContent = "Copy local command"), 1500);
        });
    };

    // Load the generated suite into the editor so it compiles live.
    const loadBtn = document.createElement("button");
    loadBtn.className = "ghost";
    loadBtn.textContent = "Open generated suite in the editor";
    loadBtn.addEventListener("click", () => {
      loadFiles(files);
      hide();
    });
    els.result.appendChild(loadBtn);
  }

  /* Minimal ustar writer + native gzip: same approach as the equivalence
   * console export. Fixed mtime keeps archives deterministic. */
  function downloadTar(files) {
    const entries = Object.entries(files).sort(([a], [b]) => (a < b ? -1 : 1));
    const blocks = [];
    let header;
    for (const [path, content] of entries) {
      header = new Uint8Array(512);
      writeOctal(header, 100, 124, 8); // size placeholder replaced below
      header.fill(0);
      setString(header, 0, path.replace(/^\/+/, ""));
      setString(header, 257, "ustar");
      setString(header, 263, "00");
      setOctal(header, 100, content.length, 11);
      setOctal(header, 136, 0, 11); // mtime 0: determinism over realism
      let sum = 0;
      for (let i = 0; i < 512; i++) sum += i >= 148 && i < 156 ? 32 : header[i];
      setOctal(header, 148, sum, 7);
      blocks.push(header);
      const data = new TextEncoder().encode(content);
      blocks.push(data);
      const pad = (512 - (data.length % 512)) % 512;
      if (pad) blocks.push(new Uint8Array(pad));
    }
    blocks.push(new Uint8Array(1024)); // end-of-archive
    const tar = concat(blocks);
    const gz = new CompressionStream("gzip");
    const stream = new Blob([tar]).stream().pipeThrough(gz);
    new Response(stream).blob().then((blob) => triggerDownload(blob, "starter-suite.tar.gz"));
  }

  function setString(buf, offset, value) {
    const bytes = new TextEncoder().encode(value);
    buf.set(bytes.subarray(0, Math.min(bytes.length, 100)), offset);
  }
  function setOctal(buf, offset, value, length) {
    const str = value.toString(8).padStart(length - 1, "0") + "\0";
    buf.set(new TextEncoder().encode(str), offset);
  }
  function writeOctal() {} // reserved; sizes are set via setOctal above
  function concat(arrays) {
    const total = arrays.reduce((n, a) => n + a.length, 0);
    const out = new Uint8Array(total);
    let at = 0;
    for (const a of arrays) {
      out.set(a, at);
      at += a.length;
    }
    return out;
  }
  function triggerDownload(blob, filename) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 5000);
  }

  els.run.addEventListener("click", runWizard);
}
