// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Package server is brainDB's HTTP API: JSON endpoints, server-sent events, MCP over Streamable HTTP and the
// embedded web UI.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/mcp"
)

//go:embed all:ui/dist
var uiFS embed.FS

// Token is an API token with a scope.
type Token struct {
	Name  string
	Value string
	Scope string   // read | write | admin
	DBs   []string // empty = all
}

func scopeRank(s string) int {
	switch s {
	case "admin":
		return 3
	case "write":
		return 2
	case "read":
		return 1
	}
	return 0
}

// Hooks lets optional packages (BDQL, storage, federation) add endpoints without import cycles.
type Hooks struct {
	Run     func(st *brain.Store, script string, readOnly, explain bool, actor string, p *brain.Principal) (any, error)
	Stats   func(st *brain.Store) any
	Convert func(st *brain.Store, to string) (any, error)
}

// Server serves one or more brain databases.
type Server struct {
	Version   string
	Author    string
	License   string
	DataDir   string // multi-db root ("" = single db)
	Default   string
	Tokens    []Token
	MCPName   string
	MCPTitle  string
	MCPTools  []string
	ReadOnly  bool
	Log       func(format string, a ...any)
	Hooks     Hooks
	AllowNone bool
	// AllowedOrigins are browser origins allowed to call the API cross-origin ("*" allows any).
	// Empty means same-origin only, which is what a local UI needs.
	AllowedOrigins []string
	// LoopbackOnly is set when the listener is bound to a loopback address. The Host header is then
	// required to be a loopback name too, which is what stops DNS rebinding: a page on evil.com
	// resolved to 127.0.0.1 reaches the socket, but arrives with Host: evil.com.
	LoopbackOnly bool

	mu       sync.RWMutex
	dbs      map[string]*brain.Store
	order    []string
	started  time.Time
	sessMu   sync.Mutex
	sessions map[string]*mcpSession
	done     chan struct{}
}

// New creates a server.
func New(version string) *Server {
	return &Server{Version: version, dbs: map[string]*brain.Store{}, started: time.Now(), sessions: map[string]*mcpSession{}, done: make(chan struct{}),
		Log: func(string, ...any) {}, MCPName: "braindb"}
}

// AddDB registers a database.
func (s *Server) AddDB(name string, st *brain.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.dbs[name]; !ok {
		s.order = append(s.order, name)
	}
	st.Name = name
	s.dbs[name] = st
	if s.Default == "" {
		s.Default = name
	}
}

// DBs returns the database names.
func (s *Server) DBs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.order...)
}

// Store returns a database by name ("" = default).
func (s *Server) Store(name string) *brain.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if name == "" {
		name = s.Default
	}
	return s.dbs[name]
}

// LoadDataDir opens every subdirectory of DataDir holding a brain.xml.
func (s *Server) LoadDataDir() error {
	if s.DataDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		return err
	}
	es, err := os.ReadDir(s.DataDir)
	if err != nil {
		return err
	}
	for _, e := range es {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(s.DataDir, e.Name(), "brain.xml")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		st, err := brain.OpenStore(p, "")
		if err != nil {
			s.Log("skip db %s: %v", e.Name(), err)
			continue
		}
		st.Log = s.Log
		s.AddDB(e.Name(), st)
	}
	return nil
}

// Close stops watchers and closes SSE streams.
func (s *Server) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	for _, n := range s.DBs() {
		if st := s.Store(n); st != nil {
			st.Close()
		}
	}
}

// ---------------------------------------------------------------- helpers

type ctxKey int

const (
	keyToken ctxKey = iota
	keyPrincipal
)

func principalOf(r *http.Request) *brain.Principal {
	if p, ok := r.Context().Value(keyPrincipal).(*brain.Principal); ok {
		return p
	}
	return brain.AdminPrincipal
}

// principal resolves the request's principal: the token's user when access control is enabled.
func (s *Server) principal(r *http.Request, st *brain.Store) (*brain.Principal, error) {
	if !st.B.AccessEnabled() {
		return brain.AdminPrincipal, nil
	}
	t := tokenOf(r)
	if t == nil || t.Name == "local" || t.Name == "default" {
		// no tokens configured, or the server-wide admin token: admin (may preview a user with X-BrainDB-User)
		if u := firstNonEmpty(r.Header.Get("X-BrainDB-View-As"), r.Header.Get("X-BrainDB-User")); u != "" && u != "admin" {
			return st.B.PrincipalFor(u)
		}
		return brain.AdminPrincipal, nil
	}
	return st.B.PrincipalFor(t.Name)
}

// viewStore wraps the principal's filtered view as a read-only store.
func (s *Server) viewStore(st *brain.Store, p *brain.Principal) *brain.Store {
	st.Sync()
	vs := brain.NewMemStore(st.B.ViewFor(p, st.Name), st.Name)
	vs.ReadOnly = true
	vs.Path = ""
	return vs
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeText(w http.ResponseWriter, ctype, body string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, body)
}

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("invalid JSON body: %v", err)
	}
	return nil
}

func dbName(r *http.Request) string {
	if d := r.URL.Query().Get("db"); d != "" {
		return d
	}
	return r.Header.Get("X-BrainDB-Database")
}

func tokenOf(r *http.Request) *Token {
	t, _ := r.Context().Value(keyToken).(*Token)
	return t
}

func actorOf(r *http.Request, def string) string {
	if a := r.Header.Get("X-BrainDB-Actor"); a != "" {
		return a
	}
	if a := r.Header.Get("X-Client"); a != "" {
		return a
	}
	if t := tokenOf(r); t != nil && t.Name != "" && t.Name != "default" {
		return t.Name
	}
	return def
}

