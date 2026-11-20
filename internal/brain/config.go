// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Gabski/brainDB/internal/patterns"
)

// Config is braindb.yaml (precedence: CLI flags > env > braindb.yaml > built-in defaults).
type Config struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Goal        string `json:"goal,omitempty"`
	Server      struct {
		Addr     string `json:"addr,omitempty"`
		TokenEnv string `json:"tokenEnv,omitempty"`
		Tokens   []struct {
			Name     string   `json:"name"`
			TokenEnv string   `json:"tokenEnv"`
			Scope    string   `json:"scope"`
			DBs      []string `json:"dbs,omitempty"`
		} `json:"tokens,omitempty"`
		AllowNoToken bool `json:"allowNoToken,omitempty"`
	} `json:"server"`
	Context struct {
		Depth          *int     `json:"depth,omitempty"`
		Threshold      *float64 `json:"threshold,omitempty"`
		Hop            *float64 `json:"hop,omitempty"`
		ShowSuperseded *bool    `json:"showSuperseded,omitempty"`
		Focus          []string `json:"focus,omitempty"`
	} `json:"context"`
	Embedding struct {
		// Provider selects the vector engine: "hash" (default, offline, lexical) or "openai" for any
		// OpenAI-compatible /embeddings endpoint (OpenRouter, OpenAI, Ollama, LM Studio, vLLM, TEI).
		Provider  string `json:"provider,omitempty"`
		Model     string `json:"model,omitempty"`
		BaseURL   string `json:"baseUrl,omitempty"`
		APIKeyEnv string `json:"apiKeyEnv,omitempty"` // env var name only; the key never touches config or brain.xml
		Dim       *int   `json:"dim,omitempty"`
		Batch     *int   `json:"batch,omitempty"`
		Timeout   *int   `json:"timeout,omitempty"`
		Retries   *int   `json:"retries,omitempty"`
		Fallback  string `json:"fallback,omitempty"` // "hash" (default) | "error"
		// InputType is the asymmetric-model hint (Cohere input_type, Gemini taskType, Voyage input_type).
		InputType string `json:"inputType,omitempty"`
		// APIVersion is Azure OpenAI's api-version query parameter.
		APIVersion string   `json:"apiVersion,omitempty"`
		Cache      string   `json:"cache,omitempty"`
		Title      string   `json:"title,omitempty"`
		Alpha      *float64 `json:"alpha,omitempty"`
		Hops       *int     `json:"hops,omitempty"`
		Use        string   `json:"use,omitempty"`
		Lambda     *float64 `json:"lambda,omitempty"`
	} `json:"embedding"`
	Budget struct {
		WarnTokens int `json:"warnTokens,omitempty"`
	} `json:"budget"`
	Kinds []struct {
		Name     string   `json:"name"`
		Label    string   `json:"label,omitempty"`
		Color    string   `json:"color,omitempty"`
		HalfLife *float64 `json:"halfLife,omitempty"`
		Floor    *float64 `json:"floor,omitempty"`
		Base     *float64 `json:"base,omitempty"`
		Prefix   string   `json:"prefix,omitempty"`
		Section  string   `json:"section,omitempty"`
	} `json:"kinds,omitempty"`
	Sections []string `json:"sections,omitempty"`
	Concepts []struct {
		ID      string   `json:"id"`
		Label   string   `json:"label,omitempty"`
		Related []string `json:"related,omitempty"`
	} `json:"concepts,omitempty"`
	Import    ImportConfig   `json:"import"`
	Sources   []SourceConfig `json:"sources,omitempty"`
	Templates struct {
		Dir string `json:"dir,omitempty"`
	} `json:"templates"`
	MCP struct {
		Actor string   `json:"actor,omitempty"`
		Tools []string `json:"tools,omitempty"`
	} `json:"mcp"`
	Publish struct {
		Mode   string   `json:"mode,omitempty"`
		Tools  []string `json:"tools,omitempty"`
		Labels []string `json:"labels,omitempty"`
	} `json:"publish"`
	Patterns struct {
		Locales       []string           `json:"locales,omitempty"`
		DefaultRegion string             `json:"defaultRegion,omitempty"`
		DateOrder     string             `json:"dateOrder,omitempty"`
		Disabled      []string           `json:"disabled,omitempty"`
		Custom        []*patterns.Custom `json:"custom,omitempty"`
		Fx            struct {
			Base  string             `json:"base,omitempty"`
			Rates map[string]float64 `json:"rates,omitempty"`
		} `json:"fx"`
	} `json:"patterns"`
	Privacy struct {
		Mask           []string `json:"mask,omitempty"`
		RedactOnIngest []string `json:"redactOnIngest,omitempty"`
	} `json:"privacy"`
	Connectors struct {
		MaxFanout int    `json:"maxFanout,omitempty"`
		Bridges   *bool  `json:"bridges,omitempty"`
		ImportHub bool   `json:"importHub,omitempty"`
		Time      string `json:"time,omitempty"`
		Anchors   []struct {
			Key   string   `json:"key"`
			Match []string `json:"match"`
		} `json:"anchors,omitempty"`
		Aliases []struct {
			Label string   `json:"label"`
			Terms []string `json:"terms"`
		} `json:"aliases,omitempty"`
		EntityAnchors []string `json:"entityAnchors,omitempty"`
	} `json:"connectors"`
	Storage struct {
		Backend string `json:"backend,omitempty"` // file | sqlite
		Path    string `json:"path,omitempty"`    // overrides the brain path
	} `json:"storage,omitempty"`
	Memory  map[string]any `json:"memory,omitempty"`
	Cache   map[string]any `json:"cache,omitempty"`
	Index   map[string]any `json:"index,omitempty"`
	Access  AccessConfig   `json:"access"`
	Purpose *Purpose       `json:"purpose,omitempty"`
}

