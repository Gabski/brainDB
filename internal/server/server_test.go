// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gabski/brainDB/internal/bdql"
	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/server"
)

func newServer(t *testing.T, tokens ...server.Token) (*server.Server, *httptest.Server, string) {
	dir := t.TempDir()
	p := filepath.Join(dir, "brain.xml")
	st, err := brain.OpenStore(p, "../brain/testdata/parity_seed.json")
	if err != nil {
		t.Fatal(err)
	}
	s := server.New("test")
	s.Author, s.License = "Gabriel Koziestański", "Apache-2.0"
	s.Tokens = tokens
	s.Hooks.Run = bdql.HTTPRun
	s.AddDB("main", st)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { s.Close(); ts.Close() })
	return s, ts, dir
}

func do(t *testing.T, method, url, body string, hdr ...string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	d, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(d), res.Header
}

func TestAPI(t *testing.T) {
	_, ts, dir := newServer(t)
	u := ts.URL
	check := func(name string, code int, body string, wantCode int, needles ...string) {
		t.Helper()
		if code != wantCode {
			t.Errorf("%s: status %d want %d: %.300s", name, code, wantCode, body)
		}
		for _, n := range needles {
			if !strings.Contains(body, n) {
				t.Errorf("%s: body lacks %q: %.300s", name, n, body)
			}
		}
	}
	c, b, _ := do(t, "GET", u+"/api/health", "")
	check("health", c, b, 200, `"status":"ok"`, `"author":"Gabriel Koziestański"`, `"license":"Apache-2.0"`)
	c, b, _ = do(t, "GET", u+"/api/state", "")
	check("state", c, b, 200, `"nodes":[`, `"edges":[`, `"params":{"depth":2`, `"embedding":{"model":"hash-256"`, `"stats":{"lit":`, `"kinds":[`, `"halfLife":null`, `"goalFocus":`)
	var st brain.State
	_ = json.Unmarshal([]byte(b), &st)
	if len(st.Nodes) == 0 || st.Status == "" {
		t.Fatal("state decode")
	}
	c, b, _ = do(t, "POST", u+"/api/context", `{"focus":["d121"],"depth":1}`)
	check("context", c, b, 200, `"focus":["d121"]`, `"depth":1`)
	c, b, _ = do(t, "POST", u+"/api/embedding", `{"use":"raw","alpha":0.3}`)
	check("embedding", c, b, 200, `"use":"raw"`)
	for _, m := range []string{"hybrid", "vector", "tree", "graph"} {
		c, b, _ = do(t, "POST", u+"/api/search", `{"query":"SSO rollout","mode":"`+m+`","k":5}`)
		check("search "+m, c, b, 200, `"mode":"`+m+`"`, `"rows":[`)
	}
	c, b, _ = do(t, "GET", u+"/api/inspect?id=d121&what=item,links,near,test,suggest,paths&to=f101", "")
	check("inspect", c, b, 200, `"item":{"node":`, `"links":[`, `"near":[`, `"test":{"reached":`, `"suggest":[`, `"paths":[`)
	c, b, _ = do(t, "GET", u+"/api/inspect?what=discover", "")
	check("discover", c, b, 200, `"discover":{"inner":`)
	c, b, _ = do(t, "GET", u+"/api/layout?view=embed", "")
	check("layout embed", c, b, 200, `"positions":{"`)
	c, b, _ = do(t, "GET", u+"/api/layout?view=tree&root=d121", "")
	check("layout tree", c, b, 200, `"treeEdges":[[`, `"root":"d121"`)
	c, b, _ = do(t, "GET", u+"/api/vector?id=d121", "")
	check("vector", c, b, 200, `"raw":[`, `"enriched":[`, `"dim":256`, `"drift":`)
	var v brain.VectorInfo
	_ = json.Unmarshal([]byte(b), &v)
	if len(v.Raw) != 64 {
		t.Errorf("vector: %d dims, want 64", len(v.Raw))
	}
	c, b, _ = do(t, "POST", u+"/api/exec", `{"ops":[{"op":"remember","kind":"fact","text":"HTTP fact","concepts":["API integrations"]}]}`)
	check("exec", c, b, 200, `"changes":["+f`, `"touched":[`, `"auto":[`, `"status":"[ok] step`)
	c, b, _ = do(t, "POST", u+"/api/exec", `{"ops":[{"op":"boost","id":"nope"}]}`)
	check("exec failure", c, b, 400, `"error":"op#1 boost: no such item nope"`)
	c, b, _ = do(t, "POST", u+"/api/kinds", `{"name":"deal","color":"#ff8787","halfLife":21}`)
	check("kinds", c, b, 200, `"name":"deal"`)
	c, b, _ = do(t, "GET", u+"/api/context.xml", "")
	check("context.xml", c, b, 200, "<context ")
	c, b, _ = do(t, "GET", u+"/api/context.md", "")
	check("context.md", c, b, 200, "# Context:")
	c, xml, _ := do(t, "GET", u+"/api/brain.xml", "")
	check("brain.xml", c, xml, 200, "<brain ")
	c, b, _ = do(t, "PUT", u+"/api/brain.xml", strings.Replace(xml, "<goal>Ship the SSO login rollout</goal>", "<goal>New goal</goal>", 1))
	check("brain.xml put", c, b, 200, `"goal":"New goal"`)
	c, b, _ = do(t, "PUT", u+"/api/brain.xml", "<nope/>")
	check("brain.xml bad", c, b, 400, `"error"`)
	c, b, _ = do(t, "POST", u+"/api/query", `{"query":"kind:fact sort:-weight limit:3","format":"md"}`)
	check("query", c, b, 200, `"columns":[`, `"rows":[`, `"rendered":"| id |`)
	c, b, _ = do(t, "POST", u+"/api/context/query", `{"query":"deadline","depth":1,"format":"yaml"}`)
	check("context query", c, b, 200, `"output":"now:`)
	c, b, _ = do(t, "POST", u+"/api/fill", `{"template":"deadline: \"rollout deadline:date\"\n","output":"json"}`)
	check("fill", c, b, 200, `"output":`, `"values":[`, `"missing":`)
	c, b, _ = do(t, "GET", u+"/api/templates", "")
	check("templates", c, b, 200, "[")
	c, b, _ = do(t, "POST", u+"/api/run", `{"script":"SELECT id FROM memory LIMIT 2; BOOST d121;"}`)
	check("run", c, b, 200, `"ok":true`, `"kind":"rows"`, `"kind":"changes"`)
	c, b, _ = do(t, "POST", u+"/api/run/explain", `{"script":"SELEC x"}`)
	check("explain error", c, b, 400, `did you mean SELECT`)
	c, b, _ = do(t, "GET", u+"/api/journal", "")
	check("journal", c, b, 200, `"tool":"exec"`)
	if _, err := os.Stat(filepath.Join(dir, "brain.journal.jsonl")); err != nil {
		t.Error("journal file missing")
	}
	c, b, _ = do(t, "GET", u+"/", "")
	check("ui", c, b, 200, "<")
	// multipart ingest
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files[]", "notes.md")
	_, _ = fw.Write([]byte("# Notes\n\nThe SSO rollout needs OIDC tests.\n"))
	fw, _ = mw.CreateFormFile("files[]", "deals.csv")
	_, _ = fw.Write([]byte("deal,value\nA,100\nB,200\n"))
	_ = mw.WriteField("focus", "true")
	mw.Close()
	req, _ := http.NewRequest("POST", u+"/api/ingest", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := io.ReadAll(res.Body)
	res.Body.Close()
	check("ingest", res.StatusCode, string(d), 200, `"report":[`, `notes.md`, `2 records`, `"fresh":[`)
}

func TestEvents(t *testing.T) {
	_, ts, _ := newServer(t)
	res, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %s", ct)
	}
	sc := bufio.NewScanner(res.Body)
	next := func() string {
		for sc.Scan() {
			if l := sc.Text(); strings.HasPrefix(l, "data: ") {
				return l[6:]
			}
		}
		return ""
	}
	if h := next(); !strings.Contains(h, `"type":"hello"`) {
		t.Fatalf("hello: %s", h)
	}
	go func() {
		req, _ := http.NewRequest("POST", ts.URL+"/api/exec", strings.NewReader(`{"ops":[{"op":"boost","id":"d121"}]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-BrainDB-Actor", "tester")
		if r, err := http.DefaultClient.Do(req); err == nil {
			r.Body.Close()
		}
	}()
	done := make(chan string, 1)
	go func() { done <- next() }()
	select {
	case ev := <-done:
		if !strings.Contains(ev, `"type":"changed"`) || !strings.Contains(ev, `"by":"tester"`) || !strings.Contains(ev, `"db":"main"`) {
			t.Fatalf("event: %s", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no SSE event")
	}
}

func TestAuthScopes(t *testing.T) {
	_, ts, _ := newServer(t, server.Token{Name: "default", Value: "admintok", Scope: "admin"}, server.Token{Name: "read", Value: "readtok", Scope: "read"})
	u := ts.URL
	if c, _, _ := do(t, "GET", u+"/api/health", ""); c != 200 {
		t.Fatal("health needs no auth")
	}
	if c, _, _ := do(t, "GET", u+"/", ""); c != 200 {
		t.Fatal("UI index needs no auth")
	}
	if c, b, _ := do(t, "GET", u+"/api/state", ""); c != 401 || !strings.Contains(b, "unauthorized") {
		t.Fatalf("no token: %d %s", c, b)
	}
	if c, _, _ := do(t, "GET", u+"/api/state?token=readtok", ""); c != 200 {
		t.Fatal("?token= works (EventSource)")
	}
	if c, _, _ := do(t, "POST", u+"/api/search", `{"query":"sso"}`, "Authorization", "Bearer readtok"); c != 200 {
		t.Fatal("read token can search")
	}
	if c, _, _ := do(t, "POST", u+"/api/exec", `{"ops":[{"op":"boost","id":"d121"}]}`, "Authorization", "Bearer readtok"); c != 403 {
		t.Fatal("read token cannot exec")
	}
	if c, b, _ := do(t, "POST", u+"/api/run", `{"script":"BOOST d121;"}`, "Authorization", "Bearer readtok"); c != 400 || !strings.Contains(b, "read-only") {
		t.Fatalf("read token runs BDQL read-only: %d %s", c, b)
	}
	if c, _, _ := do(t, "POST", u+"/api/exec", `{"ops":[{"op":"boost","id":"d121"}]}`, "Authorization", "Bearer admintok"); c != 200 {
		t.Fatal("admin token can exec")
	}
	if c, _, _ := do(t, "GET", u+"/api/state", "", "Authorization", "Bearer wrong"); c != 401 {
		t.Fatal("wrong token")
	}
}

func TestMultiDB(t *testing.T) {
	dir := t.TempDir()
	s := server.New("test")
	s.DataDir = dir
	if _, err := s.CreateDB("alpha", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDB("beta", ""); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	defer s.Close()
	do(t, "POST", ts.URL+"/api/exec?db=beta", `{"ops":[{"op":"remember","kind":"fact","text":"only in beta"}]}`)
	_, a, _ := do(t, "GET", ts.URL+"/api/state?db=alpha", "")
	_, b, _ := do(t, "GET", ts.URL+"/api/state", "", "X-BrainDB-Database", "beta")
	if strings.Contains(a, "only in beta") || !strings.Contains(b, "only in beta") {
		t.Fatal("databases are not isolated")
	}
	c, l, _ := do(t, "GET", ts.URL+"/api/dbs", "")
	if c != 200 || !strings.Contains(l, `"name":"alpha"`) || !strings.Contains(l, `"name":"beta"`) {
		t.Fatalf("dbs: %s", l)
	}
	if c, _, _ := do(t, "POST", ts.URL+"/api/dbs", `{"name":"gamma"}`); c != 200 {
		t.Fatal("create db")
	}
	if c, _, _ := do(t, "DELETE", ts.URL+"/api/dbs/gamma", ""); c != 400 {
		t.Fatal("delete needs confirm")
	}
	if c, _, _ := do(t, "DELETE", ts.URL+"/api/dbs/gamma?confirm=gamma", ""); c != 200 {
		t.Fatal("delete db")
	}
	if c, _, _ := do(t, "GET", ts.URL+"/api/state?db=gamma", ""); c != 404 {
		t.Fatal("deleted db still served")
	}
}

func TestMCPOverHTTP(t *testing.T) {
	_, ts, _ := newServer(t)
	c, b, h := do(t, "POST", ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`, "Accept", "application/json, text/event-stream")
	sid := h.Get("Mcp-Session-Id")
	if c != 200 || sid == "" || !strings.Contains(b, `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize: %d %s", c, b)
	}
	if c, _, _ := do(t, "POST", ts.URL+"/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, "Mcp-Session-Id", sid); c != 202 {
		t.Fatalf("notification: %d", c)
	}
	c, b, _ = do(t, "POST", ts.URL+"/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"brain_run","arguments":{"script":"SELECT id FROM memory LIMIT 1;"}}}`,
		"Mcp-Session-Id", sid, "Accept", "text/event-stream")
	if c != 200 || !strings.HasPrefix(b, "event: message\ndata: ") || !strings.Contains(b, "[ok] step") {
		t.Fatalf("SSE tool call: %d %s", c, b)
	}
	if c, _, _ := do(t, "DELETE", ts.URL+"/mcp", "", "Mcp-Session-Id", sid); c != 204 {
		t.Fatal("delete session")
	}
	if c, _, _ := do(t, "POST", ts.URL+"/mcp", `{"jsonrpc":"2.0","id":3,"method":"ping"}`, "Mcp-Session-Id", sid); c != 404 {
		t.Fatal("ended session must be unknown")
	}
}

func TestLoopbackGuard(t *testing.T) {
	if !server.IsLoopback("127.0.0.1:7791") || !server.IsLoopback("localhost:1") || server.IsLoopback("0.0.0.0:7791") {
		t.Fatal("IsLoopback")
	}
	s := server.New("t")
	if err := s.ListenAndServe(context.Background(), "0.0.0.0:0"); err == nil || !strings.Contains(err.Error(), "without a token") {
		t.Fatalf("non-loopback without token must refuse: %v", err)
	}
}

// The UI index must never be cached: it names content-hashed asset files, so a cached copy keeps
// serving the previous build after an upgrade. The hashed assets themselves are immutable.
func TestUICacheHeaders(t *testing.T) {
	_, ts, _ := newServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("index Cache-Control = %q, want %q", got, "no-cache")
	}

	// an unknown path falls back to the index, and must not be cached either
	res2, err := http.Get(ts.URL + "/some/spa/route")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if got := res2.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("SPA fallback Cache-Control = %q, want %q", got, "no-cache")
	}

	// a missing asset falls back to the index too, so it must not be cached as an asset would be
	res3, err := http.Get(ts.URL + "/assets/does-not-exist-ABC123.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if got := res3.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("missing asset Cache-Control = %q, want %q", got, "no-cache")
	}

	// The hashed assets are only embedded once the web UI has been built (they are gitignored),
	// so this half of the policy is checked only in a tree where `make web` has run.
	entries, err := os.ReadDir("ui/dist/assets")
	if err != nil || len(entries) == 0 {
		t.Skip("no built UI assets embedded; run 'make web' to cover the immutable policy")
	}
	res4, err := http.Get(ts.URL + "/assets/" + entries[0].Name())
	if err != nil {
		t.Fatal(err)
	}
	defer res4.Body.Close()
	if got := res4.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset Cache-Control = %q, want an immutable policy", got)
	}
}

