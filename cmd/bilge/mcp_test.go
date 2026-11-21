package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMCPStdioLifecycleAndTools(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"bilge_scan","arguments":{"task":"dev-cache"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"unknown"}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	called := false
	err := serveMCP(strings.NewReader(input), &output, func(_ context.Context, name string, args json.RawMessage) mcpToolOutput {
		called = true
		if name != "bilge_scan" || string(args) != `{"task":"dev-cache"}` {
			t.Fatalf("unexpected tool call: %s %s", name, args)
		}
		return mcpToolOutput{Text: `{"dry_run":true}`}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("tool handler was not called")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d responses, want 5: %s", len(lines), output.String())
	}
	var responses []map[string]any
	for _, line := range lines {
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("invalid JSON-RPC response %q: %v", line, err)
		}
		responses = append(responses, response)
	}
	if responses[0]["error"].(map[string]any)["code"] != float64(-32000) {
		t.Fatalf("pre-initialization request was accepted: %v", responses[0])
	}
	initial := responses[1]["result"].(map[string]any)
	if initial["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("wrong protocol version: %v", initial)
	}
	tools := responses[2]["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 3 {
		t.Fatalf("missing tools: %v", tools)
	}
	call := responses[3]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	if call["text"] != `{"dry_run":true}` {
		t.Fatalf("wrong tool response: %v", call)
	}
	if responses[4]["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("unknown method was accepted: %v", responses[4])
	}
}

func TestMCPTaskPlanRequiresCleanIntentAndValidScope(t *testing.T) {
	tests := []struct {
		name  string
		args  mcpTaskArgs
		clean bool
		want  []string
		err   bool
	}{
		{name: "all dry run", want: []string{"--json", "scan"}},
		{name: "project scan", args: mcpTaskArgs{Task: "dev-cache", ScanPath: "/tmp/work", Languages: "node", MinAge: "30d"}, want: []string{"--json", "--scan", "/tmp/work", "dev-cache", "--languages", "node", "--min-age", "30d"}},
		{name: "project clean", args: mcpTaskArgs{Confirm: true, Task: "dev-cache", UnsafeClean: true}, clean: true, want: []string{"--json", "clean", "dev-cache", "--yes", "--unsafe-clean"}},
		{name: "clean without confirmation", args: mcpTaskArgs{Task: "all"}, clean: true, err: true},
		{name: "system filters on all", args: mcpTaskArgs{Targets: "docker"}, err: true},
		{name: "project filters on git", args: mcpTaskArgs{Task: "git-cleaner", Languages: "node"}, err: true},
		{name: "unknown task", args: mcpTaskArgs{Task: "other"}, err: true},
		{name: "negative depth", args: mcpTaskArgs{Task: "dev-cache", Depth: -1}, err: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := planMCPTask(test.args, test.clean)
			if (err != nil) != test.err {
				t.Fatalf("error = %v, want error %v", err, test.err)
			}
			if !test.err && !reflect.DeepEqual(got, test.want) {
				t.Fatalf("arguments = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMCPRejectsUnsafeCallsBeforeExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		args string
	}{
		{"bilge_clean", `{}`},
		{"bilge_scan", `{"task":"git-cleaner","unsafe_clean":true}`},
		{"bilge_list_targets", `{"unexpected":true}`},
		{"unknown", `{}`},
	} {
		result := executeMCPTool(context.Background(), test.name, json.RawMessage(test.args))
		if !result.IsError {
			t.Fatalf("%s accepted %s", test.name, test.args)
		}
	}
}
