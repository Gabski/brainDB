// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Command braindb is the brainDB CLI: HTTP server + UI, MCP server, seeding, ingest, queries and template fill.
// Logs go to stderr only: stdout is the MCP channel in `braindb mcp`.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/mcp"
	"github.com/Gabski/brainDB/internal/server"
)

var version = "dev"

//go:embed NOTICE
var notice string

const (
	author  = "Gabriel Koziestański"
	license = "Apache-2.0"
	title   = "brainDB by Gabriel Koziestański"
)

func banner() string { return fmt.Sprintf("brainDB %s · © 2026 %s · %s", version, author, license) }

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, "[braindb] "+format+"\n", a...) }

func die(err error) {
	fmt.Fprintln(os.Stderr, "braindb: "+err.Error())
	os.Exit(1)
}

// ---------------------------------------------------------------- argument parsing (flags may follow positionals)

type flagSpec map[string]bool // name → takes a value

type args struct {
	vals map[string][]string
	pos  []string
}

func parse(in []string, spec flagSpec) (*args, error) {
	a := &args{vals: map[string][]string{}}
	for i := 0; i < len(in); i++ {
		s := in[i]
		if s == "--" {
			a.pos = append(a.pos, in[i+1:]...)
			break
		}
		if !strings.HasPrefix(s, "-") || s == "-" {
			a.pos = append(a.pos, s)
			continue
		}
		name := strings.TrimLeft(s, "-")
		val, hasVal := "", false
		if k, v, ok := strings.Cut(name, "="); ok {
			name, val, hasVal = k, v, true
		}
		takes, known := spec[name]
		if !known {
			return nil, fmt.Errorf("unknown flag %s", s)
		}
		if takes && !hasVal {
			if i+1 >= len(in) {
				return nil, fmt.Errorf("flag %s needs a value", s)
			}
			i++
			val = in[i]
		} else if !takes && !hasVal {
			val = "true"
		}
		a.vals[name] = append(a.vals[name], val)
	}
	return a, nil
}

func (a *args) str(name, def string) string {
	if v := a.vals[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return def
}

func (a *args) all(name string) []string { return a.vals[name] }

func (a *args) bool(name string) bool {
	v := a.str(name, "")
	return v == "true" || v == "1" || v == "yes"
}

func (a *args) has(name string) bool { return len(a.vals[name]) > 0 }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// brainPath resolves which brain file to open: --brain, then BRAINDB_BRAIN, then storage.path in the
// braindb.yaml sitting next to the default, then brain.xml. The file's extension picks the backend.
func brainPath(a *args) string {
	p := a.str("brain", env("BRAINDB_BRAIN", ""))
	if p != "" {
		return p
	}
	if cfg, _, err := brain.LoadConfigFile(brain.ConfigPath("brain.xml")); err == nil && cfg != nil && cfg.Storage.Path != "" {
		return cfg.Storage.Path
	}
	return "brain.xml"
}

// listenAddr resolves the listen address: --addr, then BRAINDB_ADDR, then server.addr in
// braindb.yaml, then the loopback default.
func listenAddr(a *args, st *brain.Store) string {
	def := "127.0.0.1:7791"
	if st != nil && st.Conf != nil && st.Conf.Server.Addr != "" {
		def = st.Conf.Server.Addr
	}
	addr := a.str("addr", env("BRAINDB_ADDR", def))
	if p := a.str("port", ""); p != "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = "127.0.0.1"
		}
		addr = net.JoinHostPort(host, p)
	}
	return addr
}

func openStore(path, seed string) *brain.Store {
	st, err := brain.OpenStore(path, seed)
	if err != nil {
		die(err)
	}
	st.Log = logf
	return st
}

func tokensFromEnv() []server.Token {
	ts := []server.Token{}
	if t := os.Getenv("BRAINDB_TOKEN"); t != "" {
		ts = append(ts, server.Token{Name: "default", Value: t, Scope: "admin"})
	}
	if t := os.Getenv("BRAINDB_TOKEN_READ"); t != "" {
		ts = append(ts, server.Token{Name: "read", Value: t, Scope: "read"})
	}
	return ts
}