// The "new database" wizard previews an import and validates a config under a name that does not
// exist yet. Those dry runs must work; anything that would write must still 404.
func TestDryRunAgainstUnknownDB(t *testing.T) {
	_, ts, _ := newServer(t)
	defer ts.Close()

	// config validation under an unknown name
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/config?db=newdb&dryRun=1", strings.NewReader("name: newdb\ndescription: test\n"))
	req.Header.Set("Content-Type", "application/yaml")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("config dry run on an unknown db = %d, want 200: %s", res.StatusCode, b)
	}

	// import preview under an unknown name
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files[]", "note.md")
	fw.Write([]byte("# Note\n\nThe margin floor is 32%.\n"))
	mw.WriteField("dryRun", "true")
	mw.Close()
	res2, err := http.Post(ts.URL+"/api/import?db=newdb", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("import dry run on an unknown db = %d, want 200: %s", res2.StatusCode, b)
	}

	// the dry runs must not have created anything
	res3, err := http.Get(ts.URL + "/api/state?db=newdb")
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusNotFound {
		t.Fatalf("state on the unknown db = %d, want 404", res3.StatusCode)
	}

	// and a real import under that name is still refused
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	fw2, _ := mw2.CreateFormFile("files[]", "note.md")
	fw2.Write([]byte("# Note\n"))
	mw2.Close()
	res4, err := http.Post(ts.URL+"/api/import?db=newdb", mw2.FormDataContentType(), &buf2)
	if err != nil {
		t.Fatal(err)
	}
	defer res4.Body.Close()
	if res4.StatusCode != http.StatusNotFound {
		t.Fatalf("real import on an unknown db = %d, want 404", res4.StatusCode)
	}
}

