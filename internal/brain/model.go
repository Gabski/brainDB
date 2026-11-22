// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Gabski/brainDB/internal/patterns"
)

// Kind describes a memory kind (built-in or custom).
type Kind struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Color    string   `json:"color"`
	Base     float64  `json:"base"`
	HalfLife *float64 `json:"halfLife"` // nil = never decays (concepts)
	// Floor is the fraction of Base that decay can never take away (0 = decays to nothing, the old behaviour).
	// Durable knowledge — facts, decisions, imported records — keeps a high floor: age should tilt ranking, not
	// erase truth. Only SUPERSEDE and FORGET remove knowledge. Attention-shaped kinds (task, question, note)
	// keep a floor of 0, because for them going quiet really does mean going away.
	Floor   float64 `json:"floor"`
	Prefix  string  `json:"prefix"`
	Section string  `json:"section"`
	Builtin bool    `json:"builtin"`
}

func (k *Kind) hl() float64 {
	if k.HalfLife == nil {
		return math.Inf(1)
	}
	return *k.HalfLife
}

// decayAt is the decay multiplier at an age in days: floor + (1-floor)·0.5^(age/half-life).
// With floor 0 this is the plain exponential half-life curve; with floor f it asymptotes to f instead of to 0,
// so old knowledge ranks below fresh knowledge without ever dropping out of the graph.
func (k *Kind) decayAt(ageDays float64) float64 {
	f := clamp01(k.Floor)
	return f + (1-f)*math.Pow(.5, ageDays/k.hl())
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func fp(x float64) *float64 { return &x }

// Concept is the kind name of concept nodes.
const Concept = "concept"

// Edge types.
const (
	EdgeConcept   = "concept"
	EdgeAssoc     = "assoc"
	EdgeSimilar   = "similar"
	EdgeSupersede = "supersede"
	EdgePart      = "part"
	EdgeSeq       = "seq"
	EdgeNew       = "new"
	EdgeConnector = "connector"
)

// ConnectorKind is the kind of connector nodes.
const ConnectorKind = "connector"

// MaxFanout limits how many neighbours a connector passes activation to.
var MaxFanout = 200

// DefaultSections is the default section order.
var DefaultSections = []string{"facts", "decisions", "tasks", "questions", "notes", "procedures", "ideas", "files"}

func builtinKinds() []*Kind {
	mk := func(n, c string, base, hl, floor float64, p, sec string) *Kind {
		k := &Kind{Name: n, Label: n, Color: c, Base: base, Floor: floor, Prefix: p, Section: sec, Builtin: true}
		if !math.IsInf(hl, 1) {
			k.HalfLife = fp(hl)
		}
		return k
	}
	return []*Kind{
		// durable: half-life tilts ranking toward the recent, the floor keeps the knowledge reachable forever
		mk("fact", "#4ea8ff", .6, 180, .70, "f", "facts"),
		mk("decision", "#ffa94d", .7, 365, .80, "d", "decisions"),
		mk("record", "#91a7ff", .5, 180, .70, "r", "records"),
		mk("file", "#f783ac", .5, 180, .60, "x", "files"),
		mk("chunk", "#94d82d", .45, 120, .55, "c", "files"),
		mk("summary", "#38d9a9", .6, 90, .40, "s", "procedures"),
		mk("idea", "#ffe066", .5, 60, .25, "i", "ideas"),
		// attention-shaped: these really do go away when nobody touches them
		mk("task", "#69db7c", .55, 14, 0, "t", "tasks"),
		mk("question", "#da77f2", .5, 7, 0, "q", "questions"),
		mk("note", "#adb5bd", .4, 7, 0, "n", "notes"),
		mk(Concept, "#e7f5ff", .8, math.Inf(1), 1, "A:", "concepts"),
		mk(ConnectorKind, "#868e96", 1, math.Inf(1), 1, "H:", "connectors"),
	}
}

// FileMeta describes an ingested file.
type FileMeta struct {
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

// Field is one named value of a record.
type Field struct{ Name, Value string }

// Fields is an ordered string map; it marshals as a JSON object preserving order.
type Fields []Field

func (f Fields) Get(name string) (string, bool) {
	for _, x := range f {
		if x.Name == name {
			return x.Value, true
		}
	}
	return "", false
}

func (f Fields) Set(name, value string) Fields {
	for i := range f {
		if f[i].Name == name {
			f[i].Value = value
			return f
		}
	}
	return append(f, Field{name, value})
}

func (f Fields) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, x := range f {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(x.Name)
		v, _ := json.Marshal(x.Value)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (f *Fields) UnmarshalJSON(data []byte) error {
	v, err := DecodeOrderedJSON(data)
	if err != nil {
		return err
	}
	m, ok := v.(*OMap)
	if !ok {
		if v == nil {
			*f = nil
			return nil
		}
		return fmt.Errorf("fields must be an object")
	}
	out := Fields{}
	for _, k := range m.Keys {
		out = append(out, Field{k, scalarString(m.Vals[k])})
	}
	*f = out
	return nil
}

func scalarString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return fmtNum(x)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// Node is a memory or a concept.
type Node struct {
	ID      string
	Kind    string
	Section string
	Text    string
	Base    float64
	Age     float64 // days
	Touched int64   // unix ms; 0 = unknown (derived from Age when saved)
	Pin     bool
	Sup     string // superseded by
	Meta    *FileMeta
	Parent  string
	Idx     int
	Fields  Fields
	Ext     *ExtRef   // non-nil for a cached stub of a node in an external source
	Conn    *ConnInfo // non-nil for a connector node (structural, knowledge-free)
	Labels  []string  // access labels (sorted); a principal must hold all of them
	Src     string    // import source locator (relative path + locator), for incremental import
	Hash    string    // content hash of the imported source
	Purpose *PurposeMark
}

// ConnInfo describes a connector node (id prefix "H:").
type ConnInfo struct {
	Type        string // anchor | bridge | hub | alias | time
	Key         string
	Gain        float64
	Transparent bool
	Terms       []string
	Source      string
}

func isConn(n *Node) bool { return n != nil && n.Conn != nil }

// ExtRef marks a stub of a remote node (id "source:remoteId").
type ExtRef struct {
	Source  string
	W       float64
	Fetched string
}

// Edge is a weighted synapse.
type Edge struct {
	A, B string
	W    float64
	Type string
}

func other(e *Edge, u string) string {
	if e.A == u {
		return e.B
	}
	return e.A
}

// Params are the activation parameters.
type Params struct {
	Depth          int     `json:"depth"`
	Threshold      float64 `json:"threshold"`
	Hop            float64 `json:"hop"`
	ShowSuperseded bool    `json:"showSuperseded"`
}

// DefaultParams are the reference implementation defaults.
var DefaultParams = Params{Depth: 2, Threshold: .16, Hop: .85}

// EmbConfig is the embedding configuration.
type EmbConfig struct {
	Model  string
	Dim    int
	Alpha  float64
	Hops   int
	Use    string // "raw" | "enriched"
	Lambda float64
}

// Brain is one long-term memory graph. All exported methods are safe for concurrent use.
type Brain struct {
	mu sync.Mutex

	Name string
	Goal string
	// WarnTokens is the budget.warnTokens of braindb.yaml: the status line warns above it (0 = 4000).
	WarnTokens int
	Dir        string // directory the brain lives in; used for the embedding cache. Not cleared by reset.
	Step       int
	P          Params
	Emb        EmbConfig
	Now        func() time.Time
	kinds      []*Kind
	kindM      map[string]*Kind
	secs       []string
	order      []string
	nodes      map[string]*Node
	edges      []*Edge
	adj        map[string][]*Edge
	seeds      []string
	goal       []string
	nextID     int

	cur   *actResult
	raw   map[string][]float32
	enr   map[string][]float32
	rawOr []string // insertion order of raw vectors (memories, then concepts)
	absC  []string // cached concept ids (nil = dirty)

	sources []*Source
	fed     *federation

	Pat            *patterns.Options // pattern library configuration
	Privacy        []string          // entity types masked in outputs (cards are always masked)
	RedactOnIngest []string          // entity types replaced before storing

	embedder Embedder // nil = built-in hash embedder
	entCache map[string]entCacheItem
	access   *AccessConfig
	views    map[string]viewEntry
	purpose  *Purpose
	pvecs    *purposeVecs

	// gen counts structural changes. The SQLite pre-filter (F4) may only narrow candidates while the
	// database still matches memory, so WriteSQLite records the generation it wrote and the planner
	// compares the two. A stale database would hide rows, which is worse than a full scan.
	gen   int64
	sqGen int64
	sq    *sql.DB
}

// touch marks the brain as changed since the last SQLite write.
func (b *Brain) touch() { b.gen++ }

// sqlReady reports whether the attached database still reflects memory.
func (b *Brain) sqlReady() bool { return b.sq != nil && b.gen == b.sqGen }

// New returns an empty brain with reference defaults.
func New() *Brain {
	b := &Brain{Now: time.Now}
	b.reset()
	return b
}

func (b *Brain) reset() {
	b.Name = ""
	b.Goal = ""
	b.Step = 1
	b.P = DefaultParams
	b.Emb = EmbConfig{Model: "hash-256", Dim: 256, Alpha: .5, Hops: 2, Use: "enriched", Lambda: .5}
	b.kinds = builtinKinds()
	b.kindM = map[string]*Kind{}
	for _, k := range b.kinds {
		b.kindM[k.Name] = k
	}
	b.secs = append([]string(nil), DefaultSections...)
	b.order = nil
	b.nodes = map[string]*Node{}
	b.edges = nil
	b.adj = map[string][]*Edge{}
	b.seeds = nil
	b.goal = nil
	b.nextID = 400
	b.cur = newAct()
	b.raw = map[string][]float32{}
	b.enr = map[string][]float32{}
	b.rawOr = nil
	b.absC = nil
	b.sources = nil
}

func (b *Brain) nowMs() int64 { return b.Now().UnixMilli() }

func (b *Brain) kind(name string) *Kind {
	if k, ok := b.kindM[name]; ok {
		return k
	}
	return b.kindM["note"]
}

// AddKind registers a custom kind (no-op when it exists). Returns the key.
func (b *Brain) addKind(k Kind) string {
	if k.Name == "" {
		return ""
	}
	if _, ok := b.kindM[k.Name]; ok {
		return k.Name
	}
	if k.Prefix == "" {
		used := map[string]bool{}
		for _, x := range b.kinds {
			used[x.Prefix] = true
		}
		p := k.Name[:1]
		for _, ch := range "abeghjklmoruvwyz" {
			if used[p] {
				p = string(ch)
			}
		}
		k.Prefix = p
	}
	if k.Label == "" {
		k.Label = k.Name
	}
	if k.Section == "" {
		k.Section = k.Label
	}
	if k.Color == "" {
		k.Color = "#adb5bd"
	}
	if k.Base == 0 {
		k.Base = .5
	}
	k.Builtin = false
	kk := k
	b.kinds = append(b.kinds, &kk)
	b.kindM[k.Name] = &kk
	return k.Name
}

func (b *Brain) addSection(s string) {
	if s == "" || s == "concepts" || s == "connectors" {
		return
	}
	for _, x := range b.secs {
		if x == s {
			return
		}
	}
	b.secs = append(b.secs, s)
}

// nodeW is the decayed weight.
func (b *Brain) nodeW(n *Node) float64 {
	if n.Kind == Concept {
		return n.Base
	}
	if n.Ext != nil {
		return n.Ext.W
	}
	if n.Conn != nil {
		return n.Conn.Gain
	}
	w := n.Base
	if n.Sup != "" {
		w *= 0.3
	}
	if !n.Pin {
		w *= b.kind(n.Kind).decayAt(n.Age)
	}
	return clamp(w)
}

func (b *Brain) addNode(n *Node) *Node {
	b.touch()
	if _, ok := b.nodes[n.ID]; !ok {
		b.order = append(b.order, n.ID)
	}
	if n.Section == "" {
		n.Section = b.kind(n.Kind).Section
	}
	b.nodes[n.ID] = n
	b.adj[n.ID] = nil
	if n.Kind == Concept {
		b.absC = nil
	}
	return n
}

func (b *Brain) newNode(id, kind, section, text string) *Node {
	k := b.kind(kind)
	return &Node{ID: id, Kind: kind, Section: section, Text: text, Base: k.Base}
}

func (b *Brain) edgeBetween(a, c string) *Edge {
	for _, e := range b.adj[a] {
		if e.A == c || e.B == c {
			return e
		}
	}
	return nil
}

func (b *Brain) addEdge(a, c string, w float64, typ string) *Edge {
	b.touch()
	if a == c || b.nodes[a] == nil || b.nodes[c] == nil {
		return nil
	}
	if ex := b.edgeBetween(a, c); ex != nil {
		ex.W = round2(math.Max(ex.W, clamp(w)))
		if typ == EdgeSupersede || typ == EdgeNew {
			ex.Type = typ
		}
		return ex
	}
	e := &Edge{A: a, B: c, W: round2(clamp(w)), Type: typ}
	b.edges = append(b.edges, e)
	b.adj[a] = append(b.adj[a], e)
	b.adj[c] = append(b.adj[c], e)
	return e
}

func without(list []*Edge, e *Edge) []*Edge {
	out := list[:0:0]
	for _, x := range list {
		if x != e {
			out = append(out, x)
		}
	}
	return out
}

func (b *Brain) removeEdge(e *Edge) {
	b.edges = without(b.edges, e)
	b.adj[e.A] = without(b.adj[e.A], e)
	b.adj[e.B] = without(b.adj[e.B], e)
}

func filterStr(xs []string, keep func(string) bool) []string {
	out := []string{}
	for _, x := range xs {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func (b *Brain) removeNode(id string, cascade bool) {
	b.touch()
	if cascade {
		for _, e := range append([]*Edge(nil), b.adj[id]...) {
			if e.Type == EdgePart && e.A == id {
				b.removeNode(e.B, false)
			}
		}
	}
	for _, e := range append([]*Edge(nil), b.adj[id]...) {
		b.removeEdge(e)
	}
	delete(b.nodes, id)
	delete(b.adj, id)
	b.order = filterStr(b.order, func(x string) bool { return x != id })
	b.absC = nil
	b.seeds = filterStr(b.seeds, func(x string) bool { return x != id })
	b.goal = filterStr(b.goal, func(x string) bool { return x != id })
	for _, x := range b.order {
		if n := b.nodes[x]; n.Sup == id {
			n.Sup = ""
		}
	}
}

// concepts returns concept ids in node order.
func (b *Brain) concepts() []string {
	if b.absC == nil {
		b.absC = []string{}
		for _, id := range b.order {
			if b.nodes[id].Kind == Concept {
				b.absC = append(b.absC, id)
			}
		}
	}
	return b.absC
}

func (b *Brain) memories() []string {
	return filterStr(b.order, func(id string) bool {
		n := b.nodes[id]
		return n.Kind != Concept && n.Ext == nil && n.Conn == nil && !isQuarantined(n)
	})
}

func tokensOf(n *Node) int { return int(math.Ceil(float64(jsLen(n.Text))/3.6)) + 18 }

func (b *Brain) newID(kind string) string {
	p := "n"
	if k, ok := b.kindM[kind]; ok {
		p = k.Prefix
	}
	for {
		id := p + strconv.Itoa(b.nextID)
		b.nextID++
		if b.nodes[id] == nil {
			return id
		}
	}
}

// recomputeNextID mirrors loadBrain: max(400, numeric suffixes) + 1.
func (b *Brain) recomputeNextID() {
	mx := 400.0
	for _, id := range b.order {
		i := 0
		for i < len(id) && !(id[i] >= '0' && id[i] <= '9') {
			i++
		}
		if v, ok := jsNumber(id[i:]); ok && v > mx {
			mx = v
		}
	}
	b.nextID = int(mx) + 1
}

// clone makes a deep copy of the mutable state (used as an exec snapshot).
func (b *Brain) clone() *Brain {
	c := &Brain{Name: b.Name, Goal: b.Goal, WarnTokens: b.WarnTokens, Step: b.Step, P: b.P, Emb: b.Emb, Now: b.Now, nextID: b.nextID, Pat: b.Pat, Privacy: b.Privacy, RedactOnIngest: b.RedactOnIngest, access: b.access, purpose: b.purpose}
	c.kinds = make([]*Kind, len(b.kinds))
	c.kindM = map[string]*Kind{}
	for i, k := range b.kinds {
		kk := *k
		c.kinds[i] = &kk
		c.kindM[k.Name] = &kk
	}
	c.secs = append([]string(nil), b.secs...)
	c.order = append([]string(nil), b.order...)
	c.nodes = make(map[string]*Node, len(b.nodes))
	for id, n := range b.nodes {
		nn := *n
		if n.Meta != nil {
			m := *n.Meta
			nn.Meta = &m
		}
		nn.Fields = append(Fields(nil), n.Fields...)
		nn.Labels = append([]string(nil), n.Labels...)
		if n.Purpose != nil {
			pm := *n.Purpose
			nn.Purpose = &pm
		}
		if n.Ext != nil {
			x := *n.Ext
			nn.Ext = &x
		}
		if n.Conn != nil {
			x := *n.Conn
			x.Terms = append([]string(nil), n.Conn.Terms...)
			nn.Conn = &x
		}
		c.nodes[id] = &nn
	}
	c.adj = make(map[string][]*Edge, len(b.adj))
	em := map[*Edge]*Edge{}
	for _, e := range b.edges {
		ee := *e
		em[e] = &ee
		c.edges = append(c.edges, &ee)
	}
	for id, l := range b.adj {
		nl := make([]*Edge, len(l))
		for i, e := range l {
			nl[i] = em[e]
		}
		c.adj[id] = nl
	}
	for _, s := range b.sources {
		ss := *s
		c.sources = append(c.sources, &ss)
	}
	c.fed = b.fed
	c.seeds = append([]string(nil), b.seeds...)
	c.goal = append([]string(nil), b.goal...)
	c.cur = b.cur
	c.raw, c.enr, c.rawOr = b.raw, b.enr, b.rawOr
	return c
}

// restore copies a snapshot back (keeps the mutex and clock).
func (b *Brain) restore(c *Brain) {
	b.Name, b.Goal, b.Step, b.P, b.Emb, b.nextID = c.Name, c.Goal, c.Step, c.P, c.Emb, c.nextID
	b.kinds, b.kindM, b.secs, b.order, b.nodes, b.edges, b.adj = c.kinds, c.kindM, c.secs, c.order, c.nodes, c.edges, c.adj
	b.seeds, b.goal, b.cur, b.raw, b.enr, b.rawOr = c.seeds, c.goal, c.cur, c.raw, c.enr, c.rawOr
	b.sources = c.sources
	b.absC = nil
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// stableSortDesc sorts ids by score descending, keeping order on ties (JS Array.sort is stable).
func stableSortDesc[T any](xs []T, key func(T) float64) {
	sort.SliceStable(xs, func(i, j int) bool { return key(xs[i]) > key(xs[j]) })
}

func errf(format string, a ...any) error { return fmt.Errorf(format, a...) }

func lower(s string) string { return strings.ToLower(s) }