// splitList turns a comma-separated environment value into a trimmed list.
func splitList(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// configTokens turns the server.tokens block of braindb.yaml into API tokens. Each entry names an
// environment variable; the value never lives in the config file.
func configTokens(st *brain.Store) []server.Token {
	out := []server.Token{}
	if st == nil || st.Conf == nil {
		return out
	}
	for _, t := range st.Conf.Server.Tokens {
		v := os.Getenv(t.TokenEnv)
		if t.TokenEnv == "" || v == "" {
			if t.TokenEnv != "" {
				logf("WARNING: server.tokens %q: %s is empty, the token is not active", t.Name, t.TokenEnv)
			}
			continue
		}
		scope := t.Scope
		switch scope {
		case "read", "write", "admin":
		case "":
			scope = "read"
		default:
			die(fmt.Errorf("server.tokens %q: scope must be read, write or admin, not %q", t.Name, t.Scope))
		}
		out = append(out, server.Token{Name: t.Name, Value: v, Scope: scope, DBs: t.DBs})
	}
	return out
}

// configServer applies the server block of braindb.yaml: the admin token env var, the extra tokens
// and allowNoToken. The listen address is handled by listenAddr, where the flag outranks it.
func configServer(s *server.Server, st *brain.Store) {
	if st == nil || st.Conf == nil {
		return
	}
	c := st.Conf.Server
	if c.TokenEnv != "" && os.Getenv("BRAINDB_TOKEN") == "" {
		if v := os.Getenv(c.TokenEnv); v != "" {
			s.Tokens = append(s.Tokens, server.Token{Name: "default", Value: v, Scope: "admin"})
		} else {
			logf("WARNING: server.tokenEnv names %s, which is empty", c.TokenEnv)
		}
	}
	s.Tokens = append(s.Tokens, configTokens(st)...)
	if c.AllowNoToken {
		s.AllowNone = true
	}
}

func allowNoToken(a *args) bool {
	return a.bool("allow-no-token") || env("BRAINDB_ALLOW_NO_TOKEN", "") == "1" || env("BRAINDB_ALLOW_NO_TOKEN", "") == "true"
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func writeOut(path, s string) {
	if path == "" || path == "-" {
		fmt.Print(s)
		if !strings.HasSuffix(s, "\n") {
			fmt.Println()
		}
		return
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		die(err)
	}
	logf("wrote %s", path)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// ---------------------------------------------------------------- commands

const usage = `usage: braindb <command> [flags]

commands:
  serve    --brain brain.xml | --data-dir DIR [--seed seed.json] [--addr 127.0.0.1:7791] [--port N] [--allow-no-token]
  mcp      --brain brain.xml [--actor claude] [--addr …|--port N] [--no-http] [--tools view,run|all] [--read-only]
  seed     SEED.json -o brain.xml [--ingest DIR]
  migrate  --brain brain.xml --to sqlite|file...
  state    --brain brain.xml
  context  --brain brain.xml [--format md|xml] [-o text.md]
  ingest   --brain brain.xml PATH... [--no-focus]
  query    --brain brain.xml 'kind:deal stage=negotiation sort:-value' [--format json|yaml|xml|md|csv]
  query    --brain brain.xml --context [--query "renewal risk"] [--focus ids] [--depth N] [--format xml|md|json|yaml]
  fill     --brain brain.xml TEMPLATE [--output same|json|yaml|xml|md] [--sources] [--scope memory|context] [--min 0.2]
  db       ls|create NAME|rm NAME --data-dir DIR
  version  (also --version [--notice])

environment: BRAINDB_BRAIN, BRAINDB_ADDR, BRAINDB_DATA_DIR, BRAINDB_TOKEN, BRAINDB_TOKEN_READ, BRAINDB_ALLOW_NO_TOKEN
`

// extraCommands lets optional files (BDQL, publish, …) add subcommands.
var extraCommands = map[string]func(argv []string){}

var extraUsage []string

func main() {
	argv := os.Args[1:]
	if runPublished(argv) {
		return
	}
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, banner())
		fmt.Fprint(os.Stderr, usage+strings.Join(extraUsage, ""))
		os.Exit(2)
	}
	cmd, rest := argv[0], argv[1:]
	switch cmd {
	case "--version", "-version", "version", "-v":
		fmt.Println(banner())
		for _, x := range rest {
			if x == "--notice" {
				fmt.Println()
				fmt.Print(notice)
			}
		}
	case "help", "--help", "-h":
		fmt.Println(banner())
		fmt.Print(usage + strings.Join(extraUsage, ""))
	case "serve":
		cmdServe(rest)
	case "mcp":
		cmdMCP(rest)
	case "seed":
		cmdSeed(rest)
	case "state":
		cmdState(rest)
	case "context":
		cmdContext(rest)
	case "ingest":
		cmdIngest(rest)
	case "query":
		cmdQuery(rest)
	case "fill":
		cmdFill(rest)
	case "db":
		cmdDB(rest)
	case "migrate":
		cmdMigrate(rest)
	default:
		if f, ok := extraCommands[cmd]; ok {
			f(rest)
			return
		}
		fmt.Fprintf(os.Stderr, "braindb: unknown command %q\n\n%s", cmd, usage+strings.Join(extraUsage, ""))
		os.Exit(2)
	}
}

// runPublished is replaced by the publish feature; it returns true when this binary ran in published mode.
var runPublished = func(argv []string) bool { return false }

// configure is a hook applied to every opened store (braindb.yaml, patterns, …).
var configure = func(st *brain.Store) {}

func newServer() *server.Server {
	s := server.New(version)
	s.Author, s.License, s.Log, s.MCPTitle = author, license, logf, title
	s.Tokens = tokensFromEnv()
	s.Hooks = serverHooks
	s.AllowedOrigins = splitList(env("BRAINDB_ALLOWED_ORIGINS", ""))
	return s
}

var serverHooks server.Hooks

func cmdServe(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "seed": true, "addr": true, "port": true, "data-dir": true, "db": true, "allow-no-token": false, "read-only": false, "tools": true})
	if err != nil {
		die(err)
	}
	s := newServer()
	s.AllowNone = allowNoToken(a)
	s.ReadOnly = a.bool("read-only")
	if t := a.str("tools", ""); t != "" {
		if s.MCPTools, err = mcp.ResolveTools(strings.Split(t, ",")); err != nil {
			die(err)
		}
	}
	dataDir := a.str("data-dir", "")
	if dataDir == "" && !a.has("brain") && os.Getenv("BRAINDB_BRAIN") == "" {
		dataDir = os.Getenv("BRAINDB_DATA_DIR")
	}
	if dataDir != "" {
		s.DataDir = dataDir
		if err := s.LoadDataDir(); err != nil {
			die(err)
		}
		if len(s.DBs()) == 0 {
			st, err := s.CreateDB("default", "")
			if err != nil {
				die(err)
			}
			if seed := a.str("seed", ""); seed != "" {
				if err := st.B.LoadSeedFile(seed); err != nil {
					die(err)
				}
				_ = st.Save()
			}
		}
		if d := a.str("db", ""); d != "" {
			s.Default = d
		}
	} else {
		p := brainPath(a)
		st := openStore(p, a.str("seed", ""))
		name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		s.AddDB(name, st)
	}
	for _, n := range s.DBs() {
		st := s.Store(n)
		configure(st)
		st.Watch(700 * time.Millisecond)
		configServer(s, st)
		s.Tokens = append(s.Tokens, userTokens(st)...)
	}
	addr := listenAddr(a, s.Store(s.Default))
	ctx, cancel := signalContext()
	defer cancel()
	logf("%s", banner())
	logf("serving %s on http://%s (auth: %s)", strings.Join(s.DBs(), ", "), addr, s.AuthMode())
	if err := s.ListenAndServe(ctx, addr); err != nil && !errors.Is(err, context.Canceled) {
		die(err)
	}
	logf("stopped")
}

