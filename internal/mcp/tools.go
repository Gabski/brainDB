// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Gabski/brainDB/internal/brain"
)

func init() {
	Register(&Tool{Name: "brain_view", Description: "INPUT. Returns the current working context: only the LIT slice of long-term memory, as XML (items sorted by activation, with depth, via-path, refs and concepts) or Markdown, plus a status line. " +
		"Optional args change how the slice is lit and persist (shared with the human in the 3D UI): focus = seed ids (or \"goal\"), add_focus/remove_focus, depth (hops of spreading activation), threshold, hop (damping), show_superseded. " +
		"In a read-only brain these args apply to this call only.",
		InputSchema: schema(`{"type":"object","properties":{
"focus":{"oneOf":[{"type":"array","items":{"type":"string"}},{"type":"string","enum":["goal"]}]},
"add_focus":{"type":"array","items":{"type":"string"}},"remove_focus":{"type":"array","items":{"type":"string"}},
"depth":{"type":"integer","minimum":0,"maximum":6},"threshold":{"type":"number","minimum":0.01,"maximum":0.9},"hop":{"type":"number","minimum":0.3,"maximum":1},
"show_superseded":{"type":"boolean"},"format":{"type":"string","enum":["xml","md"],"description":"xml (default) or md: Markdown summary with focus, sections, activation tree and stats"},
"sources":{"type":"array","items":{"type":"string"},"description":"external sources to federate (names or *)"}}}`),
		Run: toolView})
	Register(&Tool{Name: "brain_search", Description: "Search the WHOLE memory (not only the lit slice). mode: hybrid (default; vector seeds → graph activation, GraphRAG), vector (kNN on graph-enriched embeddings), " +
		"tree (beam search over the graph guided by the query vector; returns a tree of paths), graph (word stems + activation; also lists knowledge found only through connections). focus=true lights the top results as the new focus.",
		InputSchema: schema(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"mode":{"type":"string","enum":["hybrid","vector","tree","graph"]},
"k":{"type":"integer","minimum":1,"maximum":30},"focus":{"type":"boolean"},"sources":{"type":"array","items":{"type":"string"}}}}`),
		Run: toolSearch})
	Register(&Tool{Name: "brain_inspect", Description: "Look at one memory/concept: what = any of item (XML + weight breakdown + path from focus), links, near (semantic neighbours), test (propagation from it + ablation of each synapse, ! = critical), " +
		"suggest (predicted missing synapses), paths (strongest knowledge paths to \"to\"). Without id: what=[\"discover\"] lists missing synapses inside the context and bridges to faded memory; what=[\"connectors\"] proposes connectors.",
		InputSchema: schema(`{"type":"object","properties":{"id":{"type":"string"},"to":{"type":"string"},
"what":{"type":"array","items":{"type":"string","enum":["item","links","near","test","suggest","paths","discover","connectors"]}}}}`),
		Run: toolInspect})
	Register(&Tool{Name: "brain_exec", Write: true, Description: "OUTPUT. Atomic batch of writes (all or nothing). Ops (JSON objects with \"op\"): " +
		"remember {kind, text, section?, concepts?: [concept label or id; created if new], links?: [{to, w}], supersedes?: id, pin?, focus?, tmp?: \"A\" → later \"@A\", fields?: {…}}; " +
		"concept {label, members?: [ids], related?: [labels]}; link {a, b, w?, type?: assoc|similar}; unlink {a, b}; boost {id}; weaken {id}; pin/unpin {id}; " +
		"supersede {old, new}; edit {id, text?, section?, kind?, fields?}; forget {id, force?}; focus {ids? | add? | remove?}; goal {text, focus?}; " +
		"connector {ctype: anchor|bridge|hub|alias|time, label, key?, connect?: [ids], terms?}; connect/disconnect {a, b: connector}; resolve {label, by: [fields], min?, dryRun?}. " +
		"New memories are embedded and auto-linked to the nearest concepts (◆) and similar memories (≈). Prefer supersede over forget. Returns changes + status line.",
		InputSchema: schema(`{"type":"object","required":["ops"],"properties":{"ops":{"type":"array","items":{"type":"object","required":["op"],"properties":{"op":{"type":"string"}}}}}}`),
		Run:         toolExec})
	Register(&Tool{Name: "brain_ingest", Write: true, Description: "Put files into the brain: a file or directory path on this machine. Text/markdown/code/pdf → file node + chunks → embedding → auto-links. CSV/TSV → one record per row with fields. Images → file node. " +
		"ctxc workspace (.ctx dir or state.json) and ctxc <context> XML → items with sections, pins, weights, refs and tags. focus=true (default) lights the new files (only files with ≤ 8 chunks).",
		InputSchema: schema(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"focus":{"type":"boolean"},"max_files":{"type":"integer"}}}`),
		Run:         toolIngest})
	Register(&Tool{Name: "brain_query", Description: "Database-style query. target=memory (default): filter, sort and project memories and records like a table; query is the compact form " +
		"(e.g. 'kind:deal stage=negotiation value>10000 owner~kim \"free text\" near:\"renewal risk\" sort:-value limit:10 select:id,text,fields.value') or a JSON object " +
		"{from, where:{kind, section, concept, text, lit, pinned, superseded, minWeight, maxAgeDays, fields:[{name, op, value}]}, near, orderBy, limit, offset, select}. Ops: = != > >= < <= ~ (numbers and currencies compare numerically). " +
		"target=context: an ad-hoc lit slice without changing the shared focus; query is {focus?, query?, depth?, threshold?, hop?} or plain text. format: json|yaml|xml|md|csv (memory), xml|md|json|yaml (context).",
		InputSchema: schema(`{"type":"object","required":["query"],"properties":{"target":{"type":"string","enum":["memory","context"]},"query":{"oneOf":[{"type":"string"},{"type":"object"}]},
"format":{"type":"string","enum":["json","yaml","xml","md","csv"]},"sources":{"type":"array","items":{"type":"string"}}}}`),
		Run: toolQuery})
	Register(&Tool{Name: "brain_fill", Description: "Fill a template from memory (deterministic, no LLM). Schema templates (JSON/YAML/XML): every string leaf is a slot \"description[:type][|option…]\"; " +
		"list slots: YAML/JSON key \"deals[]\": \"open deals|kind=deal|limit=5\" with item fields \"deals[].title\": \".text\" (or .id .kind .weight .fields.<name>, or a description searched near the item); XML: an element with repeat=\"…\". " +
		"Text templates: {{ description:type }} and {{#each spec}} … {{/each}}. Types: text name number money percent date email phone url bool list id org domain duration iban vat address version. " +
		"Options: kind=a,b section= concept= scope=context|memory limit=N min=0.25 field=name. The result lists missing slots: complete them, then store results with brain_exec.",
		InputSchema: schema(`{"type":"object","required":["template"],"properties":{"template":{"type":"string","description":"template text, or a path to a template file"},
"template_format":{"type":"string","enum":["auto","json","yaml","xml","md"]},"output":{"type":"string","enum":["same","json","yaml","xml","md"]},
"scope":{"type":"string","enum":["memory","context","federated"]},"with_sources":{"type":"boolean"},"min":{"type":"number"}}}`),
		Run: toolFill})
}

