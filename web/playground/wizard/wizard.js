/* Setup wizard page (ADR-0008): collects a small AnswerSet, runs the same Go
 * generator as `typeference init` through the wasm bridge, and offers the
 * exit ramp: download the tree plus answers.json, or copy the exact
 * `typeference init --answers answers.json --verify <digest>` command.
 *
 * Presentation layer only. All semantics come from the generator; this file
 * never invents TypeFerence content. */

(function () {
  var els = {
    status: document.getElementById("wiz-status"),
    run: document.getElementById("wiz-run"),
    org: document.getElementById("wiz-org"),
    version: document.getElementById("wiz-version"),
    norms: document.getElementById("wiz-norms"),
    level: document.getElementById("wiz-level"),
    agent: document.getElementById("wiz-agent"),
    result: document.getElementById("wiz-result"),
    downloadTree: document.getElementById("wiz-download-tree"),
    downloadAnswers: document.getElementById("wiz-download-answers"),
    copyCmd: document.getElementById("wiz-copy-cmd"),
    exitButtons: document.getElementById("wiz-exit-buttons")
  };

  var lastResult = null;

  function collectAnswerSet() {
    var org = (els.org.value || "").trim().toLowerCase().replace(/[^a-z0-9-]/g, "") || "my-org";
    var version = (els.version.value || "1.0.0").trim() || "1.0.0";
    var normTexts = els.norms.value.split("\n").map(function (l) { return l.trim(); }).filter(Boolean).slice(0, 5);
    var levels = (els.level.value || "").split(",").map(function (s) { return s.trim(); }).filter(Boolean).slice(0, 3)
      .map(function (name) { return { name: name }; });
    return {
      schemaVersion: 1,
      organization: { name: org, version: version },
      norms: normTexts.map(function (text) { return { text: text }; }),
      levels: levels,
      agent: { name: (els.agent.value || "my-agent").trim() || "my-agent" },
      targets: { hosts: ["neutral"] }
    };
  }

  function renderError(message) {
    els.result.hidden = false;
    els.result.innerHTML = "";
    var p = document.createElement("p");
    p.className = "wiz-error";
    p.textContent = message;
    els.result.appendChild(p);
  }

  async function loadCompiler() {
    if (!window.TypeFerence || typeof window.TypeFerence.scaffold !== "function") {
      try {
        var go = new Go();
        var result = await WebAssembly.instantiateStreaming(
          fetch("../typeference.wasm"), go.importObject);
        go.run(result.instance);
      } catch (err) {
        els.status.textContent = "Compiler failed to load: " + err;
        els.run.disabled = true;
        return;
      }
    }
    if (window.TypeFerence && typeof window.TypeFerence.scaffold === "function") {
      els.status.textContent = "Compiler ready.";
      els.run.disabled = false;
    } else {
      els.status.textContent = "This compiler build does not include the wizard generator.";
      els.run.disabled = true;
    }
  }

  function runWizard() {
    if (!window.TypeFerence || typeof window.TypeFerence.scaffold !== "function") {
      renderError("The compiler bridge is not ready.");
      return;
    }
    var answerSet = collectAnswerSet();
    if (answerSet.norms.length === 0) {
      renderError("Write at least one norm statement — that is the whole point.");
      return;
    }
    var response;
    try {
      response = window.TypeFerence.scaffold({
        answerSet: JSON.stringify(answerSet, null, 2)
      });
    } catch (err) {
      renderError(String(err));
      return;
    }
    if (!response || !response.ok) {
      renderError((response && response.error) || "Generation failed.");
      return;
    }
    lastResult = { response: response, answerSet: answerSet };
    renderSuccess(response);
  }

  function renderSuccess(response) {
    var files = {};
    var raw = response.files;
    // syscall/js may surface Go maps as ES2015 Map objects; normalize.
    if (raw instanceof Map) {
      raw.forEach(function (v, k) { files[k] = v; });
    } else {
      Object.keys(raw).forEach(function (k) { files[k] = raw[k]; });
    }
    var digest = response.digest;
    var manifest = response.manifest || {};
    var paths = Object.keys(files).sort();
    var normPaths = paths.filter(function (p) { return p.indexOf("context/norm-") === 0; });

    els.result.hidden = false;
    els.result.innerHTML = "";

    var summary = document.createElement("p");
    summary.innerHTML = "<strong>" + paths.length + " files</strong> generated &middot; digest <code>" +
      digest.slice(0, 23) + "&hellip;</code><br>Generator " +
      (manifest.generatorVersion || "?") + " &middot; answer schema " +
      (manifest.schemaVersion != null ? manifest.schemaVersion : "?");
    els.result.appendChild(summary);

    if (normPaths.length > 0) {
      var teach = document.createElement("p");
      teach.className = "wiz-teach";
      teach.textContent =
        "Your norm statements are now context documents: ordinary prose with a " +
        "path, a version, and the built-in text type, held by the base profile " +
        "that every level embeds. Nothing that reaches the model is untyped.";
      els.result.appendChild(teach);
    }

    var treeList = document.createElement("ul");
    treeList.className = "wiz-tree";
    paths.forEach(function (p) {
      var li = document.createElement("li");
      li.textContent = p + (normPaths.indexOf(p) >= 0 ? " <-" : "");
      treeList.appendChild(li);
    });
    els.result.appendChild(treeList);

    var note = document.createElement("p");
    note.className = "hint";
    note.textContent = "<- marks files created directly from your answer.";
    els.result.appendChild(note);

    els.exitButtons.hidden = false;
    els.downloadTree.hidden = false;
    els.downloadAnswers.hidden = false;
    els.copyCmd.hidden = false;

    els.downloadTree.onclick = function () { downloadTar(files); };
    els.downloadAnswers.onclick = function () {
      var blob = new Blob([JSON.stringify(lastResult.answerSet, null, 2)],
        { type: "application/json" });
      triggerDownload(blob, "answers.json");
    };
    els.copyCmd.onclick = function () {
      navigator.clipboard.writeText(
        "typeference init --answers answers.json --verify " + digest
      ).then(function () {
        els.copyCmd.textContent = "Copied!";
        setTimeout(function () { els.copyCmd.textContent = "Copy local command"; }, 1500);
      });
    };
  }

  /* Minimal ustar writer + native gzip: fixed mtime keeps archives
   * deterministic, matching what the equivalence console exports. */
  function downloadTar(files) {
    var entries = Object.entries(files).sort(function (a, b) { return a[0] < b[0] ? -1 : 1; });
    var blocks = [];
    entries.forEach(function (entry) {
      var path = entry[0], content = entry[1];
      var header = new Uint8Array(512);
      setString(header, 0, path.replace(/^\/+/, ""));
      setString(header, 257, "ustar");
      setString(header, 263, "00");
      setOctal(header, 100, content.length, 11);
      setOctal(header, 136, 0, 11); /* mtime 0: determinism over realism */
      var sum = 0;
      for (var i = 0; i < 512; i++) sum += (i >= 148 && i < 156) ? 32 : header[i];
      setOctal(header, 148, sum, 7);
      blocks.push(header);
      var data = new TextEncoder().encode(content);
      blocks.push(data);
      var pad = (512 - (data.length % 512)) % 512;
      if (pad) blocks.push(new Uint8Array(pad));
    });
    blocks.push(new Uint8Array(1024));
    var tar = concat(blocks);
    var stream = new Blob([tar]).stream().pipeThrough(new CompressionStream("gzip"));
    new Response(stream).blob().then(function (blob) {
      triggerDownload(blob, "starter-suite.tar.gz");
    });
  }

  function setString(buf, offset, value) {
    var bytes = new TextEncoder().encode(value);
    buf.set(bytes.subarray(0, Math.min(bytes.length, 100)), offset);
  }
  function setOctal(buf, offset, value, length) {
    var str = value.toString(8).padStart(length - 1, "0") + "\0";
    buf.set(new TextEncoder().encode(str), offset);
  }
  function concat(arrays) {
    var total = arrays.reduce(function (n, a) { return n + a.length; }, 0);
    var out = new Uint8Array(total);
    var at = 0;
    arrays.forEach(function (a) { out.set(a, at); at += a.length; });
    return out;
  }
  function triggerDownload(blob, filename) {
    var url = URL.createObjectURL(blob);
    var a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    setTimeout(function () { URL.revokeObjectURL(url); }, 5000);
  }

  els.run.addEventListener("click", runWizard);
  els.run.disabled = true;
  loadCompiler();
})();
