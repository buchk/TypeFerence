package eval

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
)

// Backend executes a Messages API request payload and returns the response
// text. Implementations exist for the Anthropic API and for dry-run; other
// providers can be added by implementing this interface.
type Backend interface {
	Name() string
	// Complete sends the exact payload and returns the concatenated text
	// content of the response.
	Complete(ctx context.Context, payload []byte) (string, error)
}

// AnthropicBackend calls the Anthropic Messages API directly over HTTPS using
// only the standard library (see ADR-0008 for why the SDK is not used here).
type AnthropicBackend struct {
	APIKey  string
	BaseURL string // defaults to https://api.anthropic.com
	Client  *http.Client
}

// Name implements Backend.
func (b *AnthropicBackend) Name() string { return "anthropic" }

// Complete implements Backend.
func (b *AnthropicBackend) Complete(ctx context.Context, payload []byte) (string, error) {
	baseURL := b.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	client := b.Client
	if client == nil {
		// Requests with adaptive thinking can legitimately run for minutes.
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", b.APIKey)
	request.Header.Set("anthropic-version", "2023-06-01")

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic api returned %d: %s", response.StatusCode, truncate(string(body), 500))
	}
	return extractText(string(body))
}

// extractText concatenates the text blocks of a Messages API response,
// checking stop_reason before reading content.
func extractText(body string) (string, error) {
	parsed, err := jsonx.Parse(body)
	if err != nil {
		return "", fmt.Errorf("invalid response JSON: %v", err)
	}
	obj, ok := parsed.(jsonx.Obj)
	if !ok {
		return "", fmt.Errorf("response is not a JSON object")
	}
	if reason := member(obj, "stop_reason"); reason != nil {
		if s, isString := reason.(jsonx.Str); isString && string(s) == "refusal" {
			return "", fmt.Errorf("the model declined the request (stop_reason: refusal)")
		}
	}
	content, ok := member(obj, "content").(jsonx.Arr)
	if !ok {
		return "", fmt.Errorf("response has no content array")
	}
	var text strings.Builder
	for _, block := range content {
		blockObj, isObj := block.(jsonx.Obj)
		if !isObj {
			continue
		}
		if t, isStr := member(blockObj, "type").(jsonx.Str); isStr && string(t) == "text" {
			if v, isStr := member(blockObj, "text").(jsonx.Str); isStr {
				text.WriteString(string(v))
			}
		}
	}
	if text.Len() == 0 {
		return "", fmt.Errorf("response contained no text content")
	}
	return text.String(), nil
}

func member(obj jsonx.Obj, key string) jsonx.Value {
	for _, m := range obj {
		if m.K == key {
			return m.V
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