// AuthMode describes the configured authentication.
func (s *Server) AuthMode() string {
	if len(s.Tokens) > 0 {
		return "token"
	}
	return "none"
}

func (s *Server) authenticate(r *http.Request) (*Token, bool) {
	if len(s.Tokens) == 0 {
		return &Token{Name: "local", Scope: "admin"}, true
	}
	v := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		v = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	} else {
		v = r.URL.Query().Get("token")
	}
	if v == "" {
		return nil, false
	}
	for i := range s.Tokens {
		t := &s.Tokens[i]
		if subtle.ConstantTimeCompare([]byte(t.Value), []byte(v)) == 1 {
			return t, true
		}
	}
	return nil, false
}

// requiredScope maps an endpoint to the scope it needs.
func requiredScope(r *http.Request) string {
	p := r.URL.Path
	switch {
	case p == "/api/dbs" && r.Method == http.MethodPost, strings.HasPrefix(p, "/api/dbs/") && r.Method == http.MethodDelete,
		// Writing the config means rewriting the access block, so it is admin whichever verb carries
		// it: the handler accepts POST as well as PUT.
		p == "/api/config" && r.Method != http.MethodGet && r.Method != http.MethodHead,
		p == "/api/storage/convert":
		return "admin"
	case r.Method == http.MethodGet, r.Method == http.MethodHead:
		return "read"
	case p == "/api/search", p == "/api/query", p == "/api/context/query", p == "/api/fill", p == "/api/run/explain", p == "/api/patterns/test", p == "/api/run", p == "/mcp":
		return "read" // /api/run and /mcp downgrade to read-only execution for read tokens
	}
	return "write"
}

// ---------------------------------------------------------------- request guards

// bodyTypeOK reports whether a Content-Type may carry a write. The three types a cross-origin HTML
// form can send without a CORS preflight (text/plain, application/x-www-form-urlencoded and a
// multipart form) are exactly the ones a CSRF attack uses, so writes must name a type a form cannot.
func bodyTypeOK(ct, path string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "application/json", "application/xml", "text/xml", "application/yaml", "text/yaml", "application/x-yaml":
		return true
	case "multipart/form-data":
		// Uploads have no other shape; the Origin check below is what protects this one.
		return path == "/api/ingest" || path == "/api/import"
	}
	return false
}

func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

func hostIsLoopback(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	if strings.EqualFold(h, "localhost") || strings.HasSuffix(strings.ToLower(h), ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) originAllowed(origin, host string) bool {
	if sameOrigin(origin, host) {
		return true
	}
	for _, a := range s.AllowedOrigins {
		if a == "*" || strings.EqualFold(strings.TrimSuffix(a, "/"), strings.TrimSuffix(origin, "/")) {
			return true
		}
	}
	return false
}

// withGuards rejects cross-site requests before they reach authentication. Without it a page the
// operator happens to visit can POST to the local server: no token is configured by default, and a
// form-shaped body needs no preflight, so the write simply lands.
func (s *Server) withGuards(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/") && p != "/mcp" {
			next.ServeHTTP(w, r)
			return
		}
		if s.LoopbackOnly && r.Host != "" && !hostIsLoopback(r.Host) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden: this server listens on loopback and only answers to a loopback Host, not " + r.Host})
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "null" && !s.originAllowed(o, r.Host) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden: cross-origin request from " + o + " (set BRAINDB_ALLOWED_ORIGINS to allow it)"})
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			ct := r.Header.Get("Content-Type")
			if ct == "" && r.ContentLength == 0 {
				break // no body to smuggle; the Origin check above covers the action itself
			}
			if !bodyTypeOK(ct, p) {
				writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
					"error": "unsupported Content-Type " + strconv.Quote(ct) + ": writes must send application/json"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/") && p != "/mcp" || p == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		t, ok := s.authenticate(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="braindb"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized: send Authorization: Bearer <token> or ?token="})
			return
		}
		if scopeRank(t.Scope) < scopeRank(requiredScope(r)) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": fmt.Sprintf("forbidden: token %q has scope %s, this needs %s", t.Name, t.Scope, requiredScope(r))})
			return
		}
		if len(t.DBs) > 0 {
			db := dbName(r)
			if db == "" {
				db = s.Default
			}
			allowed := false
			for _, d := range t.DBs {
				if d == db || d == "*" {
					allowed = true
				}
			}
			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: token not valid for database " + db})
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyToken, t)))
	})
}

func canWrite(r *http.Request) bool {
	t := tokenOf(r)
	return t == nil || scopeRank(t.Scope) >= 2
}

// ---------------------------------------------------------------- routes

