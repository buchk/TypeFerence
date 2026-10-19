package mcpserver

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type call struct{ args []string }

func session(t *testing.T, code int, lines ...string) ([]map[string]any, []call) {
	t.Helper()
	var calls []call
	run := func(args []string) (string, string, int, error) {
		calls = append(calls, call{args})
		return "out\n", "", code, nil
	}
	var out bytes.Buffer
	if err := Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, run, "test"); err != nil {
		t.Fatal(err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("response is not JSON: %s", line)
		}
		responses = append(responses, r)
	}
	return responses, calls
}

func TestInitializeListAndNotifications(t *testing.T) {
	responses, _ := session(t, 0,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
	)
	if len(responses) != 3 {
		t.Fatalf("a notification must not be answered; got %d responses", len(responses))
	}
	init := responses[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" {
		t.Fatalf("server must speak the client's supported revision: %v", init["protocolVersion"])
	}
	names := []string{}
	for _, tool := range responses[1]["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	for _, want := range []string{"typeference_validate", "typeference_build", "typeference_diff", "typeference_import", "typeference_restore"} {
		found := false
		for _, n := range names {
			found = found || n == want
		}
		if !found {
			t.Fatalf("tools/list is missing %s: %v", want, names)
		}
	}
}

func TestToolCallRunsTheCLIWithPositionalsThenFlags(t *testing.T) {
	_, calls := session(t, 0,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"typeference_build","arguments":{"source":"pkg","out":"dist","packagesDir":"obj/p"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"typeference_import","arguments":{"source":"old","out":"new","lossy":true}}}`,
	)
	want := [][]string{
		{"build", "pkg", "--out", "dist", "--packages-dir", "obj/p"},
		{"import", "old", "--out", "new", "--lossy"},
	}
	for i, c := range calls {
		if !reflect.DeepEqual(c.args, want[i]) {
			t.Fatalf("call %d: got %v, want %v", i, c.args, want[i])
		}
	}
}

func TestDiffDifferencesAreAResultNotAFailure(t *testing.T) {
	responses, _ := session(t, 1,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"typeference_diff","arguments":{"source":"pkg","against":"dist"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"typeference_build","arguments":{"source":"pkg"}}}`,
	)
	if responses[0]["result"].(map[string]any)["isError"] != false {
		t.Fatal("diff exit 1 reports differences and is not an error")
	}
	if responses[1]["result"].(map[string]any)["isError"] != true {
		t.Fatal("build exit 1 is an error")
	}
}

func TestBadArgumentsNeverReachTheCLI(t *testing.T) {
	responses, calls := session(t, 0,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"typeference_build","arguments":{"source":"pkg","target":"x"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"typeference_build","arguments":{"source":"--out"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"typeference_diff","arguments":{"source":"pkg"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"typeference_publish","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/list"}`,
	)
	if len(calls) != 0 {
		t.Fatalf("invalid calls must not run the CLI: %v", calls)
	}
	for i := 0; i < 3; i++ {
		if responses[i]["result"].(map[string]any)["isError"] != true {
			t.Fatalf("response %d must be a tool error: %v", i, responses[i])
		}
	}
	if responses[3]["error"].(map[string]any)["code"].(float64) != -32602 {
		t.Fatalf("an unknown tool is invalid params: %v", responses[3])
	}
	if responses[4]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatalf("an unknown method is method not found: %v", responses[4])
	}
}
