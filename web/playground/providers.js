// LLM provider adapters for the playground's Run tab. Bring-your-own-key:
// every request goes directly from the browser to the provider the user
// configured — there is no backend, no proxy, and no telemetry. Keys live in
// page memory unless the user explicitly opts into localStorage.
//
// Providers were chosen empirically: each endpoint here answers browser CORS
// preflights. GitHub Copilot itself is deliberately absent — it has no public
// API, and the token workarounds unofficial clients use violate GitHub's
// terms and can flag users' accounts. GitHub Models is the sanctioned way to
// call models with a GitHub account (a fine-grained PAT with models:read).
"use strict";

/**
 * Shared SSE reader: yields the `data:` payload of every event in a fetch
 * response body, stopping at [DONE].
 */
async function* sseData(response) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary;
    while ((boundary = buffer.indexOf("\n")) >= 0) {
      const line = buffer.slice(0, boundary).replace(/\r$/, "");
      buffer = buffer.slice(boundary + 1);
      if (!line.startsWith("data:")) continue;
      const data = line.slice(5).trim();
      if (data === "[DONE]") return;
      if (data) yield data;
    }
  }
}

/** Reads a non-OK response into a single readable error message. */
async function providerError(response) {
  let detail = "";
  try {
    const body = await response.json();
    detail = body?.error?.message || body?.error?.details || JSON.stringify(body).slice(0, 300);
  } catch {
    detail = (await response.text().catch(() => "")).slice(0, 300);
  }
  return new Error(`HTTP ${response.status}${detail ? ` — ${detail}` : ""}`);
}

/**
 * Each provider implements:
 *   listModels(key) -> Promise<string[]>   (best effort; may throw)
 *   stream({key, model, system, message, signal}) -> AsyncGenerator<string>
 */
const PROVIDERS = {
  anthropic: {
    label: "Anthropic",
    keyHint: "sk-ant-…",
    keyUrl: "https://console.anthropic.com/settings/keys",
    defaultModel: "claude-sonnet-5",
    headers(key) {
      return {
        "content-type": "application/json",
        "x-api-key": key,
        "anthropic-version": "2023-06-01",
        // Anthropic's explicit opt-in for direct browser (CORS) access.
        "anthropic-dangerous-direct-browser-access": "true",
      };
    },
    async listModels(key) {
      const r = await fetch("https://api.anthropic.com/v1/models?limit=50", { headers: this.headers(key) });
      if (!r.ok) throw await providerError(r);
      return (await r.json()).data.map((m) => m.id);
    },
    async *stream({ key, model, system, message, signal }) {
      const r = await fetch("https://api.anthropic.com/v1/messages", {
        method: "POST",
        headers: this.headers(key),
        signal,
        body: JSON.stringify({
          model,
          max_tokens: 1024,
          system,
          messages: [{ role: "user", content: message }],
          stream: true,
        }),
      });
      if (!r.ok) throw await providerError(r);
      for await (const data of sseData(r)) {
        const event = JSON.parse(data);
        if (event.type === "content_block_delta" && event.delta?.type === "text_delta") {
          yield event.delta.text;
        }
        if (event.type === "error") throw new Error(event.error?.message || "stream error");
      }
    },
  },

  openai: {
    label: "OpenAI",
    keyHint: "sk-…",
    keyUrl: "https://platform.openai.com/api-keys",
    defaultModel: "gpt-4o",
    // OpenAI omits CORS headers on some error responses, which the browser
    // can only report as a generic network failure.
    corsErrorHint: "OpenAI hides error details from browsers on some failures — double-check the API key and model name.",
    async listModels(key) {
      const r = await fetch("https://api.openai.com/v1/models", { headers: { authorization: `Bearer ${key}` } });
      if (!r.ok) throw await providerError(r);
      return (await r.json()).data.map((m) => m.id).sort();
    },
    async *stream({ key, model, system, message, signal }) {
      // The Responses API: the only OpenAI generation endpoint that answers
      // browser CORS preflights (chat/completions does not, verified 2026-07).
      const r = await fetch("https://api.openai.com/v1/responses", {
        method: "POST",
        headers: { "content-type": "application/json", authorization: `Bearer ${key}` },
        signal,
        body: JSON.stringify({
          model,
          instructions: system,
          input: message,
          stream: true,
        }),
      });
      if (!r.ok) throw await providerError(r);
      for await (const data of sseData(r)) {
        const event = JSON.parse(data);
        if (event.type === "response.output_text.delta" && event.delta) yield event.delta;
        if (event.type === "response.failed") {
          throw new Error(event.response?.error?.message || "response failed");
        }
        if (event.type === "error") throw new Error(event.message || "stream error");
      }
    },
  },

  gemini: {
    label: "Google Gemini",
    keyHint: "AIza…",
    keyUrl: "https://aistudio.google.com/apikey",
    defaultModel: "gemini-2.5-flash",
    async listModels(key) {
      const r = await fetch(`https://generativelanguage.googleapis.com/v1beta/models?pageSize=50&key=${encodeURIComponent(key)}`);
      if (!r.ok) throw await providerError(r);
      return (await r.json()).models
        .filter((m) => (m.supportedGenerationMethods || []).includes("generateContent"))
        .map((m) => m.name.replace(/^models\//, ""));
    },
    async *stream({ key, model, system, message, signal }) {
      const url = `https://generativelanguage.googleapis.com/v1beta/models/${encodeURIComponent(model)}:streamGenerateContent?alt=sse&key=${encodeURIComponent(key)}`;
      const r = await fetch(url, {
        method: "POST",
        headers: { "content-type": "application/json" },
        signal,
        body: JSON.stringify({
          systemInstruction: { parts: [{ text: system }] },
          contents: [{ role: "user", parts: [{ text: message }] }],
        }),
      });
      if (!r.ok) throw await providerError(r);
      for await (const data of sseData(r)) {
        const parts = JSON.parse(data).candidates?.[0]?.content?.parts || [];
        for (const part of parts) if (part.text) yield part.text;
      }
    },
  },

  github: {
    label: "GitHub Models",
    keyHint: "GitHub PAT with models:read",
    keyUrl: "https://github.com/settings/personal-access-tokens",
    defaultModel: "gpt-4o-mini",
    async listModels(key) {
      const r = await fetch("https://models.inference.ai.azure.com/models", {
        headers: { authorization: `Bearer ${key}` },
      });
      if (!r.ok) throw await providerError(r);
      return (await r.json()).map((m) => m.name || m.id).filter(Boolean).sort();
    },
    async *stream({ key, model, system, message, signal }) {
      const r = await fetch("https://models.inference.ai.azure.com/chat/completions", {
        method: "POST",
        headers: { "content-type": "application/json", authorization: `Bearer ${key}` },
        signal,
        body: JSON.stringify({
          model,
          messages: [
            { role: "system", content: system },
            { role: "user", content: message },
          ],
          stream: true,
        }),
      });
      if (!r.ok) throw await providerError(r);
      for await (const data of sseData(r)) {
        const delta = JSON.parse(data).choices?.[0]?.delta?.content;
        if (delta) yield delta;
      }
    },
  },
};