func cmdMCP(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "actor": true, "addr": true, "port": true, "no-http": false, "tools": true, "read-only": false,
		"allow-no-token": false, "seed": true, "snapshot": true, "db": true, "data-dir": true, "user": true, "token-env": true})
	if err != nil {
		die(err)
	}
	var st *brain.Store
	if snap := a.str("snapshot", ""); snap != "" {
		st = openSnapshot(snap)
	} else if dd := a.str("data-dir", ""); dd != "" && a.has("db") {
		st = openStore(filepath.Join(dd, a.str("db", ""), "brain.xml"), "")
	} else {
		st = openStore(brainPath(a), a.str("seed", ""))
	}
	configure(st)
	ms := &mcp.Server{Store: st, Actor: a.str("actor", env("BRAINDB_ACTOR", "agent")), Name: "braindb", Title: title, Version: version, Log: logf,
		ReadOnly: a.bool("read-only") || st.ReadOnly}
	if u := a.str("user", ""); u != "" || st.B.AccessEnabled() {
		p, err := stdioPrincipal(st, u, a.str("token-env", ""))
		if err != nil {
			die(err)
		}
		ms.Principal = p
		if !a.has("actor") && p.Name != "" {
			ms.Actor = p.Name
		}
	}
	if t := a.str("tools", ""); t != "" {
		if ms.Tools, err = mcp.ResolveTools(strings.Split(t, ",")); err != nil {
			die(err)
		}
	}
	ctx, cancel := signalContext()
	defer cancel()
	httpUp := false
	if !a.bool("no-http") {
		s := newServer()
		s.AllowNone = allowNoToken(a)
		s.ReadOnly = ms.ReadOnly
		s.AddDB(st.Name, st)
		configServer(s, st)
		addr := listenAddr(a, st)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			logf("HTTP on %s unavailable (%v): another process serves the UI; changes sync through %s", addr, err, filepath.Base(st.Path))
		} else if !server.IsLoopback(addr) && len(s.Tokens) == 0 && !s.AllowNone {
			ln.Close()
			logf("not starting HTTP on non-loopback %s without a token", addr)
		} else {
			httpUp = true
			s.LoopbackOnly = server.IsLoopback(addr)
			ms.UIURL = "http://" + addr
			logf("live view: http://%s", addr)
			go func() { _ = s.Serve(ctx, ln) }()
		}
	}
	st.Watch(700 * time.Millisecond)
	err = ms.Serve(ctx, os.Stdin, os.Stdout)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		logf("mcp: %v", err)
	}
	if httpUp && ctx.Err() == nil {
		logf("stdin closed; HTTP view keeps running (Ctrl+C to stop)")
		<-ctx.Done()
	}
	if !st.ReadOnly && st.Path != "" {
		_ = st.Save()
	}
}