// A nil slice marshals to JSON null; the UI reads these as lists and crashed on null.
func TestInstallExampleAPI(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "examples", "mini")
	if err := os.MkdirAll(filepath.Join(src, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "seed.json"), `{"name":"Mini","memories":[{"id":"f1","kind":"fact","text":"sodium packs ship in Q4"}]}`)
	write(filepath.Join(src, "braindb.yaml"), "name: mini\ndescription: a tiny example\n")
	write(filepath.Join(src, "docs", "note.md"), "# Note\n\nPacks are late.\n")

	s := server.New("test")
	s.DataDir = filepath.Join(root, "data")
	s.ExamplesDir = filepath.Join(root, "examples")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { s.Close(); ts.Close() })

	c, body, _ := do(t, "GET", ts.URL+"/api/examples", "")
	if c != 200 || !strings.Contains(body, `"name":"mini"`) || !strings.Contains(body, "a tiny example") {
		t.Fatalf("examples: %d %s", c, body)
	}
	c, body, _ = do(t, "POST", ts.URL+"/api/dbs", `{"example":"mini","name":"packs"}`)
	if c != 200 || !strings.Contains(body, `"name":"packs"`) {
		t.Fatalf("install: %d %s", c, body)
	}
	c, body, _ = do(t, "GET", ts.URL+"/api/state?db=packs", "")
	if c != 200 || !strings.Contains(body, "sodium packs ship in Q4") {
		t.Fatalf("installed state: %d %s", c, body)
	}
	if c, body, _ = do(t, "POST", ts.URL+"/api/dbs", `{"example":"mini","config":"name: x\n"}`); c != 400 || !strings.Contains(body, "not both") {
		t.Fatalf("config+example: %d %s", c, body)
	}
	lone := server.New("test")
	lone.ExamplesDir = s.ExamplesDir
	ts2 := httptest.NewServer(lone.Handler())
	t.Cleanup(func() { lone.Close(); ts2.Close() })
	if c, body, _ = do(t, "POST", ts2.URL+"/api/dbs", `{"example":"mini"}`); c != 400 || !strings.Contains(body, "--data-dir") {
		t.Fatalf("single brain install: %d %s", c, body)
	}
}