// isDryRun reports whether the request only previews: ?dryRun=1|true, or the multipart field of
// the same name that POST /api/import uses. Parsing the form here is safe — ParseMultipartForm is
// idempotent, so the handler's own call returns the form this one already read.
func isDryRun(r *http.Request) bool {
	switch q := r.URL.Query().Get("dryRun"); q {
	case "1", "true":
		return true
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(256 << 20); err == nil {
			v := r.FormValue("dryRun")
			return v == "true" || v == "1"
		}
	}
	return false
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// scratch: when set, a dry run addressed to a database that does not exist yet is served on a
	// throwaway in-memory brain instead of 404ing. The "new database" wizard needs that: it previews
	// an import and validates a config under the new name before the database is created.
	register := func(path string, scratch bool, h func(w http.ResponseWriter, r *http.Request, st *brain.Store)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			st := s.Store(dbName(r))
			if st == nil && scratch && dbName(r) != "" && isDryRun(r) {
				st = brain.NewMemStore(brain.New(), dbName(r))
			}
			if st == nil {
				writeErr(w, http.StatusNotFound, fmt.Errorf("no database %q (have: %s)", dbName(r), strings.Join(s.DBs(), ", ")))
				return
			}
			p, err := s.principal(r, st)
			if err != nil || !p.CanDB(st.Name) {
				writeErr(w, http.StatusNotFound, fmt.Errorf("no database %q", dbName(r)))
				return
			}
			if !p.Admin {
				switch r.URL.Path {
				case "/api/brain.xml", "/api/brain", "/api/kinds", "/api/embedding", "/api/storage/convert", "/api/config", "/api/import":
					// reading the kind table or the list of embedding providers leaks nothing; writing needs admin
					readable := r.Method == http.MethodGet && (r.URL.Path == "/api/kinds" || r.URL.Path == "/api/embedding")
					if !readable {
						writeErr(w, http.StatusForbidden, errors.New("access denied: this endpoint needs admin"))
						return
					}
				case "/api/exec", "/api/context", "/api/ingest", "/api/run", "/api/run/explain", "/api/events", "/api/journal", "/api/whoami", "/api/snapshot", "/api/migrate",
					"/api/quarantine", "/api/quarantine/admit", "/api/quarantine/reject", "/api/purpose":
					// handled with the principal
				default:
					st = s.viewStore(st, p)
				}
			}
			h(w, r.WithContext(context.WithValue(r.Context(), keyPrincipal, p)), st)
		})
	}
	api := func(path string, h func(w http.ResponseWriter, r *http.Request, st *brain.Store)) {
		register(path, false, h)
	}
	mux.HandleFunc("/api/health", s.health)
	api("/api/state", s.state)
	api("/api/context", s.context)
	api("/api/embedding", s.embedding)
	api("/api/exec", s.exec)
	api("/api/search", s.search)
	api("/api/inspect", s.inspect)
	api("/api/layout", s.layout)
	api("/api/vector", s.vector)
	api("/api/ingest", s.ingest)
	api("/api/kinds", s.kinds)
	api("/api/context.xml", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		st.Sync()
		writeText(w, "application/xml; charset=utf-8", st.B.ContextXML())
	})
	api("/api/context.md", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		st.Sync()
		writeText(w, "text/markdown; charset=utf-8", st.B.ContextMD())
	})
	api("/api/brain.xml", s.brainXML)
	api("/api/brain", s.brainXML)
	api("/api/events", s.events)
	api("/api/journal", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		evs := st.Journal(100)
		p := principalOf(r)
		for i := range evs {
			evs[i] = filterEvent(st, p, evs[i])
		}
		writeJSON(w, 200, evs)
	})
	api("/api/whoami", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		p := principalOf(r)
		// A nil slice marshals to JSON null, and clients that expect a list then break on it.
		list := func(v []string) []string {
			if v == nil {
				return []string{}
			}
			return v
		}
		// "name" is what the UI reads; "user" stays for older clients. The grants matter too: a role
		// may carry the review grant without "review" in its name, and the UI cannot guess that.
		writeJSON(w, 200, map[string]any{
			"name": p.Name, "user": p.Name, "display": p.Display,
			"admin": p.Admin, "roles": list(p.Roles), "labels": list(p.Labels), "dbs": list(p.DBs),
			"defaultWriteLabels": list(p.Default),
			"access":             st.B.AccessEnabled(), "db": st.Name,
			"can": map[string]bool{
				"review":          p.Admin || p.Can("review", "*", "", "", st.Name),
				"purposeOverride": p.Admin || p.Can("purpose-override", "*", "", "", st.Name),
			},
		})
	})
	register("/api/import", true, s.importHandler)
	api("/api/purpose", s.purposeHandler)
	api("/api/purpose/check", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		var req struct {
			Text    string       `json:"text"`
			Kind    string       `json:"kind"`
			Section string       `json:"section"`
			Fields  brain.Fields `json:"fields"`
			// Optional: what the item would be linked to once stored. A real REMEMBER is scored with
			// its links, so without these the preview is a lower bound on the score.
			LinkTo []string `json:"linkTo,omitempty"`
			Links  int      `json:"links,omitempty"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, st.B.CheckPurposeWith(req.Text, req.Kind, req.Section, req.Fields, req.LinkTo, req.Links))
	})
	api("/api/quarantine", s.quarantineHandler)
	api("/api/quarantine/admit", s.quarantineHandler)
	api("/api/quarantine/reject", s.quarantineHandler)
	register("/api/config", true, s.configHandler)
	api("/api/query", s.query)
	api("/api/context/query", s.contextQuery)
	api("/api/fill", s.fill)
	api("/api/templates", s.templates)
	api("/api/sources", s.sources)
	api("/api/sources/", s.sources)
	api("/api/patterns", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		if r.Method == http.MethodPost {
			s.patternsToggle(w, r, st)
			return
		}
		s.patternsList(w, r, st)
	})
	api("/api/snapshot", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		if !method(w, r, http.MethodPost) {
			return
		}
		if err := st.Save(); err != nil {
			writeErr(w, 500, err)
			return
		}
		s.stats(w, r, st)
	})
	api("/api/migrate", func(w http.ResponseWriter, r *http.Request, st *brain.Store) {
		if !method(w, r, http.MethodPost) {
			return
		}
		var req struct {
			To string `json:"to"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		out, err := st.MigrateTo(req.To)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "backend": req.To, "path": out})
	})
	api("/api/patterns/test", s.patternsTest)
	api("/api/entities", s.entities)
	api("/api/run", func(w http.ResponseWriter, r *http.Request, st *brain.Store) { s.run(w, r, st, false) })
	api("/api/run/explain", func(w http.ResponseWriter, r *http.Request, st *brain.Store) { s.run(w, r, st, true) })
	api("/api/stats", s.stats)
	api("/api/storage/convert", s.convert)
	mux.HandleFunc("/api/dbs", s.dbsHandler)
	mux.HandleFunc("/api/dbs/", s.dbsHandler)
	mux.HandleFunc("/mcp", s.mcpHTTP)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such endpoint %s", r.URL.Path))
	})
	mux.Handle("/", s.ui())
	return s.withGuards(s.withAuth(mux))
}