// openSnapshot is replaced by the snapshot feature.
var openSnapshot = func(path string) *brain.Store {
	die(fmt.Errorf("snapshots are not supported in this build"))
	return nil
}

func cmdSeed(argv []string) {
	a, err := parse(argv, flagSpec{"o": true, "out": true, "ingest": true, "brain": true})
	if err != nil {
		die(err)
	}
	if len(a.pos) != 1 {
		die(errors.New("usage: braindb seed SEED.json -o brain.xml [--ingest DIR]"))
	}
	out := a.str("o", a.str("out", a.str("brain", "brain.xml")))
	b := brain.New()
	if err := b.LoadSeedFile(a.pos[0]); err != nil {
		die(err)
	}
	st := brain.NewMemStore(b, strings.TrimSuffix(filepath.Base(out), filepath.Ext(out)))
	st.Path = out
	// the example's braindb.yaml (next to the seed) configures kinds, patterns and access; copy it next to the brain
	if cp := brain.ConfigPath(a.pos[0]); fileExists(cp) {
		dst := brain.ConfigPath(out)
		if !fileExists(dst) {
			if d, err := os.ReadFile(cp); err == nil {
				_ = os.MkdirAll(filepath.Dir(dst), 0o755)
				_ = os.WriteFile(dst, d, 0o644)
			}
		}
	}
	if err := st.LoadConfig(false); err != nil {
		die(err)
	}
	configure(st)
	for _, d := range a.all("ingest") {
		r, err := b.IngestPath(d, brain.IngestOptions{Focus: false})
		if err != nil {
			die(err)
		}
		for _, l := range r.Report {
			logf("%s", l)
		}
	}
	if err := st.Save(); err != nil {
		die(err)
	}
	n, e := b.Counts()
	logf("seeded %s: %d nodes, %d edges · %s", out, n, e, b.Status(""))
}

func cmdState(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true})
	if err != nil {
		die(err)
	}
	st := openExisting(brainPath(a))
	printJSON(st.B.State())
}

func openExisting(p string) *brain.Store {
	if _, err := os.Stat(p); err != nil {
		die(fmt.Errorf("no brain at %s (create one with braindb seed or braindb serve --seed)", p))
	}
	st := openStore(p, "")
	configure(st)
	return st
}

func cmdContext(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "format": true, "o": true, "out": true})
	if err != nil {
		die(err)
	}
	st := openExisting(brainPath(a))
	var s string
	switch a.str("format", "xml") {
	case "md", "markdown":
		s = st.B.ContextMD()
	case "xml":
		s = st.B.ContextXML()
	default:
		die(errors.New("--format must be md or xml"))
	}
	writeOut(a.str("o", a.str("out", "")), s)
}