// ImportConfig configures bulk import.
type ImportConfig struct {
	KeepRaw   bool `json:"keepRaw,omitempty"`
	ChunkSize int  `json:"chunkSize,omitempty"`
	Autolink  struct {
		ConceptMin *float64 `json:"conceptMin,omitempty"`
		SimilarMin *float64 `json:"similarMin,omitempty"`
		MaxSimilar int      `json:"maxSimilar,omitempty"`
	} `json:"autolink"`
	Limits struct {
		FileSize int64 `json:"fileSize,omitempty"`
		Records  int   `json:"records,omitempty"`
		Chunks   int   `json:"chunks,omitempty"`
		Nodes    int   `json:"nodes,omitempty"`
	} `json:"limits"`
	Rules []ImportRule `json:"rules,omitempty"`
}

// ImportRule maps files (glob on the relative path) to an importer and defaults.
type ImportRule struct {
	Match              string                 `json:"match"`
	Importer           string                 `json:"importer,omitempty"`
	Kind               string                 `json:"kind,omitempty"`
	Section            string                 `json:"section,omitempty"`
	Concepts           []string               `json:"concepts,omitempty"`
	HeadingsAsConcepts *bool                  `json:"headingsAsConcepts,omitempty"`
	Tables             map[string]TableConfig `json:"tables,omitempty"`
	JSON               struct {
		Path string `json:"path,omitempty"`
	} `json:"json"`
	Text   string   `json:"text,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Skip   bool     `json:"skip,omitempty"`
}

// TableConfig configures one SQL table (or JSON/CSV source).
type TableConfig struct {
	Kind        string   `json:"kind,omitempty"`
	Section     string   `json:"section,omitempty"`
	Text        string   `json:"text,omitempty"`
	Concepts    []string `json:"concepts,omitempty"`
	Key         string   `json:"key,omitempty"`
	ForeignKeys any      `json:"foreignKeys,omitempty"` // true | false | "anchor"
	Skip        bool     `json:"skip,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// ---------------------------------------------------------------- schema validation

type schemaNode struct {
	kind   string // map | list | scalar | any | strmap
	fields map[string]*schemaNode
	elem   *schemaNode
}

func sm(fields map[string]*schemaNode) *schemaNode { return &schemaNode{kind: "map", fields: fields} }
func sl(elem *schemaNode) *schemaNode              { return &schemaNode{kind: "list", elem: elem} }

var (
	sc  = &schemaNode{kind: "scalar"}
	sa  = &schemaNode{kind: "any"}
	ssl = sl(sc)
)

func strmap(elem *schemaNode) *schemaNode { return &schemaNode{kind: "strmap", elem: elem} }

var tableSchema = sm(map[string]*schemaNode{"kind": sc, "section": sc, "text": sc, "concepts": ssl, "key": sc, "foreignKeys": sc, "skip": sc, "labels": ssl})

var configSchema = sm(map[string]*schemaNode{
	"name": sc, "description": sc, "goal": sc,
	"server": sm(map[string]*schemaNode{"addr": sc, "tokenEnv": sc, "allowNoToken": sc,
		"tokens": sl(sm(map[string]*schemaNode{"name": sc, "tokenEnv": sc, "scope": sc, "dbs": ssl}))}),
	"context": sm(map[string]*schemaNode{"depth": sc, "threshold": sc, "hop": sc, "showSuperseded": sc, "focus": ssl}),
	"embedding": sm(map[string]*schemaNode{"provider": sc, "model": sc, "baseUrl": sc, "apiKeyEnv": sc, "dim": sc, "batch": sc,
		"timeout": sc, "retries": sc, "fallback": sc, "cache": sc, "title": sc, "alpha": sc, "hops": sc, "use": sc, "lambda": sc,
		"inputType": sc, "apiVersion": sc}),
	"budget":   sm(map[string]*schemaNode{"warnTokens": sc}),
	"kinds":    sl(sm(map[string]*schemaNode{"name": sc, "label": sc, "color": sc, "halfLife": sc, "floor": sc, "base": sc, "prefix": sc, "section": sc})),
	"sections": ssl,
	"concepts": sl(sm(map[string]*schemaNode{"id": sc, "label": sc, "related": ssl})),
	"import": sm(map[string]*schemaNode{"keepRaw": sc, "chunkSize": sc,
		"autolink": sm(map[string]*schemaNode{"conceptMin": sc, "similarMin": sc, "maxSimilar": sc}),
		"limits":   sm(map[string]*schemaNode{"fileSize": sc, "records": sc, "chunks": sc, "nodes": sc}),
		"rules": sl(sm(map[string]*schemaNode{"match": sc, "importer": sc, "kind": sc, "section": sc, "concepts": ssl, "headingsAsConcepts": sc,
			"tables": strmap(tableSchema), "json": sm(map[string]*schemaNode{"path": sc}), "text": sc, "labels": ssl, "skip": sc}))}),
	"sources":   sl(sm(map[string]*schemaNode{"name": sc, "uri": sc, "tokenEnv": sc, "weight": sc, "ttl": sc, "mode": sc, "federateDepth": sc, "enabled": sc, "tokens": sa})),
	"templates": sm(map[string]*schemaNode{"dir": sc}),
	"mcp":       sm(map[string]*schemaNode{"actor": sc, "tools": ssl}),
	"publish":   sm(map[string]*schemaNode{"mode": sc, "tools": ssl, "labels": ssl}),
	"patterns": sm(map[string]*schemaNode{"locales": ssl, "defaultRegion": sc, "dateOrder": sc, "disabled": ssl,
		"custom": sl(sm(map[string]*schemaNode{"name": sc, "type": sc, "regex": sc, "normalize": sc, "pii": sc})),
		"fx":     sm(map[string]*schemaNode{"base": sc, "rates": strmap(sc)})}),
	"privacy": sm(map[string]*schemaNode{"mask": ssl, "redactOnIngest": ssl}),
	"connectors": sm(map[string]*schemaNode{"maxFanout": sc, "bridges": sc, "importHub": sc, "time": sc, "entityAnchors": ssl,
		"anchors": sl(sm(map[string]*schemaNode{"key": sc, "match": ssl})), "aliases": sl(sm(map[string]*schemaNode{"label": sc, "terms": ssl}))}),
	"storage": sm(map[string]*schemaNode{"backend": sc, "path": sc}), "memory": sa, "cache": sa, "index": sa,
	"purpose": sm(map[string]*schemaNode{"name": sc, "description": sc,
		"inScope":    sm(map[string]*schemaNode{"topics": ssl, "concepts": ssl, "kinds": ssl, "sections": ssl}),
		"outOfScope": sm(map[string]*schemaNode{"topics": ssl, "entities": ssl}),
		"examples":   sm(map[string]*schemaNode{"accept": ssl, "reject": ssl}),
		"admission":  sm(map[string]*schemaNode{"mode": sc, "accept": sc, "review": sc, "apply": ssl, "exempt": ssl, "chunks": sc, "records": sc})}),
	"access": sm(map[string]*schemaNode{"enabled": sc, "file": sc,
		"labels":   sl(sm(map[string]*schemaNode{"name": sc, "description": sc})),
		"defaults": sm(map[string]*schemaNode{"sections": strmap(ssl), "kinds": strmap(ssl), "import": strmap(ssl)}),
		"roles": sl(sm(map[string]*schemaNode{"name": sc, "grants": sl(sm(map[string]*schemaNode{"actions": ssl, "labels": sa, "dbs": sa,
			"sections": ssl, "kinds": ssl, "sources": ssl, "deny": sc}))})),
		"users": sl(sm(map[string]*schemaNode{"name": sc, "display": sc, "roles": ssl, "labels": ssl, "defaultWriteLabels": ssl, "dbs": ssl,
			"disabled": sc, "tokenEnv": sc})),
		"jwt": sm(map[string]*schemaNode{"issuer": sc, "audience": sc, "jwksUrl": sc, "secretEnv": sc, "publicKeyFile": sc, "alg": sc,
			"claims": sm(map[string]*schemaNode{"user": sc, "roles": sc, "labels": sc})})}),
})

// ConfigError is a validation error with a line number.
type ConfigError struct {
	Line int    `json:"line"`
	Path string `json:"path"`
	Msg  string `json:"message"`
}

func (e ConfigError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s: %s", e.Line, e.Path, e.Msg)
	}
	return e.Path + ": " + e.Msg
}