func (s *Server) ui() http.Handler {
	sub, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(sub, p); err != nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		// Asset file names carry a content hash, so they may be cached forever. index.html names
		// them, so it must never be cached: a stale copy keeps serving the previous UI build
		// (and 404s once the old assets are gone) long after the server was upgraded.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func method(w http.ResponseWriter, r *http.Request, ms ...string) bool {
	for _, m := range ms {
		if r.Method == m {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(ms, ", "))
	writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s not allowed", r.Method))
	return false
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	st := s.Store(dbName(r))
	out := map[string]any{"status": "ok", "version": s.Version, "uptimeSec": int(time.Since(s.started).Seconds()), "auth": s.AuthMode(),
		"author": s.Author, "license": s.License, "storage": "file", "dbs": len(s.DBs()),
		// false when the server was started with --brain: it serves one file and cannot create or
		// delete databases, so a client should not offer that.
		"manageDbs": s.DataDir != ""}
	if st != nil {
		n, _ := st.B.Counts()
		out["storage"] = st.BackendName()
		out["name"], out["nodes"], out["step"], out["db"] = st.B.State().Name, n, st.B.CurrentStep(), st.Name
		if out["name"] == "" {
			out["name"] = st.Name
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	st.Sync()
	writeJSON(w, 200, st.B.State())
}

func (s *Server) mutate(w http.ResponseWriter, r *http.Request, st *brain.Store, tool string, fn func(b *brain.Brain) (string, []string, error), resp func() any) {
	if s.ReadOnly || st.ReadOnly {
		writeErr(w, http.StatusForbidden, brain.ErrReadOnly)
		return
	}
	if err := st.Mutate(actorOf(r, "ui"), tool, fn); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, 200, resp())
}

func (s *Server) context(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var c brain.ContextChange
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err)
		return
	}
	p := principalOf(r)
	s.mutate(w, r, st, "context", func(b *brain.Brain) (string, []string, error) {
		if !p.Admin {
			ids := []string{}
			if c.Focus != nil {
				ids = append(ids, (*c.Focus)...)
			}
			ids = append(ids, c.AddFocus...)
			if err := b.CheckOps(p, st.Name, []brain.Op{{Op: "focus", IDs: ids}}); err != nil {
				return "", nil, err
			}
		}
		if _, err := b.SetContext(c); err != nil {
			return "", nil, err
		}
		p := b.State().Params
		return fmt.Sprintf("context: focus %s · depth %d · threshold %v", strings.Join(b.Focus(), ","), p.Depth, p.Threshold), b.Focus(), nil
	}, func() any { return st.B.ViewFor(p, st.Name).State() })
}

func (s *Server) embedding(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	// GET lists the providers the server accepts, so the settings form cannot offer one the loader would reject.
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		writeJSON(w, 200, map[string]any{"providers": brain.EmbeddingProviders()})
		return
	}
	if !method(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	var c brain.EmbeddingChange
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.mutate(w, r, st, "embedding", func(b *brain.Brain) (string, []string, error) {
		return "embedding settings changed", nil, b.SetEmbedding(c)
	}, func() any { return st.B.State() })
}

func (s *Server) exec(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Ops []brain.Op `json:"ops"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	var res *brain.ExecResult
	p := principalOf(r)
	s.mutate(w, r, st, "exec", func(b *brain.Brain) (string, []string, error) {
		if !p.Admin {
			for i := range req.Ops {
				if req.Ops[i].Op == "remember" {
					req.Ops[i].Labels = append(req.Ops[i].Labels, p.Default...)
				}
			}
			if err := b.CheckOps(p, st.Name, req.Ops); err != nil {
				return "", nil, err
			}
		}
		x, err := b.Exec(req.Ops)
		if err != nil {
			return "", nil, err
		}
		res = x
		return strings.Join(x.Changes, " "), x.Touched, nil
	}, func() any {
		res.Status = st.B.Status("")
		return res
	})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Query   string        `json:"query"`
		Mode    string        `json:"mode"`
		K       int           `json:"k"`
		Focus   bool          `json:"focus"`
		Sources brain.StrList `json:"sources"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	st.Sync()
	res, err := st.B.Search(req.Query, req.Mode, req.K)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.Focus && canWrite(r) && !st.ReadOnly && !s.ReadOnly {
		_ = st.Mutate(actorOf(r, "ui"), "search", func(b *brain.Brain) (string, []string, error) {
			b.SearchFocus(res)
			return fmt.Sprintf("%s: %q → focus", res.Mode, req.Query), b.Focus(), nil
		})
	}
	writeJSON(w, 200, res)
}