func cmdIngest(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "no-focus": false, "max-files": true})
	if err != nil {
		die(err)
	}
	if len(a.pos) == 0 {
		die(errors.New("usage: braindb ingest --brain brain.xml PATH..."))
	}
	st := openExisting(brainPath(a))
	max, _ := strconv.Atoi(a.str("max-files", "200"))
	for _, p := range a.pos {
		err := st.Mutate("cli", "ingest", func(b *brain.Brain) (string, []string, error) {
			r, err := b.IngestPath(p, brain.IngestOptions{Focus: !a.bool("no-focus"), MaxFiles: max})
			if err != nil {
				return "", nil, err
			}
			for _, l := range r.Report {
				fmt.Println(l)
			}
			return strings.Join(r.Report, "\n"), r.Fresh, nil
		})
		if err != nil {
			die(err)
		}
	}
	fmt.Println(st.B.Status(""))
}

func cmdQuery(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "format": true, "context": false, "query": true, "focus": true, "depth": true, "threshold": true, "hop": true, "o": true})
	if err != nil {
		die(err)
	}
	st := openExisting(brainPath(a))
	if a.bool("context") {
		cq := brain.ContextQuery{Query: a.str("query", strings.Join(a.pos, " ")), Format: a.str("format", "xml")}
		if f := a.str("focus", ""); f != "" {
			cq.Focus = strings.Split(f, ",")
		}
		if d := a.str("depth", ""); d != "" {
			n, _ := strconv.Atoi(d)
			cq.Depth = &n
		}
		if t := a.str("threshold", ""); t != "" {
			v, _ := strconv.ParseFloat(t, 64)
			cq.Threshold = &v
		}
		if h := a.str("hop", ""); h != "" {
			v, _ := strconv.ParseFloat(h, 64)
			cq.Hop = &v
		}
		r, err := st.B.QueryContext(cq)
		if err != nil {
			die(err)
		}
		writeOut(a.str("o", ""), r.Output)
		return
	}
	q, err := brain.ParseQuery(strings.Join(a.pos, " "))
	if err != nil {
		die(err)
	}
	r, err := st.B.RunQuery(q, a.str("format", "md"))
	if err != nil {
		die(err)
	}
	writeOut(a.str("o", ""), r.Rendered)
	logf("%d of %d rows", r.Count, r.Total)
}

func cmdFill(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "output": true, "sources": false, "scope": true, "min": true, "format": true, "o": true, "json": false})
	if err != nil {
		die(err)
	}
	if len(a.pos) != 1 {
		die(errors.New("usage: braindb fill --brain brain.xml TEMPLATE [--output json]"))
	}
	tpl, err := os.ReadFile(a.pos[0])
	if err != nil {
		die(err)
	}
	st := openExisting(brainPath(a))
	req := brain.FillRequest{Template: string(tpl), TemplateFormat: a.str("format", ""), Output: a.str("output", ""), Scope: a.str("scope", ""), WithSources: a.bool("sources")}
	if req.TemplateFormat == "" {
		switch strings.ToLower(filepath.Ext(a.pos[0])) {
		case ".json":
			req.TemplateFormat = "json"
		case ".yaml", ".yml":
			req.TemplateFormat = "yaml"
		case ".xml":
			req.TemplateFormat = "xml"
		case ".md", ".markdown", ".txt":
			req.TemplateFormat = "md"
		}
	}
	if m := a.str("min", ""); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		req.Min = &v
	}
	r, err := st.B.Fill(req)
	if err != nil {
		die(err)
	}
	if a.bool("json") {
		printJSON(r)
		return
	}
	writeOut(a.str("o", ""), r.Output)
	if len(r.Missing) > 0 {
		logf("missing slots (%d): %s", len(r.Missing), strings.Join(r.Missing, ", "))
	}
}

