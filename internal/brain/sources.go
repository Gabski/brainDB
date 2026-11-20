// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// EdgeExternal links a local node to a stub of a node in an external source.
const EdgeExternal = "external"

// Source is a mounted external source (Addendum C).
type Source struct {
	Name          string
	URI           string
	TokenEnv      string
	Weight        float64
	TTL           int
	Mode          string
	FederateDepth int
	Enabled       bool
}

// SourceConfig is the JSON/YAML form of a source.
type SourceConfig struct {
	Name          string   `json:"name"`
	URI           string   `json:"uri"`
	TokenEnv      string   `json:"tokenEnv,omitempty"`
	Weight        *float64 `json:"weight,omitempty"`
	TTL           *int     `json:"ttl,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	FederateDepth *int     `json:"federateDepth,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

func (c SourceConfig) toSource() *Source {
	s := &Source{Name: c.Name, URI: c.URI, TokenEnv: c.TokenEnv, Weight: .7, TTL: 300, Mode: "live", FederateDepth: 1, Enabled: true}
	if strings.HasPrefix(c.URI, "url+") {
		s.Mode, s.TTL, s.Weight, s.FederateDepth = "cached", 86400, .5, 0
	}
	if c.Weight != nil {
		s.Weight = clamp(*c.Weight)
	}
	if c.TTL != nil {
		s.TTL = *c.TTL
	}
	if c.Mode != "" {
		s.Mode = c.Mode
	}
	if c.FederateDepth != nil {
		s.FederateDepth = *c.FederateDepth
	}
	if c.Enabled != nil {
		s.Enabled = *c.Enabled
	}
	return s
}

func (s *Source) validate() error {
	if s.Name == "" || s.Name == "A" || strings.ContainsAny(s.Name, ": ,@") {
		return errf("invalid source name %q (must be non-empty, not \"A\", without ':', ',', '@' or spaces)", s.Name)
	}
	ok := false
	for _, p := range []string{"http://", "https://", "file://", "snap://", "exec://", "url+http://", "url+https://"} {
		if strings.HasPrefix(s.URI, p) {
			ok = true
		}
	}
	if !ok {
		return errf("source %s: unsupported uri %q", s.Name, s.URI)
	}
	return nil
}

func sourceFromXML(e *XMLEl) *Source {
	return &Source{Name: e.A("name"), URI: e.A("uri"), TokenEnv: e.A("token-env"), Weight: atof(e.A("weight"), .7), TTL: atoi(e.A("ttl"), 300),
		Mode: e.A("mode"), FederateDepth: atoi(e.A("federate-depth"), 1), Enabled: e.A("enabled") != "0"}
}

func (s *Source) xml() string {
	en := "1"
	if !s.Enabled {
		en = "0"
	}
	return `<source name="` + esc(s.Name) + `" uri="` + esc(s.URI) + `"` + xa("token-env", s.TokenEnv) + ` weight="` + fmtNum(s.Weight) +
		`" ttl="` + strconv.Itoa(s.TTL) + `"` + xa("mode", s.Mode) + ` federate-depth="` + strconv.Itoa(s.FederateDepth) + `" enabled="` + en + `"/>`
}

// Config returns the JSON form.
func (s *Source) Config() SourceConfig {
	w, t, f, e := s.Weight, s.TTL, s.FederateDepth, s.Enabled
	return SourceConfig{Name: s.Name, URI: s.URI, TokenEnv: s.TokenEnv, Weight: &w, TTL: &t, Mode: s.Mode, FederateDepth: &f, Enabled: &e}
}

func (b *Brain) source(name string) *Source {
	for _, s := range b.sources {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// isExternalID reports whether id is "name:remote" for a mounted source.
func (b *Brain) isExternalID(id string) bool {
	i := strings.Index(id, ":")
	return i > 0 && id[:i] != "A" && b.source(id[:i]) != nil
}

func (b *Brain) addStub(id, kind, text string, w float64) *Node {
	src := id[:strings.Index(id, ":")]
	if kind == "" {
		kind = "note"
	}
	if text == "" {
		text = id
	}
	n := &Node{ID: id, Kind: kind, Section: "external", Text: text, Base: w, Ext: &ExtRef{Source: src, W: w, Fetched: isoMs(b.nowMs())}}
	if _, ok := b.kindM[kind]; !ok {
		n.Kind = "note"
	}
	return b.addNode(n)
}

// ensureExternal creates a stub for an external id, fetching kind/text/weight when the source is reachable.
func (b *Brain) ensureExternal(id string) error {
	if b.nodes[id] != nil {
		return nil
	}
	i := strings.Index(id, ":")
	src := b.source(id[:i])
	if src == nil {
		return errf("no source %s", id[:i])
	}
	kind, text, w := "", "", .5
	if b.fed != nil && b.fed.fetch != nil {
		if k, t, ww, err := b.fed.fetch(src, id[i+1:]); err == nil {
			kind, text, w = k, t, ww
		} else {
			b.fed.warn(src.Name)
		}
	}
	b.addStub(id, kind, text, w)
	return nil
}

// federation holds runtime state for external sources (remote client hooks, warnings).
type federation struct {
	mu       sync.Mutex
	fetch    func(src *Source, remoteID string) (kind, text string, w float64, err error)
	warnings map[string]time.Time
}

func (f *federation) warn(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.warnings == nil {
		f.warnings = map[string]time.Time{}
	}
	f.warnings[name] = time.Now()
}

func (b *Brain) fedWarnings() []string {
	if b.fed == nil {
		return nil
	}
	b.fed.mu.Lock()
	defer b.fed.mu.Unlock()
	out := []string{}
	for _, k := range sortedKeys(b.fed.warnings) {
		if time.Since(b.fed.warnings[k]) < 5*time.Minute {
			out = append(out, "source "+k+" unreachable")
		}
	}
	return out
}

// SourceInfo is the status of a mounted source.
type SourceInfo struct {
	SourceConfig
	Status    string `json:"status"`
	LatencyMs int    `json:"latencyMs"`
	Nodes     int    `json:"nodes"`
	LastError string `json:"lastError,omitempty"`
	LastSync  string `json:"lastSync,omitempty"`
}

// SourceStatus lists mounted sources with their status.
func (b *Brain) SourceStatus() []SourceInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []SourceInfo{}
	warn := map[string]bool{}
	for _, w := range b.fedWarnings() {
		warn[strings.TrimSuffix(strings.TrimPrefix(w, "source "), " unreachable")] = true
	}
	for _, s := range b.sources {
		st := "ok"
		if !s.Enabled {
			st = "disabled"
		} else if warn[s.Name] {
			st = "error"
		}
		n := 0
		for _, id := range b.order {
			if x := b.nodes[id]; x.Ext != nil && x.Ext.Source == s.Name {
				n++
			}
		}
		out = append(out, SourceInfo{SourceConfig: s.Config(), Status: st, Nodes: n})
	}
	return out
}