func (s *Server) inspect(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	q := r.URL.Query()
	what := brain.StrList(nil)
	for _, x := range q["what"] {
		what = append(what, strings.Split(x, ",")...)
	}
	st.Sync()
	res, err := st.B.Inspect(q.Get("id"), what, q.Get("to"))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) layout(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	st.Sync()
	res, err := st.B.Layout(r.URL.Query().Get("view"), r.URL.Query().Get("root"))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) vector(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	st.Sync()
	res, err := st.B.Vector(r.URL.Query().Get("id"), r.URL.Query().Get("full") == "1")
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, 400, fmt.Errorf("multipart form: %v", err))
		return
	}
	files := []brain.InFile{}
	for _, key := range []string{"files[]", "files", "file"} {
		for _, fh := range r.MultipartForm.File[key] {
			f, err := fh.Open()
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			d, err := io.ReadAll(io.LimitReader(f, 50<<20))
			f.Close()
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			files = append(files, brain.InFile{Name: fh.Filename, Data: d})
		}
	}
	if len(files) == 0 {
		writeErr(w, 400, fmt.Errorf("no files (multipart field files[])"))
		return
	}
	focus := r.FormValue("focus") != "false"
	var res *brain.IngestResult
	s.mutate(w, r, st, "ingest", func(b *brain.Brain) (string, []string, error) {
		x, err := b.Ingest(files, brain.IngestOptions{Focus: focus})
		if err != nil {
			return "", nil, err
		}
		res = x
		return strings.Join(x.Report, "\n"), x.Fresh, nil
	}, func() any { res.Status = st.B.Status(""); return res })
}

func (s *Server) kinds(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, st.B.State().Kinds)
		return
	}
	if !method(w, r, http.MethodPost) {
		return
	}
	var k brain.NewKind
	if err := readJSON(r, &k); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.mutate(w, r, st, "kinds", func(b *brain.Brain) (string, []string, error) {
		key, err := b.AddKind(k)
		return "new kind " + key, nil, err
	}, func() any { return st.B.State() })
}

func (s *Server) brainXML(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	switch r.Method {
	case http.MethodGet:
		st.Sync()
		writeText(w, "application/xml; charset=utf-8", st.B.XML())
	case http.MethodPut, http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		s.mutate(w, r, st, "brain.xml", func(b *brain.Brain) (string, []string, error) {
			tmp := brain.New()
			tmp.Now = b.Now
			if err := tmp.LoadXML(string(body)); err != nil {
				return "", nil, err
			}
			if err := b.LoadXML(string(body)); err != nil {
				return "", nil, err
			}
			return "brain replaced from XML: focus " + strings.Join(b.Focus(), ","), nil, nil
		}, func() any { return st.B.State() })
	default:
		method(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, cancel := st.Subscribe()
	defer cancel()
	hello, _ := json.Marshal(brain.Event{Type: "hello", TS: time.Now().UnixMilli(), By: "server", IDs: []string{}, DB: st.Name})
	fmt.Fprintf(w, "data: %s\n\n", hello)
	fl.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case <-tick.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			fl.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			ev = filterEvent(st, principalOf(r), ev)
			d, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", d)
			fl.Flush()
		}
	}
}

func (s *Server) query(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Query   json.RawMessage `json:"query"`
		Format  string          `json:"format"`
		Sources brain.StrList   `json:"sources"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	var qs string
	if len(req.Query) > 0 && req.Query[0] == '"' {
		_ = json.Unmarshal(req.Query, &qs)
	} else {
		qs = string(req.Query)
	}
	q, err := brain.ParseQuery(qs)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if len(req.Sources) > 0 {
		q.Sources = req.Sources
	}
	st.Sync()
	res, err := st.B.RunQuery(q, req.Format)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) contextQuery(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var c brain.ContextQuery
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err)
		return
	}
	st.Sync()
	res, err := st.B.QueryContext(c)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) fill(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req brain.FillRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	st.Sync()
	res, err := st.B.Fill(req)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

// TemplateInfo is one template file.
type TemplateInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

// ListTemplates finds templates next to the brain file and in examples/*/templates.
func ListTemplates(brainPath string) []TemplateInfo {
	dirs := []string{}
	if brainPath != "" {
		dirs = append(dirs, filepath.Join(filepath.Dir(brainPath), "templates"))
	}
	if m, _ := filepath.Glob("examples/*/templates"); m != nil {
		dirs = append(dirs, m...)
	}
	out := []TemplateInfo{}
	seen := map[string]bool{}
	for _, d := range dirs {
		es, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range es {
			if e.IsDir() {
				continue
			}
			p := filepath.Join(d, e.Name())
			if abs, err := filepath.Abs(p); err == nil {
				if seen[abs] {
					continue
				}
				seen[abs] = true
			}
			c, err := os.ReadFile(p)
			if err != nil || len(c) > 1<<20 {
				continue
			}
			f := strings.TrimPrefix(strings.ToLower(filepath.Ext(e.Name())), ".")
			switch f {
			case "yml":
				f = "yaml"
			case "markdown", "txt":
				f = "md"
			}
			out = append(out, TemplateInfo{Name: e.Name(), Path: p, Format: f, Content: string(c)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (s *Server) templates(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	writeJSON(w, 200, ListTemplates(st.Path))
}

// sources lists the federated sources, and adds, removes or syncs one under /api/sources/<name>.
func (s *Server) sources(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sources"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, st.B.SourceStatus())
		case http.MethodPost:
			var c brain.SourceConfig
			if err := readJSON(r, &c); err != nil {
				writeErr(w, 400, err)
				return
			}
			s.mutate(w, r, st, "sources", func(b *brain.Brain) (string, []string, error) {
				return "+source:" + c.Name, nil, b.AddSource(c)
			}, func() any { return st.B.SourceStatus() })
		default:
			method(w, r, http.MethodGet, http.MethodPost)
		}
		return
	}
	name, action, _ := strings.Cut(rest, "/")
	name, err := url.PathUnescape(name)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodDelete:
		s.mutate(w, r, st, "sources", func(b *brain.Brain) (string, []string, error) {
			return "-source:" + name, nil, b.RemoveSource(name)
		}, func() any { return st.B.SourceStatus() })
	case action == "sync" && r.Method == http.MethodPost:
		// Remote transports are not built yet (README "Federation": partial), so say that plainly
		// instead of reporting a sync that never reached anything.
		writeErr(w, http.StatusNotImplemented, errors.New("source sync is not available in this build: remote transports are not implemented yet"))
	case action == "search" && r.Method == http.MethodGet:
		writeErr(w, http.StatusNotImplemented, errors.New("federated search is not available in this build: remote transports are not implemented yet"))
	default:
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such endpoint %s", r.URL.Path))
	}
}

func (s *Server) patternsList(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	writeJSON(w, 200, st.B.Patterns())
}

func (s *Server) patternsTest(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	var req struct {
		Text  string        `json:"text"`
		Types brain.StrList `json:"types"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"entities": st.B.ExtractText(req.Text, req.Types)})
}

