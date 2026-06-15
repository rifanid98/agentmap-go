// Package mcp implements an in-process stdio JSON-RPC 2.0 MCP server exposing
// the same 8 tools as the original agentmap: any, find, relates, map, hubs,
// features, feature, symbols. The server holds the cache warm across calls.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/rifanid/agentmap/internal/build"
	"github.com/rifanid/agentmap/internal/model"
	"github.com/rifanid/agentmap/internal/query"
	"github.com/rifanid/agentmap/internal/rank"
)

const protocolVersion = "2024-11-05"

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type rpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Serve starts the MCP server loop, reading line-delimited JSON-RPC from stdin.
func Serve() error {
	wd, _ := os.Getwd()
	// warm cache once at startup
	m, _ := build.EnsureFresh(wd)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024*1024), 64*1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		// Notifications (no id) get no reply.
		if req.ID == nil && req.Method != "initialize" {
			continue
		}
		resp := dispatch(req, wd, &m)
		enc.Encode(resp)
	}
	return scanner.Err()
}

func dispatch(req rpcRequest, wd string, mp **model.Map) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "agentmap", "version": "0.1.0"},
			},
		}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": toolList()}}
	case "tools/call":
		return callTool(req, wd, mp)
	default:
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

func callTool(req rpcRequest, wd string, mp **model.Map) rpcResponse {
	name, _ := req.Params["name"].(string)
	args, _ := req.Params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	// re-use or refresh the in-process cache
	if *mp == nil {
		m, _ := build.EnsureFresh(wd)
		*mp = m
	}

	var result any
	var isErr bool

	switch name {
	case "any":
		q := str(args, "query")
		if q == "" {
			return errResp(req.ID, "--any needs a query")
		}
		res, ok := query.Any(*mp, wd, q)
		if !ok {
			isErr = false // zero results is not isError
		}
		result = map[string]any{"command": "any", "result": res}

	case "find":
		q := str(args, "query")
		if q == "" {
			return errResp(req.ID, "--find needs a symbol query")
		}
		res := query.Find(*mp, q)
		result = map[string]any{"command": "find", "result": res}
		if len(res.Matches) == 0 {
			isErr = false
		}

	case "relates":
		q := str(args, "query")
		if q == "" {
			return errResp(req.ID, "--relates needs a package path/name")
		}
		res := query.Relates(*mp, q)
		if res.Error != "" {
			isErr = true
		}
		result = map[string]any{"command": "relates", "result": res}

	case "map":
		focusArg := str(args, "focus")
		budget, _ := strconv.Atoi(str(args, "tokens"))
		res, _ := query.MapDigest(*mp, focusArg, budget)
		result = map[string]any{"command": "map", "result": res}

	case "hubs":
		result = map[string]any{"command": "hubs", "packageCount": (*mp).PackageCount, "sha": (*mp).GeneratedSha, "hubs": (*mp).Hubs}

	case "features":
		out := map[string]int{}
		for k, v := range (*mp).Features {
			out[k] = len(v)
		}
		result = map[string]any{"command": "features", "features": out}

	case "feature":
		q := str(args, "query")
		res := featureLookup(*mp, q)
		result = map[string]any{"command": "feature", "result": res}
		if res["error"] != nil {
			isErr = true
		}

	case "symbols":
		n := 30
		if v, err := strconv.Atoi(str(args, "n")); err == nil && v > 0 {
			n = v
		}
		syms := (*mp).RankedSymbols
		if len(syms) > n {
			syms = syms[:n]
		}
		_ = rank.RankedSymbolsLimit
		result = map[string]any{"command": "symbols", "symbols": syms}

	default:
		return errResp(req.ID, "unknown tool: "+name)
	}

	text, _ := json.Marshal(result)
	return rpcResponse{
		JSONRPC: "2.0", ID: req.ID,
		Result: toolResult{Content: []contentItem{{Type: "text", Text: string(text)}}, IsError: isErr},
	}
}

func featureLookup(m *model.Map, q string) map[string]any {
	for k, v := range m.Features {
		if k == q {
			return map[string]any{"name": k, "files": v}
		}
	}
	return map[string]any{"error": "no match", "query": q}
}

func errResp(id any, msg string) rpcResponse {
	text, _ := json.Marshal(map[string]any{"error": msg})
	return rpcResponse{
		JSONRPC: "2.0", ID: id,
		Result: toolResult{Content: []contentItem{{Type: "text", Text: string(text)}}, IsError: true},
	}
}

func str(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func toolList() []map[string]any {
	return []map[string]any{
		{"name": "any", "description": "Route a query: package → symbol → feature → git-grep",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}},
		{"name": "find", "description": "Find exported symbols by (sub)name",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}},
		{"name": "relates", "description": "Blast radius: imports/dependents/related for a package",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}},
		{"name": "map", "description": "Token-budgeted ranked digest",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"focus":  map[string]any{"type": "string"},
				"tokens": map[string]any{"type": "string"},
			}}},
		{"name": "hubs", "description": "Top packages by PageRank importance",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
		{"name": "features", "description": "List DDD domain features by size",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
		{"name": "feature", "description": "Packages composing a DDD domain feature",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}},
		{"name": "symbols", "description": "Top-n Aider-style ranked symbols",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"n": map[string]any{"type": "string"}}}},
	}
}

// FormatText formats a result as a JSON text for MCP content.
func FormatText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// PrintVersion returns the server version string.
func PrintVersion() string { return fmt.Sprintf("agentmap mcp 0.1.0 (%s)", protocolVersion) }
