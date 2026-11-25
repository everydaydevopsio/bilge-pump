package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// The stdio transport uses one JSON-RPC message per line. Keep stdout exclusively
// for protocol messages; task output is captured from child processes.
const (
	mcpProtocolVersion = "2025-06-18"
	mcpMaxMessage      = 4 << 20
	mcpMaxOutput       = 4 << 20
	mcpCommandTimeout  = 5 * time.Minute
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolResult struct {
	Content []mcpText `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type mcpText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolOutput struct {
	Text    string
	IsError bool
}

type mcpExecutor func(context.Context, string, json.RawMessage) mcpToolOutput

func serveMCP(in io.Reader, out io.Writer, execute mcpExecutor) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), mcpMaxMessage)
	encoder := json.NewEncoder(out)
	initialized := false
	for scanner.Scan() {
		line := scanner.Bytes()
		var request mcpRequest
		if err := json.Unmarshal(line, &request); err != nil {
			if err := encoder.Encode(mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpError{Code: -32700, Message: "invalid JSON"}}); err != nil {
				return err
			}
			continue
		}
		if len(request.ID) == 0 {
			// notifications/initialized and notifications/cancelled have no response.
			continue
		}
		response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
		switch {
		case request.JSONRPC != "2.0" || request.Method == "":
			response.Error = &mcpError{Code: -32600, Message: "invalid JSON-RPC request"}
		case request.Method == "initialize":
			initialized = true
			response.Result = map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "bilge", "version": version},
				"instructions":    "Scan before cleaning. Cleanup changes local caches and repositories; obtain the user's approval before calling bilge_clean.",
			}
		case !initialized:
			response.Error = &mcpError{Code: -32000, Message: "initialize the MCP connection first"}
		case request.Method == "ping":
			response.Result = map[string]any{}
		case request.Method == "tools/list":
			response.Result = map[string]any{"tools": mcpTools()}
		case request.Method == "tools/call":
			params, err := decodeMCPToolCall(request.Params)
			if err != nil {
				response.Error = &mcpError{Code: -32602, Message: "invalid tools/call params: " + err.Error()}
				break
			}
			result := execute(context.Background(), params.Name, params.Arguments)
			response.Result = mcpToolResult{Content: []mcpText{{Type: "text", Text: result.Text}}, IsError: result.IsError}
		default:
			response.Error = &mcpError{Code: -32601, Message: "method not found"}
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func mcpTools() []map[string]any {
	selection := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"task":         map[string]any{"type": "string", "enum": []string{"all", "dev-cache", "git-cleaner", "system"}, "description": "Cleanup task; system means the current OS cleaner."},
			"scan_path":    map[string]any{"type": "string", "description": "Root directory for dev-cache or git-cleaner."},
			"config":       map[string]any{"type": "string", "description": "Optional unified Bilge config file."},
			"targets":      map[string]any{"type": "string", "description": "Comma-separated system cleaner targets, such as npm,go."},
			"languages":    map[string]any{"type": "string", "description": "Comma-separated dev-cache languages."},
			"depth":        map[string]any{"type": "integer", "minimum": 0},
			"exclude":      map[string]any{"type": "string", "description": "Comma-separated dev-cache path patterns to exclude."},
			"include_only": map[string]any{"type": "string", "description": "Comma-separated dev-cache path patterns to include."},
			"min_age":      map[string]any{"type": "string", "description": "Only dev caches older than this duration, such as 30d."},
			"max_age":      map[string]any{"type": "string", "description": "Only dev caches no older than this duration."},
			"details":      map[string]any{"type": "boolean", "description": "Detailed macOS system findings."},
			"docker_prune": map[string]any{"type": "boolean", "description": "Include macOS Docker prune commands."},
		},
	}
	clean := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"confirm"},
		"properties": map[string]any{
			"confirm":      map[string]any{"type": "boolean", "description": "Must be true after the user has authorized this cleanup."},
			"task":         map[string]any{"type": "string", "enum": []string{"all", "dev-cache", "git-cleaner", "system"}},
			"scan_path":    map[string]any{"type": "string"},
			"config":       map[string]any{"type": "string"},
			"targets":      map[string]any{"type": "string"},
			"languages":    map[string]any{"type": "string"},
			"depth":        map[string]any{"type": "integer", "minimum": 0},
			"exclude":      map[string]any{"type": "string"},
			"include_only": map[string]any{"type": "string"},
			"min_age":      map[string]any{"type": "string"},
			"max_age":      map[string]any{"type": "string"},
			"details":      map[string]any{"type": "boolean"},
			"docker_prune": map[string]any{"type": "boolean"},
			"unsafe_clean": map[string]any{"type": "boolean", "description": "Allow dev-cache deletion outside Git-ignored paths; requires separate user authorization."},
		},
	}
	tools := []map[string]any{
		{"name": "bilge_scan", "description": "Dry-run all Bilge tasks or one selected task. Returns the existing Bilge JSON report and estimated cache sizes.", "inputSchema": selection, "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "bilge_clean", "description": "Execute cleanup for all tasks or one selected task. Requires explicit user authorization and confirm=true. Project cache confirmation is supplied non-interactively.", "inputSchema": clean, "annotations": map[string]any{"destructiveHint": true}},
		{"name": "bilge_list_targets", "description": "List available system cleaner targets and whether each is enabled.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false}, "annotations": map[string]any{"readOnlyHint": true}},
	}
	if runtime.GOOS == "darwin" {
		tools = append(tools, map[string]any{"name": "bilge_check_tools", "description": "Check installed command dependencies for macOS cleaner targets.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"targets": map[string]any{"type": "string"}}}, "annotations": map[string]any{"readOnlyHint": true}})
	}
	return tools
}

type mcpTaskArgs struct {
	Confirm     bool   `json:"confirm"`
	Task        string `json:"task"`
	ScanPath    string `json:"scan_path"`
	Config      string `json:"config"`
	Targets     string `json:"targets"`
	Languages   string `json:"languages"`
	Depth       int    `json:"depth"`
	Exclude     string `json:"exclude"`
	IncludeOnly string `json:"include_only"`
	MinAge      string `json:"min_age"`
	MaxAge      string `json:"max_age"`
	Details     bool   `json:"details"`
	DockerPrune bool   `json:"docker_prune"`
	UnsafeClean bool   `json:"unsafe_clean"`
}

type mcpToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// decodeMCPToolCall reads tools/call params. The MCP spec lets clients add
// top-level fields such as _meta (Claude Code sends progressToken there), so
// unknown params are ignored; tool arguments stay strict in executeMCPTool.
func decodeMCPToolCall(raw json.RawMessage) (mcpToolCall, error) {
	var params mcpToolCall
	if err := decodeObject(raw, &params, false, "params"); err != nil {
		return params, err
	}
	if params.Name == "" {
		return params, errors.New("tool name is required")
	}
	arguments := bytes.TrimSpace(params.Arguments)
	if len(arguments) == 0 || bytes.Equal(arguments, []byte("null")) {
		params.Arguments = json.RawMessage("{}")
	} else if arguments[0] != '{' {
		return params, errors.New("arguments must be an object")
	}
	return params, nil
}

func decodeMCPObject(raw json.RawMessage, dst any) error {
	return decodeObject(raw, dst, true, "arguments")
}

func decodeObject(raw json.RawMessage, dst any, strict bool, what string) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return fmt.Errorf("%s must be an object", what)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("unexpected data after %s", what)
	}
	return nil
}

func executeMCPTool(ctx context.Context, name string, raw json.RawMessage) mcpToolOutput {
	if name != "bilge_scan" && name != "bilge_clean" && name != "bilge_list_targets" && name != "bilge_check_tools" {
		return mcpToolOutput{Text: "unknown tool " + name, IsError: true}
	}
	var args mcpTaskArgs
	if name == "bilge_list_targets" {
		var empty struct{}
		if err := decodeMCPObject(raw, &empty); err != nil {
			return mcpToolOutput{Text: "invalid arguments: " + err.Error(), IsError: true}
		}
	} else if name == "bilge_check_tools" {
		var check struct {
			Targets string `json:"targets"`
		}
		if err := decodeMCPObject(raw, &check); err != nil {
			return mcpToolOutput{Text: "invalid arguments: " + err.Error(), IsError: true}
		}
		args.Targets = check.Targets
	} else if err := decodeMCPObject(raw, &args); err != nil {
		return mcpToolOutput{Text: "invalid arguments: " + err.Error(), IsError: true}
	}
	var executable string
	var command []string
	switch name {
	case "bilge_scan", "bilge_clean":
		clean := name == "bilge_clean"
		var err error
		command, err = planMCPTask(args, clean)
		if err != nil {
			return mcpToolOutput{Text: err.Error(), IsError: true}
		}
		executable, err = os.Executable()
		if err != nil {
			return mcpToolOutput{Text: err.Error(), IsError: true}
		}
	case "bilge_list_targets", "bilge_check_tools":
		if name == "bilge_check_tools" && runtime.GOOS != "darwin" {
			return mcpToolOutput{Text: "tool checking is available only for the macOS cleaner", IsError: true}
		}
		var err error
		executable, err = executableFor(platformCleaner())
		if err != nil {
			return mcpToolOutput{Text: err.Error(), IsError: true}
		}
		if name == "bilge_list_targets" {
			command = []string{"--list-targets"}
		} else {
			command = []string{"--check-tools"}
			if args.Targets != "" {
				command = append(command, "--targets", args.Targets)
			}
		}
	}
	return runMCPCommand(ctx, executable, command)
}

func planMCPTask(args mcpTaskArgs, clean bool) ([]string, error) {
	if clean != args.Confirm {
		if clean {
			return nil, errors.New("cleanup requires confirm=true after user authorization")
		}
		return nil, errors.New("confirm is only valid for cleanup")
	}
	if args.Depth < 0 {
		return nil, errors.New("depth must be nonnegative")
	}
	task := args.Task
	if task == "" {
		task = "all"
	}
	if task != "all" && task != "dev-cache" && task != "git-cleaner" && task != "system" {
		return nil, fmt.Errorf("unknown task %q", task)
	}
	if task == "system" && platformCleaner() == "" {
		return nil, errors.New("no cleaner is available for this operating system")
	}
	if (args.Targets != "" || args.Details || args.DockerPrune) && task != "system" {
		return nil, errors.New("targets, details and docker_prune require task=system")
	}
	if (args.Details || args.DockerPrune) && runtime.GOOS != "darwin" {
		return nil, errors.New("details and docker_prune are macOS-only")
	}
	if (args.Languages != "" || args.Depth != 0 || args.Exclude != "" || args.IncludeOnly != "" || args.MinAge != "" || args.MaxAge != "" || args.UnsafeClean) && task != "dev-cache" {
		return nil, errors.New("dev-cache filters require task=dev-cache")
	}
	if args.UnsafeClean && !clean {
		return nil, errors.New("unsafe_clean is only valid for cleanup")
	}
	if args.ScanPath != "" && task == "system" {
		return nil, errors.New("scan_path applies only to dev-cache and git-cleaner")
	}
	for _, value := range []string{args.ScanPath, args.Config, args.Targets, args.Languages, args.Exclude, args.IncludeOnly, args.MinAge, args.MaxAge} {
		if len(value) > 4096 || strings.ContainsRune(value, 0) {
			return nil, errors.New("argument is too long or contains a NUL byte")
		}
	}
	command := []string{"--json"}
	if args.Config != "" {
		command = append(command, "--config", args.Config)
	}
	if args.ScanPath != "" {
		command = append(command, "--scan", args.ScanPath)
	}
	if clean {
		command = append(command, "clean")
	} else if task == "all" {
		command = append(command, "scan")
	}
	switch task {
	case "all":
		if clean {
			command = append(command, "--all")
		}
	case "system":
		command = append(command, platformCleaner())
	default:
		command = append(command, task)
	}
	if clean && (task == "dev-cache" || task == "all") {
		command = append(command, "--yes")
	}
	if args.Targets != "" {
		command = append(command, "--targets", args.Targets)
	}
	if args.Languages != "" {
		command = append(command, "--languages", args.Languages)
	}
	if args.Depth != 0 {
		command = append(command, "--depth", fmt.Sprint(args.Depth))
	}
	for _, pair := range [][2]string{{"--exclude", args.Exclude}, {"--include-only", args.IncludeOnly}, {"--min-age", args.MinAge}, {"--max-age", args.MaxAge}} {
		if pair[1] != "" {
			command = append(command, pair[0], pair[1])
		}
	}
	if args.Details {
		command = append(command, "--details")
	}
	if args.DockerPrune {
		command = append(command, "--docker-prune")
	}
	if args.UnsafeClean {
		command = append(command, "--unsafe-clean")
	}
	return command, nil
}

type limitWriter struct {
	bytes.Buffer
	limit int
	full  bool
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.Len() {
		w.full = true
		return 0, errors.New("command output exceeded limit")
	}
	return w.Buffer.Write(p)
}

func runMCPCommand(ctx context.Context, executable string, args []string) mcpToolOutput {
	ctx, cancel := context.WithTimeout(ctx, mcpCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = nil
	stdout := &limitWriter{limit: mcpMaxOutput}
	stderr := &limitWriter{limit: mcpMaxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if stdout.full || stderr.full {
		return mcpToolOutput{Text: "Bilge output exceeded the 4 MiB response limit; narrow the scan", IsError: true}
	}
	if ctx.Err() != nil {
		return mcpToolOutput{Text: "Bilge command timed out or was cancelled", IsError: true}
	}
	if err != nil {
		return mcpToolOutput{Text: strings.TrimSpace(stdout.String() + "\n" + stderr.String() + "\n" + err.Error()), IsError: true}
	}
	return mcpToolOutput{Text: strings.TrimSpace(stdout.String())}
}