// entities lists the extracted entities of the brain, optionally narrowed by type or value.
func (s *Server) entities(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	spec := &brain.SelectSpec{From: "entities", Columns: []brain.SelCol{{Col: "id"}, {Col: "node"}, {Col: "type"},
		{Col: "value"}, {Col: "raw"}, {Col: "confidence"}, {Col: "pii"}}}
	typ, val := q.Get("type"), q.Get("value")
	if typ != "" || val != "" {
		spec.Where = func(get func(string) any) (bool, error) {
			if typ != "" && !st.B.CompareValues(get("type"), "=", typ) {
				return false, nil
			}
			if val != "" && !st.B.CompareValues(get("value"), "~", val) {
				return false, nil
			}
			return true, nil
		}
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		spec.Limit, spec.MaxRows = n, n
	}
	res, err := st.B.Select(spec)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res.Rows)
}

func (s *Server) run(w http.ResponseWriter, r *http.Request, st *brain.Store, explain bool) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if s.Hooks.Run == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("BDQL is not available in this build"))
		return
	}
	var req struct {
		Script string `json:"script"`
		Query  string `json:"query"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.Script == "" {
		req.Script = req.Query
	}
	res, err := s.Hooks.Run(st, req.Script, !canWrite(r) || s.ReadOnly || st.ReadOnly, explain, actorOf(r, "ui"), principalOf(r))
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error(), "result": res})
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	st.Sync()
	if s.Hooks.Stats != nil {
		writeJSON(w, 200, s.Hooks.Stats(st))
		return
	}
	n, e := st.B.Counts()
	files := []map[string]any{}
	if fi, err := os.Stat(st.Path); err == nil {
		files = append(files, map[string]any{"name": filepath.Base(st.Path), "bytes": fi.Size()})
	}
	format := "xml"
	if st.BackendName() == "sqlite" {
		format = "sqlite"
	}
	writeJSON(w, 200, map[string]any{"db": st.Name, "nodes": n, "edges": e, "step": st.B.CurrentStep(), "format": format, "files": files, "storage": st.BackendName()})
}

func (s *Server) convert(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if s.Hooks.Convert == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("storage conversion is not available in this build"))
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := s.Hooks.Convert(st, req.To)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

// ---------------------------------------------------------------- databases

// DBInfo summarises one database.
type DBInfo struct {
	Name      string `json:"name"`
	Nodes     int    `json:"nodes"`
	Edges     int    `json:"edges"`
	Step      int    `json:"step"`
	SizeBytes int64  `json:"sizeBytes"`
	Updated   string `json:"updated"`
	Goal      string `json:"goal"`
}

func validDBName(n string) bool {
	if n == "" || len(n) > 64 {
		return false
	}
	for _, r := range n {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// CreateDB creates an empty database in DataDir.
func (s *Server) CreateDB(name, configYAML string) (*brain.Store, error) {
	if s.DataDir == "" {
		return nil, errors.New("this server was started with --brain; use --data-dir to manage databases")
	}
	if !validDBName(name) {
		return nil, fmt.Errorf("invalid database name %q (letters, digits, - and _)", name)
	}
	if s.Store(name) != nil {
		return nil, fmt.Errorf("database %s already exists", name)
	}
	dir := filepath.Join(s.DataDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if configYAML != "" {
		if err := os.WriteFile(filepath.Join(dir, "braindb.yaml"), []byte(configYAML), 0o644); err != nil {
			return nil, err
		}
	}
	st, err := brain.OpenStore(filepath.Join(dir, "brain.xml"), "")
	if err != nil {
		return nil, err
	}
	st.B.Name = name
	_ = st.Save()
	st.Log = s.Log
	s.AddDB(name, st)
	return st, nil
}

func (s *Server) dbsHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/dbs"), "/")
	switch {
	case r.Method == http.MethodGet && name == "":
		out := []DBInfo{}
		for _, n := range s.DBs() {
			st := s.Store(n)
			nodes, edges := st.B.Counts()
			info := DBInfo{Name: n, Nodes: nodes, Edges: edges, Step: st.B.CurrentStep(), Goal: st.B.GoalText()}
			if fi, err := os.Stat(st.Path); err == nil {
				info.SizeBytes, info.Updated = fi.Size(), fi.ModTime().UTC().Format(time.RFC3339)
			}
			out = append(out, info)
		}
		writeJSON(w, 200, out)
	case r.Method == http.MethodPost && name == "":
		var req struct {
			Name   string `json:"name"`
			Config string `json:"config"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		st, err := s.CreateDB(req.Name, req.Config)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"name": st.Name, "path": st.Path})
	case r.Method == http.MethodDelete && name != "":
		if r.URL.Query().Get("confirm") != name {
			writeErr(w, 400, fmt.Errorf("add ?confirm=%s to delete this database", name))
			return
		}
		st := s.Store(name)
		if st == nil || s.DataDir == "" {
			writeErr(w, 404, fmt.Errorf("no database %s", name))
			return
		}
		st.Close()
		s.mu.Lock()
		delete(s.dbs, name)
		ord := []string{}
		for _, n := range s.order {
			if n != name {
				ord = append(ord, n)
			}
		}
		s.order = ord
		if s.Default == name {
			s.Default = ""
			if len(ord) > 0 {
				s.Default = ord[0]
			}
		}
		s.mu.Unlock()
		dir := filepath.Join(s.DataDir, name)
		trash := filepath.Join(s.DataDir, ".deleted-"+name+"-"+strconv.FormatInt(time.Now().Unix(), 10))
		if err := os.Rename(dir, trash); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": name, "movedTo": trash})
	default:
		method(w, r, http.MethodGet, http.MethodPost, http.MethodDelete)
	}
}