func validateNode(v any, s *schemaNode, path string, line int, errs *[]ConfigError) {
	if s == nil || s.kind == "any" || v == nil {
		return
	}
	switch s.kind {
	case "map", "strmap":
		m, ok := v.(*OMap)
		if !ok {
			*errs = append(*errs, ConfigError{line, path, "expected a mapping"})
			return
		}
		for _, k := range m.Keys {
			l := line
			if m.Lines != nil && m.Lines[k] > 0 {
				l = m.Lines[k]
			}
			p := joinPath(path, k)
			if s.kind == "strmap" {
				validateNode(m.Vals[k], s.elem, p, l, errs)
				continue
			}
			f, ok := s.fields[k]
			if !ok {
				keys := make([]string, 0, len(s.fields))
				for x := range s.fields {
					keys = append(keys, x)
				}
				sort.Strings(keys)
				msg := fmt.Sprintf("unknown key %q", k)
				if c := Closest(k, keys, 3); c != "" {
					msg += fmt.Sprintf(" (did you mean %q?)", c)
				}
				*errs = append(*errs, ConfigError{l, p, msg})
				continue
			}
			validateNode(m.Vals[k], f, p, l, errs)
		}
	case "list":
		l, ok := v.([]any)
		if !ok {
			*errs = append(*errs, ConfigError{line, path, "expected a list"})
			return
		}
		for i, it := range l {
			validateNode(it, s.elem, fmt.Sprintf("%s[%d]", path, i), line, errs)
		}
	case "scalar":
		switch v.(type) {
		case *OMap, []any:
			*errs = append(*errs, ConfigError{line, path, "expected a single value"})
		}
	}
}

