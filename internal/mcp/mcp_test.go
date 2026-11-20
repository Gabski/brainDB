// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package mcp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/Gabski/brainDB/internal/bdql"
	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/mcp"
)

func store(t *testing.T) *brain.Store {
	b := brain.New()
	b.Now = func() time.Time { return time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC) }
	if err := b.LoadSeedFile("../brain/testdata/parity_seed.json"); err != nil {
		t.Fatal(err)
	}
	return brain.NewMemStore(b, "test")
}

type session struct {
	w   io.WriteCloser
	r   *bufio.Scanner
	id  int
	t   *testing.T
	end chan error
}

func start(t *testing.T, srv *mcp.Server) *session {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &session{w: inW, r: bufio.NewScanner(outR), t: t, end: make(chan error, 1)}
	s.r.Buffer(make([]byte, 1<<20), 16<<20)
	go func() { s.end <- srv.Serve(context.Background(), inR, outW); outW.Close() }()
	return s
}

func (s *session) call(method string, params any) map[string]any {
	s.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params})
	if _, err := s.w.Write(append(msg, '\n')); err != nil {
		s.t.Fatal(err)
	}
	if !s.r.Scan() {
		s.t.Fatalf("no reply to %s", method)
	}
	var out map[string]any
	if err := json.Unmarshal(s.r.Bytes(), &out); err != nil {
		s.t.Fatal(err)
	}
	if int(out["id"].(float64)) != s.id {
		s.t.Fatalf("id mismatch: %v", out)
	}
	return out
}

func (s *session) notify(method string) {
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	_, _ = s.w.Write(append(msg, '\n'))
}

func tool(s *session, name string, args any) (string, bool) {
	r := s.call("tools/call", map[string]any{"name": name, "arguments": args})["result"].(map[string]any)
	txt := r["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr, _ := r["isError"].(bool)
	return txt, isErr
}

func TestHandshakeAndTools(t *testing.T) {
	st := store(t)
	srv := &mcp.Server{Store: st, Actor: "claude", Name: "braindb", Title: "brainDB by Gabriel Koziestański", Version: "test"}
	all, _ := mcp.ResolveTools([]string{"all"})
	srv.Tools = all
	s := start(t, srv)
	init := s.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}})["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" {
		t.Fatalf("protocolVersion must be echoed: %v", init["protocolVersion"])
	}
	info := init["serverInfo"].(map[string]any)
	if info["name"] != "braindb" || !strings.Contains(info["title"].(string), "Koziestański") || !strings.Contains(init["instructions"].(string), "brainDB") {
		t.Fatalf("initialize: %v", init)
	}
	s.notify("notifications/initialized")
	if r := s.call("ping", nil); r["result"] == nil {
		t.Fatal("ping")
	}
	tools := s.call("tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	names := []string{}
	for _, x := range tools {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	for _, want := range []string{"brain_view", "brain_search", "brain_inspect", "brain_exec", "brain_ingest", "brain_query", "brain_fill", "brain_run"} {
		if !strings.Contains(strings.Join(names, ","), want) {
			t.Errorf("tools/list lacks %s: %v", want, names)
		}
	}
	statusOK := func(txt string) {
		t.Helper()
		lines := strings.Split(strings.TrimSpace(txt), "\n")
		if !strings.HasPrefix(lines[len(lines)-1], "[ok] step ") {
			t.Errorf("result does not end with a status line: %q", lines[len(lines)-1])
		}
	}
	txt, isErr := tool(s, "brain_view", map[string]any{"format": "md", "depth": 1})
	if isErr || !strings.Contains(txt, "# Context:") {
		t.Fatalf("view: %s", txt)
	}
	statusOK(txt)
	txt, _ = tool(s, "brain_search", map[string]any{"query": "SSO OIDC", "mode": "tree"})
	if !strings.Contains(txt, "tree (beam search") {
		t.Fatalf("search: %s", txt)
	}
	statusOK(txt)
	txt, _ = tool(s, "brain_inspect", map[string]any{"id": "d121", "what": []string{"item", "test"}})
	if !strings.Contains(txt, "weight") || !strings.Contains(txt, "ablation") {
		t.Fatalf("inspect: %s", txt)
	}
	txt, isErr = tool(s, "brain_exec", map[string]any{"ops": []any{map[string]any{"op": "remember", "kind": "fact", "text": "x", "tmp": "A"}, map[string]any{"op": "link", "a": "@A", "b": "nope"}}})
	if !isErr || !strings.Contains(txt, "[error] op#2 link: no such item nope") {
		t.Fatalf("failing exec: %s", txt)
	}
	statusOK(txt)
	txt, isErr = tool(s, "brain_exec", map[string]any{"ops": []any{map[string]any{"op": "remember", "kind": "fact", "text": "Nightly sync fits the API limit", "concepts": []string{"API integrations"}}}})
	if isErr || !strings.HasPrefix(txt, "changes: +f") {
		t.Fatalf("exec: %s", txt)
	}
	txt, isErr = tool(s, "brain_query", map[string]any{"query": "kind:fact sort:-weight limit:3", "format": "md"})
	if isErr || !strings.Contains(txt, "| id |") {
		t.Fatalf("query: %s", txt)
	}
	txt, isErr = tool(s, "brain_fill", map[string]any{"template": "deadline: \"rollout deadline:date\"\n", "template_format": "yaml", "output": "json"})
	if isErr || !strings.Contains(txt, "deadline") {
		t.Fatalf("fill: %s", txt)
	}
	txt, isErr = tool(s, "brain_run", map[string]any{"script": "SELECT id FROM memory WHERE kind = decision LIMIT 2; BOOST d121;"})
	if isErr || !strings.Contains(txt, "changes: ▲d121") {
		t.Fatalf("run: %s", txt)
	}
	txt, isErr = tool(s, "brain_nope", map[string]any{})
	if !isErr {
		t.Fatal("unknown tool must be an error result")
	}
	if r := s.call("nope/method", nil); r["error"] == nil {
		t.Fatal("unknown method must be a JSON-RPC error")
	}
	s.w.Close()
	if err := <-s.end; err != nil {
		t.Fatalf("serve: %v", err)
	}
}

func TestReadOnlyAndDefaultTools(t *testing.T) {
	st := store(t)
	srv := &mcp.Server{Store: st, ReadOnly: true}
	s := start(t, srv)
	s.call("initialize", map[string]any{})
	tools := s.call("tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("default tools are brain_view + brain_run: %d", len(tools))
	}
	desc := tools[1].(map[string]any)["description"].(string)
	if !strings.Contains(desc, "SELECT") || !strings.Contains(desc, "REMEMBER") {
		t.Fatal("brain_run carries the BDQL cheat-sheet")
	}
	before := st.B.Focus()
	txt, isErr := tool(s, "brain_view", map[string]any{"focus": []string{"f101"}})
	if isErr || strings.Join(st.B.Focus(), ",") != strings.Join(before, ",") || !strings.Contains(txt, `<focus refs="f101"/>`) {
		t.Fatalf("read-only view must not persist focus: %s", txt)
	}
	if txt, isErr := tool(s, "brain_run", map[string]any{"script": "BOOST d121;"}); !isErr || !strings.Contains(txt, "read-only") {
		t.Fatalf("read-only run must reject writes: %s", txt)
	}
	s.w.Close()
	<-s.end
}
