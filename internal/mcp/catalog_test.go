// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package mcp_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/mcp"
)

type fakeCatalog struct{ stores map[string]*brain.Store }

func (f *fakeCatalog) List() []mcp.DBInfo {
	out := []mcp.DBInfo{}
	for n, st := range f.stores {
		nodes, edges := st.B.Counts()
		out = append(out, mcp.DBInfo{Name: n, Nodes: nodes, Edges: edges, Description: "db " + n})
	}
	return out
}

func (f *fakeCatalog) Open(name string) (*brain.Store, error) {
	if st := f.stores[name]; st != nil {
		return st, nil
	}
	return nil, fmt.Errorf("no database %q", name)
}

func TestBrainUseSwitchesDatabase(t *testing.T) {
	a, b := store(t), brain.NewMemStore(brain.New(), "other")
	a.Name = "main"
	cat := &fakeCatalog{stores: map[string]*brain.Store{"main": a, "other": b}}
	srv := &mcp.Server{Store: a, Catalog: cat}
	s := start(t, srv)
	init := s.call("initialize", map[string]any{})["result"].(map[string]any)
	if !strings.Contains(init["instructions"].(string), "brain_use") {
		t.Fatal("instructions must tell the agent it can switch databases")
	}
	tools := s.call("tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("view, run and use expected, got %d", len(tools))
	}
	if txt, isErr := tool(s, "brain_use", map[string]any{}); isErr || !strings.Contains(txt, "▶ | main") || !strings.Contains(txt, "other") {
		t.Fatalf("listing must mark the active database: %s", txt)
	}
	if txt, isErr := tool(s, "brain_run", map[string]any{"script": `REMEMBER fact "only in other";`}); isErr {
		t.Fatalf("write to main: %s", txt)
	}
	if txt, isErr := tool(s, "brain_use", map[string]any{"name": "other"}); isErr || !strings.Contains(txt, "now using other") {
		t.Fatalf("switch: %s", txt)
	}
	if srv.Store != b {
		t.Fatal("session must now be bound to the other store")
	}
	if txt, isErr := tool(s, "brain_run", map[string]any{"script": `SELECT COUNT(*) FROM memory;`}); isErr || !strings.Contains(txt, "| 0 |") {
		t.Fatalf("other database is empty: %s", txt)
	}
	if txt, isErr := tool(s, "brain_use", map[string]any{"name": "missing"}); !isErr || !strings.Contains(txt, "no database") || srv.Store != b {
		t.Fatalf("unknown name must fail and keep the current database: %s", txt)
	}
	s.w.Close()
	<-s.end
}

func TestUseStatementReadsAnotherDatabaseOnce(t *testing.T) {
	a, b := store(t), brain.NewMemStore(brain.New(), "other")
	a.Name = "main"
	cat := &fakeCatalog{stores: map[string]*brain.Store{"main": a, "other": b}}
	srv := &mcp.Server{Store: a, Catalog: cat}
	s := start(t, srv)
	s.call("initialize", map[string]any{})
	if txt, isErr := tool(s, "brain_run", map[string]any{"script": `USE other; SELECT COUNT(*) FROM memory;`}); isErr || !strings.Contains(txt, "using other") || !strings.Contains(txt, "| 0 |") {
		t.Fatalf("USE in brain_run: %s", txt)
	}
	if srv.Store != a {
		t.Fatal("USE applies to one script, the session stays on its database")
	}
	if txt, isErr := tool(s, "brain_run", map[string]any{"script": `SHOW dbs;`}); isErr || !strings.Contains(txt, "other") || !strings.Contains(txt, "main") {
		t.Fatalf("SHOW dbs lists the catalog: %s", txt)
	}
	s.w.Close()
	<-s.end
}

func TestNoCatalogMeansNoSwitching(t *testing.T) {
	srv := &mcp.Server{Store: store(t)}
	s := start(t, srv)
	s.call("initialize", map[string]any{})
	tools := s.call("tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("brain_use is hidden when the session is bound to one database: %d tools", len(tools))
	}
	if txt, isErr := tool(s, "brain_run", map[string]any{"script": `USE other;`}); !isErr || !strings.Contains(txt, "bound to one database") {
		t.Fatalf("USE must stay refused: %s", txt)
	}
	s.w.Close()
	<-s.end
}