// ---------------------------------------------------------------- MCP over Streamable HTTP

type mcpSession struct {
	srv  *mcp.Server
	last time.Time
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) mcpHTTP(w http.ResponseWriter, r *http.Request) {
	sid := r.Header.Get("Mcp-Session-Id")
	switch r.Method {
	case http.MethodDelete:
		s.sessMu.Lock()
		_, ok := s.sessions[sid]
		delete(s.sessions, sid)
		s.sessMu.Unlock()
		if !ok {
			writeErr(w, 404, errors.New("unknown session"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet:
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			writeErr(w, http.StatusMethodNotAllowed, errors.New("GET /mcp needs Accept: text/event-stream"))
			return
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			writeErr(w, 500, errors.New("streaming unsupported"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		fl.Flush()
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-s.done:
				return
			case <-t.C:
				_, _ = io.WriteString(w, ": ping\n\n")
				fl.Flush()
			}
		}
	case http.MethodPost:
	default:
		method(w, r, http.MethodPost, http.MethodGet, http.MethodDelete)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	trim := strings.TrimSpace(string(body))
	msgs := []json.RawMessage{}
	batch := strings.HasPrefix(trim, "[")
	if batch {
		if err := json.Unmarshal(body, &msgs); err != nil {
			writeJSON(w, 400, map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}})
			return
		}
	} else {
		msgs = append(msgs, json.RawMessage(body))
	}
	isInit := false
	for _, m := range msgs {
		var h struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(m, &h)
		if h.Method == "initialize" {
			isInit = true
		}
	}
	s.sessMu.Lock()
	sess := s.sessions[sid]
	s.sessMu.Unlock()
	if sess == nil {
		if !isInit && sid != "" {
			writeErr(w, 404, errors.New("unknown or expired session; initialize again"))
			return
		}
		st := s.Store(dbName(r))
		if st == nil {
			writeErr(w, 404, fmt.Errorf("no database %q", dbName(r)))
			return
		}
		p, err := s.principal(r, st)
		if err != nil || !p.CanDB(st.Name) {
			writeErr(w, 404, fmt.Errorf("no database %q", dbName(r)))
			return
		}
		ms := &mcp.Server{Store: st, Actor: actorOf(r, "agent"), Name: s.MCPName, Title: s.MCPTitle, Version: s.Version, Tools: s.MCPTools,
			ReadOnly: s.ReadOnly || !canWrite(r), Log: s.Log, Principal: p}
		sess = &mcpSession{srv: ms}
		if isInit {
			sid = newSessionID()
			s.sessMu.Lock()
			s.sessions[sid] = sess
			s.sessMu.Unlock()
		}
	}
	sess.last = time.Now()
	if sid != "" {
		w.Header().Set("Mcp-Session-Id", sid)
	}
	outs := []json.RawMessage{}
	for _, m := range msgs {
		if o := sess.srv.Handle(m); o != nil {
			outs = append(outs, o)
		}
	}
	if len(outs) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/event-stream") && !strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		for _, o := range outs {
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", o)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if batch {
		d, _ := json.Marshal(outs)
		_, _ = w.Write(d)
		return
	}
	_, _ = w.Write(outs[0])
}

// ---------------------------------------------------------------- listening

// IsLoopback reports whether addr only listens on a loopback interface.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ListenAndServe serves until ctx is done, then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.LoopbackOnly = IsLoopback(addr)
	if !s.LoopbackOnly && len(s.Tokens) == 0 {
		if !s.AllowNone {
			return fmt.Errorf("refusing to listen on non-loopback address %s without a token: set BRAINDB_TOKEN (or server.tokens), or pass --allow-no-token / BRAINDB_ALLOW_NO_TOKEN=1", addr)
		}
		s.Log("WARNING: listening on %s without authentication (--allow-no-token)", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve serves on a listener until ctx is done.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	hs := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	s.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = hs.Shutdown(sctx)
	for _, n := range s.DBs() {
		if st := s.Store(n); st != nil && !st.ReadOnly {
			_ = st.Save()
		}
	}
	return nil
}

// filterEvent removes ids the principal cannot see; a summary of a change that touched hidden ids is replaced.
func filterEvent(st *brain.Store, p *brain.Principal, ev brain.Event) brain.Event {
	if p == nil || p.Admin || !st.B.AccessEnabled() {
		return ev
	}
	ids, hidden := st.B.VisibleIDs(p, st.Name, ev.IDs)
	if hidden {
		ev.Summary = "[restricted change]"
	}
	if ids == nil {
		ids = []string{}
	}
	ev.IDs = ids
	return ev
}

// importHandler serves POST /api/import (multipart files[] with relative paths, optional config yaml and dryRun).
func (s *Server) importHandler(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeErr(w, 400, fmt.Errorf("multipart form: %v", err))
		return
	}
	files := []brain.InFile{}
	for _, key := range []string{"files[]", "files", "file"} {
		for _, fh := range r.MultipartForm.File[key] {
			f, err := fh.Open()
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			d, err := io.ReadAll(io.LimitReader(f, 50<<20))
			f.Close()
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			rel := filepath.ToSlash(filepath.Clean("/" + fh.Filename))[1:]
			files = append(files, brain.InFile{Name: filepath.Base(rel), Data: d, Rel: rel})
		}
	}
	if len(files) == 0 {
		writeErr(w, 400, errors.New("no files (multipart field files[])"))
		return
	}
	cfg := st.Conf
	if c := r.FormValue("config"); strings.TrimSpace(c) != "" {
		parsed, errs := brain.ParseConfig(c)
		if len(errs) > 0 {
			writeJSON(w, 400, map[string]any{"error": "invalid config", "errors": errs})
			return
		}
		cfg = parsed
	}
	dry := r.FormValue("dryRun") == "true" || r.FormValue("dryRun") == "1"
	progress := func(done, total int, file string) {
		st.Broadcast(brain.Event{Type: "import.progress", TS: time.Now().UnixMilli(), By: actorOf(r, "ui"), Tool: "import", Summary: file, IDs: []string{}, DB: st.Name,
			Done: done, Total: total, File: file})
	}
	if dry {
		rep, err := st.B.Import(files, brain.ImportOptions{DryRun: true, Config: cfg, Progress: progress})
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, rep)
		return
	}
	var rep *brain.ImportReport
	s.mutate(w, r, st, "import", func(b *brain.Brain) (string, []string, error) {
		x, err := b.Import(files, brain.ImportOptions{Update: r.FormValue("update") == "true", Config: cfg, Progress: progress})
		if err != nil {
			return "", nil, err
		}
		rep = x
		return fmt.Sprintf("import: %d files, %d nodes, %d edges", x.Totals.Files, x.Totals.Nodes, x.Totals.Edges), nil, nil
	}, func() any { return rep })
}