func cmdDB(argv []string) {
	a, err := parse(argv, flagSpec{"data-dir": true, "config": true, "yes": false})
	if err != nil {
		die(err)
	}
	if len(a.pos) == 0 {
		die(errors.New("usage: braindb db ls|create NAME|rm NAME [--data-dir DIR]"))
	}
	s := newServer()
	s.DataDir = a.str("data-dir", env("BRAINDB_DATA_DIR", "./data"))
	if err := s.LoadDataDir(); err != nil {
		die(err)
	}
	switch a.pos[0] {
	case "ls", "list":
		for _, n := range s.DBs() {
			st := s.Store(n)
			nodes, edges := st.B.Counts()
			fmt.Printf("%-20s %6d nodes %6d edges  step %d  %s\n", n, nodes, edges, st.B.CurrentStep(), st.Path)
		}
	case "create":
		if len(a.pos) != 2 {
			die(errors.New("usage: braindb db create NAME [--config braindb.yaml]"))
		}
		cfg := ""
		if c := a.str("config", ""); c != "" {
			d, err := os.ReadFile(c)
			if err != nil {
				die(err)
			}
			cfg = string(d)
		}
		st, err := s.CreateDB(a.pos[1], cfg)
		if err != nil {
			die(err)
		}
		fmt.Println("created", st.Path)
	case "rm", "remove":
		if len(a.pos) != 2 {
			die(errors.New("usage: braindb db rm NAME --yes"))
		}
		if !a.bool("yes") {
			die(fmt.Errorf("add --yes to delete database %s", a.pos[1]))
		}
		dir := filepath.Join(s.DataDir, a.pos[1])
		if _, err := os.Stat(filepath.Join(dir, "brain.xml")); err != nil {
			die(fmt.Errorf("no database %s in %s", a.pos[1], s.DataDir))
		}
		trash := filepath.Join(s.DataDir, ".deleted-"+a.pos[1]+"-"+strconv.FormatInt(time.Now().Unix(), 10))
		if err := os.Rename(dir, trash); err != nil {
			die(err)
		}
		fmt.Println("moved to", trash)
	default:
		die(fmt.Errorf("unknown db command %q", a.pos[0]))
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// userTokens turns configured users with a token env var into API tokens.
func userTokens(st *brain.Store) []server.Token {
	out := []server.Token{}
	a := st.B.Access()
	if a == nil {
		return out
	}
	for _, u := range a.Users {
		if u.TokenEnv == "" || u.Disabled {
			continue
		}
		v := os.Getenv(u.TokenEnv)
		if v == "" || v == os.Getenv("BRAINDB_TOKEN") && u.TokenEnv != "BRAINDB_TOKEN" {
			continue
		}
		if u.TokenEnv == "BRAINDB_TOKEN" {
			continue // already the server-wide admin token
		}
		p, err := a.Principal(u.Name)
		if err != nil {
			continue
		}
		scope := "read"
		if p.Admin {
			scope = "admin"
		} else if st.B.CanWriteAny(p, st.Name) {
			scope = "write"
		}
		out = append(out, server.Token{Name: u.Name, Value: v, Scope: scope, DBs: u.DBs})
	}
	return out
}

// stdioPrincipal resolves the user of a stdio MCP session. With access control enabled a user is required unless
// BRAINDB_TOKEN (admin) is present; the user's token must be in its tokenEnv (or the --token-env variable).
func stdioPrincipal(st *brain.Store, user, tokenEnv string) (*brain.Principal, error) {
	if !st.B.AccessEnabled() {
		return brain.AdminPrincipal, nil
	}
	if user == "" {
		return brain.AdminPrincipal, nil // the local operator of a stdio session
	}
	a := st.B.Access()
	u := a.User(user)
	if u == nil {
		return nil, fmt.Errorf("no such user %s", user)
	}
	want := ""
	if u.TokenEnv != "" {
		want = os.Getenv(u.TokenEnv)
	}
	got := want
	if tokenEnv != "" {
		got = os.Getenv(tokenEnv)
	} else if v := os.Getenv("BRAINDB_TOKEN_" + strings.ToUpper(strings.ReplaceAll(user, "-", "_"))); v != "" {
		got = v
	}
	if u.TokenEnv != "" && (want == "" || got != want) {
		return nil, fmt.Errorf("user %s: token not found in %s (or --token-env)", user, u.TokenEnv)
	}
	return a.Principal(user)
}

// cmdMigrate copies a brain between the file and SQLite backends. Both directions are lossless, and
// the source file is left untouched.
func cmdMigrate(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "to": true})
	if err != nil {
		die(err)
	}
	to := a.str("to", "")
	if to == "" {
		die(errors.New("usage: braindb migrate --brain brain.xml --to sqlite|file"))
	}
	st := openStore(brainPath(a), "")
	configure(st)
	out, err := st.MigrateTo(to)
	if err != nil {
		die(err)
	}
	n, e := st.B.Counts()
	logf("migrated %s to %s: %s (%d nodes, %d edges)", st.Path, to, out, n, e)
}