func toolView(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		Focus          json.RawMessage `json:"focus"`
		AddFocus       brain.StrList   `json:"add_focus"`
		RemoveFocus    brain.StrList   `json:"remove_focus"`
		Depth          *int            `json:"depth"`
		Threshold      *float64        `json:"threshold"`
		Hop            *float64        `json:"hop"`
		ShowSuperseded *bool           `json:"show_superseded"`
		Format         string          `json:"format"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	c := brain.ContextChange{AddFocus: a.AddFocus, RemoveFocus: a.RemoveFocus, Depth: a.Depth, Threshold: a.Threshold, Hop: a.Hop, ShowSuperseded: a.ShowSuperseded}
	if len(a.Focus) > 0 && string(a.Focus) != "null" {
		var f brain.StrList
		if err := json.Unmarshal(a.Focus, &f); err != nil {
			return "", fmt.Errorf("focus must be a list of ids or \"goal\"")
		}
		c.Focus = &f
	}
	changed := c.Focus != nil || c.AddFocus != nil || c.RemoveFocus != nil || c.Depth != nil || c.Threshold != nil || c.Hop != nil || c.ShowSuperseded != nil
	b := s.brain()
	format := a.Format
	if format == "" {
		format = "xml"
	}
	if format != "xml" && format != "md" {
		return "", fmt.Errorf("format must be xml or md")
	}
	if changed && (s.ReadOnly || s.Store.ReadOnly || (s.Principal != nil && !s.Principal.Admin)) {
		cq := brain.ContextQuery{Depth: a.Depth, Threshold: a.Threshold, Hop: a.Hop, ShowSuperseded: a.ShowSuperseded, Format: format}
		f := b.Focus()
		if c.Focus != nil {
			f = *c.Focus
		}
		f = append(f, a.AddFocus...)
		rm := map[string]bool{}
		for _, x := range a.RemoveFocus {
			rm[x] = true
		}
		cq.Focus = nil
		for _, x := range f {
			if !rm[x] {
				cq.Focus = append(cq.Focus, x)
			}
		}
		if len(cq.Focus) == 0 {
			cq.Focus = brain.StrList{"goal"}
		}
		r, err := b.QueryContext(cq)
		if err != nil {
			return "", err
		}
		return r.Output, nil
	}
	if changed {
		err := s.Store.Mutate(s.Actor, "brain_view", func(b *brain.Brain) (string, []string, error) {
			if _, err := b.SetContext(c); err != nil {
				return "", nil, err
			}
			p := b.State().Params
			return fmt.Sprintf("focus %s · depth %d · threshold %v", strings.Join(b.Focus(), ","), p.Depth, p.Threshold), b.Focus(), nil
		})
		if err != nil {
			return "", err
		}
	} else {
		s.Store.Sync()
		b = s.brain()
	}
	if format == "md" {
		return b.ContextMD(), nil
	}
	return b.ContextXML(), nil
}

func toolSearch(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		Query string `json:"query"`
		Mode  string `json:"mode"`
		K     int    `json:"k"`
		Focus bool   `json:"focus"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	s.Store.Sync()
	b := s.brain()
	r, err := b.Search(a.Query, a.Mode, a.K)
	if err != nil {
		return "", err
	}
	ids := []string{}
	for _, x := range append(r.Rows, r.Via...) {
		ids = append(ids, x.ID)
	}
	sum := fmt.Sprintf("%s: %q", r.Mode, a.Query)
	if a.Focus && !s.ReadOnly && !s.Store.ReadOnly && (s.Principal == nil || s.Principal.Admin) {
		if err := s.Store.Mutate(s.Actor, "brain_search", func(b *brain.Brain) (string, []string, error) {
			b.SearchFocus(r)
			return sum + " → focus", ids, nil
		}); err != nil {
			return "", err
		}
	} else {
		s.Store.Emit(s.Actor, "brain_search", sum, ids)
	}
	return fmtSearch(b, r), nil
}