// ParseConfig parses and validates braindb.yaml text.
func ParseConfig(src string) (*Config, []ConfigError) {
	v, err := ParseYAML(src)
	if err != nil {
		if ye, ok := err.(*YAMLError); ok {
			return nil, []ConfigError{{Line: ye.Line, Path: "yaml", Msg: ye.Msg}}
		}
		return nil, []ConfigError{{Path: "yaml", Msg: err.Error()}}
	}
	cfg := &Config{}
	if v == nil {
		return cfg, nil
	}
	errs := []ConfigError{}
	validateNode(v, configSchema, "", 0, &errs)
	if len(errs) > 0 {
		return nil, errs
	}
	d, _ := json.Marshal(normalizeYAMLForJSON(v))
	if err := json.Unmarshal(d, cfg); err != nil {
		return nil, []ConfigError{{Path: "config", Msg: err.Error()}}
	}
	errs = append(errs, cfg.check()...)
	if len(errs) > 0 {
		return nil, errs
	}
	return cfg, nil
}

// normalizeYAMLForJSON turns YAML scalars into JSON-friendly values (numbers stay numbers, numeric strings for
// string fields are handled by retrying as strings in check()).
func normalizeYAMLForJSON(v any) any {
	switch x := v.(type) {
	case *OMap:
		m := NewOMap()
		for _, k := range x.Keys {
			m.Set(k, normalizeYAMLForJSON(x.Vals[k]))
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeYAMLForJSON(e)
		}
		return out
	}
	return v
}