func TestWhoamiListsAreNeverNull(t *testing.T) {
	_, ts, _ := newServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/whoami")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"roles", "labels", "dbs", "defaultWriteLabels"} {
		v, ok := raw[k]
		if !ok {
			t.Fatalf("whoami has no %q", k)
		}
		if string(v) == "null" {
			t.Errorf("whoami.%s is null, want a (possibly empty) list", k)
		}
		var list []string
		if err := json.Unmarshal(v, &list); err != nil {
			t.Errorf("whoami.%s does not decode as a list: %v", k, err)
		}
	}
}

func mcpSession(t *testing.T, url string, hdr ...string) string {
	t.Helper()
	h := append([]string{"Accept", "application/json"}, hdr...)
	c, b, rh := do(t, "POST", url+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, h...)
	if c != 200 {
		t.Fatalf("initialize: %d %s", c, b)
	}
	return rh.Get("Mcp-Session-Id")
}

func mcpCall(t *testing.T, url, sid, tool, args string, hdr ...string) string {
	t.Helper()
	h := append([]string{"Mcp-Session-Id", sid, "Accept", "application/json"}, hdr...)
	_, b, _ := do(t, "POST", url+"/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+tool+`","arguments":`+args+`}}`, h...)
	return b
}

func TestMCPBrainUseOverHTTPRespectsTokenScope(t *testing.T) {
	s := server.New("test")
	s.DataDir = t.TempDir()
	s.Hooks.Run = bdql.HTTPRun
	for _, n := range []string{"alpha", "beta"} {
		if _, err := s.CreateDB(n, ""); err != nil {
			t.Fatal(err)
		}
	}
	s.Tokens = []server.Token{
		{Name: "default", Value: "adm", Scope: "admin"},
		{Name: "bot", Value: "bot", Scope: "write", DBs: []string{"alpha"}},
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	defer s.Close()

	sid := mcpSession(t, ts.URL, "Authorization", "Bearer adm", "X-BrainDB-Database", "alpha")
	if out := mcpCall(t, ts.URL, sid, "brain_use", `{"name":"beta"}`, "Authorization", "Bearer adm", "X-BrainDB-Database", "alpha"); !strings.Contains(out, "now using beta") {
		t.Fatalf("admin switches: %s", out)
	}

	sid = mcpSession(t, ts.URL, "Authorization", "Bearer bot", "X-BrainDB-Database", "alpha")
	out := mcpCall(t, ts.URL, sid, "brain_use", `{"name":"beta"}`, "Authorization", "Bearer bot", "X-BrainDB-Database", "alpha")
	if strings.Contains(out, "now using beta") || !strings.Contains(out, "unknown tool") {
		t.Fatalf("a token scoped to one database sees only that database, so there is nothing to switch: %s", out)
	}
	if out := mcpCall(t, ts.URL, sid, "brain_run", `{"script":"USE beta; SELECT COUNT(*) FROM memory;"}`, "Authorization", "Bearer bot", "X-BrainDB-Database", "alpha"); !strings.Contains(out, "bound to one database") {
		t.Fatalf("USE must stay refused for a scoped token: %s", out)
	}
}