func toolInspect(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		ID   string        `json:"id"`
		To   string        `json:"to"`
		What brain.StrList `json:"what"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	s.Store.Sync()
	r, err := s.brain().Inspect(a.ID, a.What, a.To)
	if err != nil {
		return "", err
	}
	if a.ID != "" {
		ids := []string{a.ID}
		if a.To != "" {
			ids = append(ids, a.To)
		}
		s.Store.Emit(s.Actor, "brain_inspect", fmt.Sprintf("%s: %s", a.ID, strings.Join(a.What, ", ")), ids)
	}
	return r.Text, nil
}

func toolExec(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		Ops []brain.Op `json:"ops"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	var res *brain.ExecResult
	err := s.Store.Mutate(s.Actor, "brain_exec", func(b *brain.Brain) (string, []string, error) {
		if p := s.Principal; p != nil && !p.Admin {
			for i := range a.Ops {
				if a.Ops[i].Op == "remember" {
					a.Ops[i].Labels = append(a.Ops[i].Labels, p.Default...)
				}
			}
			if err := b.CheckOps(p, s.Store.Name, a.Ops); err != nil {
				return "", nil, err
			}
		}
		r, err := b.Exec(a.Ops)
		if err != nil {
			return "", nil, err
		}
		res = r
		sum := strings.Join(r.Changes, " ")
		if len(r.Auto) > 0 {
			sum += "\nauto-links: " + autoText(r.Auto)
		}
		return sum, r.Touched, nil
	})
	if err != nil {
		return "", err
	}
	out := "changes: " + strings.Join(res.Changes, " ")
	if len(res.Auto) > 0 {
		out += "\nauto-links: " + autoText(res.Auto)
	}
	return out, nil
}