// isEnvName reports whether s is shaped like an environment variable name. Config keys that end in Env
// hold a name, never a value, and a secret pasted into one would be committed with the file.
func isEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func (c *Config) check() []ConfigError {
	errs := []ConfigError{}
	for i, k := range c.Kinds {
		if k.Name == "" {
			errs = append(errs, ConfigError{Path: fmt.Sprintf("kinds[%d]", i), Msg: "name is required"})
		}
	}
	for i, s := range c.Sources {
		if err := s.toSource().validate(); err != nil {
			errs = append(errs, ConfigError{Path: fmt.Sprintf("sources[%d]", i), Msg: err.Error()})
		}
	}
	for i, p := range c.Patterns.Custom {
		if err := p.Compile(); err != nil {
			errs = append(errs, ConfigError{Path: fmt.Sprintf("patterns.custom[%d]", i), Msg: err.Error()})
		}
	}
	switch c.Storage.Backend {
	case "", "file", "sqlite":
		if p := c.Storage.Path; p != "" && c.Storage.Backend != "" && BackendFor(p) != c.Storage.Backend {
			errs = append(errs, ConfigError{Path: "storage.backend",
				Msg: fmt.Sprintf("says %q but storage.path %q is a %s brain; the extension decides (.db/.sqlite = sqlite, anything else = file)",
					c.Storage.Backend, p, BackendFor(p))})
		}
	default:
		errs = append(errs, ConfigError{Path: "storage.backend", Msg: fmt.Sprintf("must be file or sqlite, not %q", c.Storage.Backend)})
	}
	// Server tokens name an environment variable; the value never belongs in this file. A scope the
	// loader rejects must fail here too, or a config that validates in the UI kills the server on its
	// next start.
	if e := c.Server.TokenEnv; e != "" && !isEnvName(e) {
		errs = append(errs, ConfigError{Path: "server.tokenEnv", Msg: fmt.Sprintf("must be the NAME of an environment variable, not %q; the token value never belongs in this file", e)})
	}
	for i, t := range c.Server.Tokens {
		at := fmt.Sprintf("server.tokens[%d]", i)
		if t.Name == "" {
			errs = append(errs, ConfigError{Path: at, Msg: "name is required"})
		}
		switch {
		case t.TokenEnv == "":
			errs = append(errs, ConfigError{Path: at + ".tokenEnv", Msg: "required: name the environment variable holding the token"})
		case !isEnvName(t.TokenEnv):
			errs = append(errs, ConfigError{Path: at + ".tokenEnv", Msg: fmt.Sprintf("must be the NAME of an environment variable, not %q; the token value never belongs in this file", t.TokenEnv)})
		}
		switch t.Scope {
		case "", "read", "write", "admin":
		default:
			errs = append(errs, ConfigError{Path: at + ".scope", Msg: fmt.Sprintf("must be read, write or admin, not %q", t.Scope)})
		}
	}
	if o := c.Patterns.DateOrder; o != "" && o != "dmy" && o != "mdy" && o != "ymd" {
		errs = append(errs, ConfigError{Path: "patterns.dateOrder", Msg: "must be dmy, mdy or ymd"})
	}
	// Nothing in the engine reads these two yet. Accepting them silently would promise automatic
	// bridge and time connectors that are never built, so say so instead.
	if c.Connectors.Bridges != nil {
		errs = append(errs, ConfigError{Path: "connectors.bridges", Msg: "not implemented: brainDB builds no bridge connectors on its own; create them with CONNECTOR bridge \"…\""})
	}
	if c.Connectors.Time != "" {
		errs = append(errs, ConfigError{Path: "connectors.time", Msg: "not implemented: there are no automatic time connectors; query time with the touched column (WHERE touched < '2026-11-01')"})
	}
	if u := c.Embedding.Use; u != "" && u != "raw" && u != "enriched" {
		errs = append(errs, ConfigError{Path: "embedding.use", Msg: "must be raw or enriched"})
	}
	switch strings.ToLower(strings.TrimSpace(c.Embedding.Provider)) {
	case "", "hash", "local":
		if c.Embedding.BaseURL != "" || c.Embedding.APIKeyEnv != "" {
			errs = append(errs, ConfigError{Path: "embedding.provider", Msg: "baseUrl and apiKeyEnv need a remote provider, such as provider: openai"})
		}
	default:
		spec, ok := lookupEmbedProvider(c.Embedding.Provider)
		if !ok {
			errs = append(errs, ConfigError{Path: "embedding.provider", Msg: "must be hash or one of: " + strings.Join(embedProviderNames(), ", ")})
			break
		}
		if c.Embedding.Model == "" {
			errs = append(errs, ConfigError{Path: "embedding.model", Msg: "required for a remote provider"})
		}
		if spec.BaseURL == "" && c.Embedding.BaseURL == "" {
			errs = append(errs, ConfigError{Path: "embedding.baseUrl", Msg: "required for provider " + spec.Name + ": " + spec.Note})
		}
		if c.Embedding.APIVersion != "" && spec.Auth != "azure" {
			errs = append(errs, ConfigError{Path: "embedding.apiVersion", Msg: "only the azure provider uses an api-version"})
		}
	}
	if f := c.Embedding.Fallback; f != "" && f != "hash" && f != "error" {
		errs = append(errs, ConfigError{Path: "embedding.fallback", Msg: "must be hash or error"})
	}
	errs = append(errs, c.Access.check()...)
	if p := c.Purpose; p != nil {
		if m := p.Admission.Mode; m != "" && m != "off" && m != "advise" && m != "enforce" {
			errs = append(errs, ConfigError{Path: "purpose.admission.mode", Msg: "must be off, advise or enforce"})
		}
		if a, r := p.thresholds(); r > a {
			errs = append(errs, ConfigError{Path: "purpose.admission", Msg: "review threshold must be ≤ accept"})
		}
	}
	return errs
}