// configHandler serves GET/PUT /api/config: the YAML text, validation errors with line numbers and the effective config.
func (s *Server) configHandler(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	switch r.Method {
	case http.MethodGet:
		eff := any(nil)
		if st.Conf != nil {
			eff = st.Conf.Effective()
		}
		writeJSON(w, 200, map[string]any{"ok": true, "yaml": st.ConfText, "errors": []any{}, "effective": eff})
	case http.MethodPut, http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		text := string(body)
		var wrapped struct {
			YAML   string `json:"yaml"`
			DryRun bool   `json:"dryRun"`
		}
		if json.Unmarshal(body, &wrapped) == nil && wrapped.YAML != "" {
			text = wrapped.YAML
		}
		if q := r.URL.Query().Get("dryRun"); wrapped.DryRun || q == "true" || q == "1" {
			cfg, errs := brain.ParseConfig(text)
			if len(errs) > 0 {
				writeJSON(w, 200, map[string]any{"ok": false, "errors": errs, "effective": nil})
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "errors": []any{}, "effective": cfg.Effective()})
			return
		}
		cfg, errs, err := st.SaveConfigText(text)
		if len(errs) > 0 {
			writeJSON(w, 400, map[string]any{"ok": false, "errors": errs, "effective": nil})
			return
		}
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "errors": []any{}, "effective": cfg.Effective()})
	default:
		method(w, r, http.MethodGet, http.MethodPut)
	}
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// patternsToggle enables or disables a pattern type ({name, enabled}).
func (s *Server) patternsToggle(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	if !principalOf(r).Admin {
		writeErr(w, http.StatusForbidden, errors.New("access denied: this endpoint needs admin"))
		return
	}
	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		writeErr(w, 400, fmt.Errorf("body {name, enabled} required"))
		return
	}
	if err := st.SetPatternEnabled(req.Name, req.Enabled); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, st.B.Patterns())
}

// purposeHandler serves GET/PUT /api/purpose (PUT is admin-only and stored in braindb.yaml).
func (s *Server) purposeHandler(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"purpose": st.B.Purpose(), "counters": st.B.PurposeCounts()})
	case http.MethodPut, http.MethodPost:
		if !principalOf(r).Admin {
			writeErr(w, http.StatusForbidden, errors.New("access denied: this endpoint needs admin"))
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		var p brain.Purpose
		if err := json.Unmarshal(body, &p); err != nil {
			writeErr(w, 400, fmt.Errorf("purpose JSON: %v", err))
			return
		}
		if err := st.SetConfigKey("purpose", &p); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "purpose": st.B.Purpose()})
	default:
		method(w, r, http.MethodGet, http.MethodPut)
	}
}

// quarantineHandler serves GET /api/quarantine and POST /api/quarantine/admit|reject (reviewers only).
func (s *Server) quarantineHandler(w http.ResponseWriter, r *http.Request, st *brain.Store) {
	p := principalOf(r)
	if !p.Admin && !p.Can("review", "*", "", "", st.Name) {
		writeErr(w, http.StatusForbidden, errors.New("access denied: the review grant is required"))
		return
	}
	switch r.URL.Path {
	case "/api/quarantine":
		st.Sync()
		writeJSON(w, 200, st.B.QuarantineList())
		return
	}
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		IDs     []string `json:"ids"`
		Section string   `json:"section"`
		Reason  string   `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil || len(req.IDs) == 0 {
		writeErr(w, 400, errors.New("body {ids, section?|reason?} required"))
		return
	}
	admit := strings.HasSuffix(r.URL.Path, "/admit")
	err := st.Mutate(actorOf(r, "ui"), "quarantine", func(b *brain.Brain) (string, []string, error) {
		if admit {
			ids, err := b.Admit(req.IDs, req.Section)
			return "admitted " + strings.Join(ids, ","), ids, err
		}
		return "rejected " + strings.Join(req.IDs, ",") + " reason: " + req.Reason, nil, b.RejectQuarantined(req.IDs)
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "ids": req.IDs})
}