func toolIngest(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		Path     string `json:"path"`
		Focus    *bool  `json:"focus"`
		MaxFiles int    `json:"max_files"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	focus := a.Focus == nil || *a.Focus
	if p := s.Principal; p != nil && !p.Admin && !s.Store.B.CanWriteAny(p, s.Store.Name) {
		return "", fmt.Errorf("access denied: no write grant")
	}
	var res *brain.IngestResult
	err := s.Store.Mutate(s.Actor, "brain_ingest", func(b *brain.Brain) (string, []string, error) {
		r, err := b.IngestPath(absPath(a.Path), brain.IngestOptions{Focus: focus, MaxFiles: a.MaxFiles})
		if err != nil {
			return "", nil, err
		}
		res = r
		ids := []string{}
		for _, id := range r.Fresh {
			if b.Has(id) {
				ids = append(ids, id)
			}
		}
		return strings.Join(r.Report, "\n"), ids, nil
	})
	if err != nil {
		return "", err
	}
	out := strings.Join(res.Report, "\n")
	if len(res.Links) > 0 {
		l := res.Links
		if len(l) > 30 {
			l = l[:30]
		}
		out += fmt.Sprintf("\nauto-links (%d): %s", len(res.Links), autoText(l))
	}
	return out, nil
}

func toolQuery(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		Target  string          `json:"target"`
		Query   json.RawMessage `json:"query"`
		Format  string          `json:"format"`
		Sources brain.StrList   `json:"sources"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	s.Store.Sync()
	b := s.brain()
	var qs string
	if len(a.Query) > 0 && a.Query[0] == '"' {
		_ = json.Unmarshal(a.Query, &qs)
	} else {
		qs = string(a.Query)
	}
	if a.Target == "context" {
		var cq brain.ContextQuery
		if strings.HasPrefix(strings.TrimSpace(qs), "{") {
			if err := json.Unmarshal([]byte(qs), &cq); err != nil {
				return "", fmt.Errorf("context query: %v", err)
			}
		} else {
			cq.Query = qs
		}
		if a.Format != "" {
			cq.Format = a.Format
		}
		r, err := b.QueryContext(cq)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ad-hoc context (focus %s, %d lit, ~%d tok):\n%s", strings.Join(r.Focus, ","), r.Lit, r.Tokens, r.Output), nil
	}
	q, err := brain.ParseQuery(qs)
	if err != nil {
		return "", err
	}
	if len(a.Sources) > 0 {
		q.Sources = a.Sources
	}
	format := a.Format
	if format == "" && q.Format == "" {
		format = "md"
	}
	r, err := b.RunQuery(q, format)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d of %d rows:\n%s", r.Count, r.Total, r.Rendered), nil
}

func toolFill(s *Server, args json.RawMessage) (string, error) {
	var a struct {
		Template       string   `json:"template"`
		TemplateFormat string   `json:"template_format"`
		Output         string   `json:"output"`
		Scope          string   `json:"scope"`
		WithSources    bool     `json:"with_sources"`
		Min            *float64 `json:"min"`
	}
	if err := decode(args, &a); err != nil {
		return "", err
	}
	tpl := a.Template
	if !strings.ContainsAny(tpl, "\n{<:") {
		if d, err := os.ReadFile(tpl); err == nil {
			tpl = string(d)
		}
	}
	s.Store.Sync()
	r, err := s.brain().Fill(brain.FillRequest{Template: tpl, TemplateFormat: a.TemplateFormat, Output: a.Output, Scope: a.Scope, WithSources: a.WithSources, Min: a.Min})
	if err != nil {
		return "", err
	}
	out := r.Output
	if len(r.Missing) > 0 {
		out += "\nmissing slots (" + fmt.Sprint(len(r.Missing)) + "): " + strings.Join(r.Missing, ", ")
	} else {
		out += "\nall slots filled"
	}
	return out, nil
}