// ConfigPath returns braindb.yaml next to a brain file.
func ConfigPath(brainPath string) string {
	return filepath.Join(filepath.Dir(brainPath), "braindb.yaml")
}

// LoadConfigFile reads and validates a config file ("" or missing → nil, nil).
func LoadConfigFile(path string) (*Config, string, error) {
	d, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	cfg, errs := ParseConfig(string(d))
	if len(errs) > 0 {
		msgs := []string{}
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return nil, string(d), fmt.Errorf("%s:\n  %s", path, strings.Join(msgs, "\n  "))
	}
	if cfg.Access.File != "" {
		p := cfg.Access.File
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(path), p)
		}
		ad, err := os.ReadFile(p)
		if err != nil {
			return nil, string(d), fmt.Errorf("access.file: %v", err)
		}
		ac, errs := ParseConfig("access:\n" + indent(string(ad), "  "))
		if len(errs) > 0 {
			return nil, string(d), fmt.Errorf("%s: %v", p, errs[0])
		}
		file := cfg.Access.File
		cfg.Access = ac.Access
		cfg.Access.File = file
	}
	return cfg, string(d), nil
}

func indent(s, pad string) string {
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		if strings.TrimSpace(l) != "" {
			ls[i] = pad + l
		}
	}
	return strings.Join(ls, "\n")
}

// ApplyConfig applies configuration to a brain. newDB also applies goal, context and embedding defaults.
func (b *Brain) ApplyConfig(c *Config, newDB bool) error {
	if c == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if newDB {
		if c.Name != "" {
			b.Name = c.Name
		}
		if c.Goal != "" {
			b.Goal = c.Goal
		}
		if x := c.Context; true {
			if x.Depth != nil {
				b.P.Depth = *x.Depth
			}
			if x.Threshold != nil {
				b.P.Threshold = *x.Threshold
			}
			if x.Hop != nil {
				b.P.Hop = *x.Hop
			}
			if x.ShowSuperseded != nil {
				b.P.ShowSuperseded = *x.ShowSuperseded
			}
		}
		e := c.Embedding
		if e.Alpha != nil {
			b.Emb.Alpha = *e.Alpha
		}
		if e.Hops != nil {
			b.Emb.Hops = *e.Hops
		}
		if e.Lambda != nil {
			b.Emb.Lambda = *e.Lambda
		}
		if e.Use != "" {
			b.Emb.Use = e.Use
		}
	}
	for _, k := range c.Kinds {
		if ex, ok := b.kindM[k.Name]; ok {
			// override a built-in or existing kind
			if k.HalfLife != nil {
				ex.HalfLife = fp(*k.HalfLife)
			}
			if k.Floor != nil {
				ex.Floor = clamp01(*k.Floor)
			}
			if k.Base != nil {
				ex.Base = *k.Base
			}
			if k.Color != "" {
				ex.Color = k.Color
			}
			if k.Label != "" {
				ex.Label = k.Label
			}
			if k.Section != "" && !ex.Builtin {
				ex.Section = k.Section
			}
			continue
		}
		hl := 14.0
		if k.HalfLife != nil {
			hl = *k.HalfLife
		}
		base := .5
		if k.Base != nil {
			base = *k.Base
		}
		floor := 0.0
		if k.Floor != nil {
			floor = clamp01(*k.Floor)
		}
		b.addKind(Kind{Name: k.Name, Label: k.Label, Color: k.Color, HalfLife: fp(hl), Floor: floor, Base: base, Prefix: k.Prefix, Section: k.Section})
		b.addSection(b.kindM[k.Name].Section)
	}
	for _, s := range c.Sections {
		b.addSection(s)
	}
	for _, cc := range c.Concepts {
		id := cid(cc.ID)
		if b.nodes[id] == nil {
			label := cc.Label
			if label == "" {
				label = cc.ID
			}
			b.addNode(&Node{ID: id, Kind: Concept, Section: "concepts", Text: label, Base: .8})
		}
		for _, r := range cc.Related {
			b.addEdge(id, b.findConcept(r, true), .6, EdgeConcept)
		}
	}
	for _, s := range c.Sources {
		if b.source(s.Name) == nil {
			b.sources = append(b.sources, s.toSource())
		}
	}
	for _, a := range c.Connectors.Aliases {
		if _, err := b.addConnector(ConnectorSpec{Type: "alias", Label: a.Label, Terms: a.Terms, Key: "alias:" + lower(a.Label)}); err != nil {
			return err
		}
	}
	if c.Connectors.MaxFanout > 0 {
		MaxFanout = c.Connectors.MaxFanout
	}
	p := c.Patterns
	b.Pat = &patterns.Options{Locales: p.Locales, DefaultRegion: p.DefaultRegion, DateOrder: p.DateOrder, Disabled: p.Disabled, Custom: p.Custom,
		FxBase: p.Fx.Base, FxRates: p.Fx.Rates}
	b.Privacy, b.RedactOnIngest = c.Privacy.Mask, c.Privacy.RedactOnIngest
	b.entCache = nil
	b.access = &c.Access
	b.purpose, b.pvecs = c.Purpose, nil
	// The vector engine is part of the config, not of the seed, so it is applied on every load. A misconfigured
	// remote provider fails here rather than silently degrading a whole import to hash vectors.
	em, err := NewEmbedder(c.embedderOptions(), b.Dir)
	if err != nil {
		return err
	}
	b.embedder, b.pvecs = em, nil
	b.embedAll()
	b.refresh()
	return nil
}

// Effective returns the effective config as an ordered map (secrets are only env var names, never values).
func (c *Config) Effective() any { return toOrdered(c) }

// embedderOptions maps the embedding block of braindb.yaml onto EmbedderOptions.
func (c *Config) embedderOptions() EmbedderOptions {
	e := c.Embedding
	o := EmbedderOptions{Provider: e.Provider, Model: e.Model, BaseURL: e.BaseURL, APIKeyEnv: e.APIKeyEnv,
		Fallback: e.Fallback, Cache: e.Cache, Title: e.Title, InputType: e.InputType, APIVersion: e.APIVersion}
	if e.Dim != nil {
		o.Dim = *e.Dim
	}
	if e.Batch != nil {
		o.Batch = *e.Batch
	}
	if e.Timeout != nil {
		o.Timeout = *e.Timeout
	}
	if e.Retries != nil {
		// an explicit 0 in the config means "never retry", which EmbedderOptions spells as negative
		if o.Retries = *e.Retries; o.Retries == 0 {
			o.Retries = -1
		}
	}
	// hash provider: `model: hash-256` is the legacy spelling of the default engine, not a remote model id
	if o.Provider == "" && strings.HasPrefix(o.Model, "hash-") {
		o.Model = ""
	}
	return EmbedderFromEnv(o)
}

// EmbedderFromEnv lets the environment override the embedding block, which is how a container is configured without
// baking a braindb.yaml into the image. Precedence is flags > env > braindb.yaml > defaults.
//
//	BRAINDB_EMBED_PROVIDER  hash | openai (aliases: openrouter, ollama, compatible)
//	BRAINDB_EMBED_MODEL     provider model id, e.g. openai/text-embedding-3-small
//	BRAINDB_EMBED_BASE_URL  endpoint root; /embeddings is appended
//	BRAINDB_EMBED_KEY_ENV   name of the variable holding the API key (never the key itself)
//	BRAINDB_EMBED_DIM       expected vector size
//	BRAINDB_EMBED_FALLBACK  hash | error
//	BRAINDB_EMBED_CACHE     path to the vector cache file
func EmbedderFromEnv(o EmbedderOptions) EmbedderOptions {
	str := func(name string, dst *string) {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			*dst = v
		}
	}
	str("BRAINDB_EMBED_PROVIDER", &o.Provider)
	str("BRAINDB_EMBED_MODEL", &o.Model)
	str("BRAINDB_EMBED_BASE_URL", &o.BaseURL)
	str("BRAINDB_EMBED_KEY_ENV", &o.APIKeyEnv)
	str("BRAINDB_EMBED_FALLBACK", &o.Fallback)
	str("BRAINDB_EMBED_CACHE", &o.Cache)
	str("BRAINDB_EMBED_INPUT_TYPE", &o.InputType)
	str("BRAINDB_EMBED_API_VERSION", &o.APIVersion)
	if v := strings.TrimSpace(os.Getenv("BRAINDB_EMBED_DIM")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			o.Dim = n
		}
	}
	return o
}
