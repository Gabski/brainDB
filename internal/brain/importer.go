// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ImportOptions control a bulk import.
type ImportOptions struct {
	Update   bool
	DryRun   bool
	Config   *Config // rules, limits, connectors (nil = defaults)
	Progress func(done, total int, file string)
}

// ImportFileReport is the outcome for one file.
type ImportFileReport struct {
	Path     string `json:"path"`
	Importer string `json:"importer"`
	Status   string `json:"status"` // created | updated | skipped | error
	Nodes    int    `json:"nodes"`
	Edges    int    `json:"edges"`
	Records  int    `json:"records"`
	Message  string `json:"message,omitempty"`
	// purpose filter (Addendum O)
	Admitted    int      `json:"admitted"`
	Quarantined int      `json:"quarantined"`
	Rejected    int      `json:"rejected"`
	Reasons     []string `json:"reasons,omitempty"`
}

// TableInfo describes a detected table (SQL) or record file.
type TableInfo struct {
	Name        string   `json:"name"`
	File        string   `json:"file"`
	Columns     []string `json:"columns"`
	Rows        int      `json:"rows"`
	ForeignKeys []string `json:"foreignKeys,omitempty"`
}

// ImportReport summarises an import.
type ImportReport struct {
	Files  []ImportFileReport `json:"files"`
	Totals struct {
		Files       int `json:"files"`
		Created     int `json:"created"`
		Updated     int `json:"updated"`
		Skipped     int `json:"skipped"`
		Errors      int `json:"errors"`
		Nodes       int `json:"nodes"`
		Edges       int `json:"edges"`
		Records     int `json:"records"`
		Admitted    int `json:"admitted"`
		Quarantined int `json:"quarantined"`
		Rejected    int `json:"rejected"`
	} `json:"totals"`
	Mapping    string      `json:"mapping"`
	Connectors []string    `json:"connectors,omitempty"`
	Tables     []TableInfo `json:"tables,omitempty"`
	Concepts   []string    `json:"concepts,omitempty"`
	DryRun     bool        `json:"dryRun"`
	Status     string      `json:"status"`
}

type importer struct {
	b       *Brain
	opt     ImportOptions
	cfg     *Config
	bySrc   map[string]string
	fresh   []string
	mapping *OMap
	links   []mdLink
	docs    map[string]string // rel path (no ext, lower) → doc node id
	tables  map[string]*sqlTable
	report  *ImportReport
	group   string                  // current table / record group key
	groupDe map[string]PurposeCheck // decisions per group
	parentD map[string]PurposeCheck // decisions of container nodes (chunks inherit)
	fr      *ImportFileReport
	nodes0  int
	edges0  int
}

type mdLink struct{ from, target, dir string }

func sha(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:8])
}

// globMatch matches a slash path against a pattern with *, ? and ** (any number of directories).
func globMatch(pattern, path string) bool {
	pattern, path = filepath.ToSlash(pattern), filepath.ToSlash(path)
	if !strings.Contains(pattern, "/") {
		if ok, _ := filepath.Match(pattern, filepath.Base(path)); ok {
			return true
		}
	}
	var rec func(p, s []string) bool
	rec = func(p, s []string) bool {
		if len(p) == 0 {
			return len(s) == 0
		}
		if p[0] == "**" {
			for i := 0; i <= len(s); i++ {
				if rec(p[1:], s[i:]) {
					return true
				}
			}
			return false
		}
		if len(s) == 0 {
			return false
		}
		if ok, _ := filepath.Match(p[0], s[0]); !ok {
			return false
		}
		return rec(p[1:], s[1:])
	}
	return rec(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func (im *importer) rule(rel string) *ImportRule {
	if im.cfg == nil {
		return nil
	}
	for i := range im.cfg.Import.Rules {
		if globMatch(im.cfg.Import.Rules[i].Match, rel) {
			return &im.cfg.Import.Rules[i]
		}
	}
	return nil
}

func detectImporter(rel string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".md", ".markdown", ".mdx":
		return "markdown"
	case ".csv", ".tsv":
		return "csv"
	case ".json":
		return "json"
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".yaml", ".yml":
		return "yaml"
	case ".sql":
		return "sql"
	case ".sqlite", ".db", ".sqlite3":
		return "sqlite"
	case ".docx":
		return "docx"
	case ".html", ".htm":
		return "html"
	case ".pdf":
		return "pdf"
	}
	if imgExt.MatchString(rel) {
		return "image"
	}
	if bytes.HasPrefix(data, []byte("SQLite format 3")) {
		return "sqlite"
	}
	return "text"
}

// Import imports files (with paths relative to the import root) atomically.
func (b *Brain) Import(files []InFile, opt ImportOptions) (rep *ImportReport, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	snap := b.clone()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("import failed: %v", r)
		}
		if err != nil || opt.DryRun {
			b.restore(snap)
			b.embedAll()
			b.refresh()
		}
	}()
	im := &importer{b: b, opt: opt, cfg: opt.Config, bySrc: map[string]string{}, mapping: NewOMap(), docs: map[string]string{}, tables: map[string]*sqlTable{},
		groupDe: map[string]PurposeCheck{}, parentD: map[string]PurposeCheck{},
		nodes0: len(b.order), edges0: len(b.edges)}
	if im.cfg == nil {
		im.cfg = &Config{}
	}
	for _, id := range b.order {
		if n := b.nodes[id]; n.Src != "" && n.Sup == "" {
			im.bySrc[n.Src] = id
		}
	}
	rep = &ImportReport{Files: []ImportFileReport{}, DryRun: opt.DryRun, Tables: []TableInfo{}, Concepts: []string{}}
	concepts0 := map[string]bool{}
	for _, id := range b.concepts() {
		concepts0[id] = true
	}
	im.report = rep
	maxSize := im.cfg.Import.Limits.FileSize
	if maxSize <= 0 {
		maxSize = 50 << 20
	}
	rules := []any{}
	for i, f := range files {
		if opt.Progress != nil {
			opt.Progress(i, len(files), f.Rel)
		}
		rel := filepath.ToSlash(f.Rel)
		if rel == "" {
			rel = f.Name
		}
		r := im.rule(rel)
		kind := detectImporter(rel, f.Data)
		if r != nil && r.Importer != "" {
			kind = r.Importer
			if ext := strings.ToLower(filepath.Ext(rel)); kind == "json" && (ext == ".jsonl" || ext == ".ndjson") {
				kind = "jsonl"
			}
		}
		fr := ImportFileReport{Path: rel, Importer: kind, Reasons: []string{}}
		im.fr, im.group = &fr, ""
		n0, e0 := len(b.order), len(b.edges)
		if r != nil && r.Skip {
			fr.Status, fr.Message = "skipped", "skipped by rule"
		} else if int64(len(f.Data)) > maxSize {
			fr.Status, fr.Message = "error", fmt.Sprintf("file larger than %d bytes", maxSize)
		} else {
			st, recs, msg, err := im.importFile(f, rel, kind, r)
			fr.Status, fr.Records, fr.Message = st, recs, msg
			if err != nil {
				fr.Status, fr.Message = "error", err.Error()
			}
		}
		fr.Nodes, fr.Edges = len(b.order)-n0, len(b.edges)-e0
		rep.Files = append(rep.Files, fr)
		if r == nil && fr.Status != "error" {
			rules = append(rules, im.suggestRule(rel, kind))
		}
	}
	im.resolveLinks()
	rep.Connectors = im.anchors()
	if len(im.fresh) > 0 {
		b.embedAll()
		if len(im.fresh) <= 5000 {
			b.autoLink(filterStr(im.fresh, func(id string) bool { return b.nodes[id] != nil }))
		}
		if im.cfg.Connectors.ImportHub {
			top := []string{}
			for _, id := range im.fresh {
				if n := b.nodes[id]; n != nil && n.Parent == "" && n.Kind == "file" {
					top = append(top, id)
				}
			}
			if len(top) > 1 {
				h, err := b.addConnector(ConnectorSpec{Type: "hub", Label: "import " + b.Now().UTC().Format("2006-01-02 15:04"), Key: "import:" + b.Now().UTC().Format("20060102T150405")})
				if err == nil {
					for _, id := range top {
						b.addEdge(id, h, .8, EdgeConnector)
					}
					rep.Connectors = append(rep.Connectors, h)
				}
			}
		}
		b.unlockedResolveEntities(im.cfg.Connectors.EntityAnchors)
		b.Step++
		b.embedAll()
		b.refresh()
	}
	for _, f := range rep.Files {
		rep.Totals.Files++
		rep.Totals.Admitted += f.Admitted
		rep.Totals.Quarantined += f.Quarantined
		rep.Totals.Rejected += f.Rejected
		switch f.Status {
		case "created":
			rep.Totals.Created++
		case "updated":
			rep.Totals.Updated++
		case "skipped":
			rep.Totals.Skipped++
		case "error":
			rep.Totals.Errors++
		}
		rep.Totals.Records += f.Records
	}
	rep.Totals.Nodes, rep.Totals.Edges = len(b.order)-im.nodes0, len(b.edges)-im.edges0
	for _, id := range b.concepts() {
		if !concepts0[id] {
			rep.Concepts = append(rep.Concepts, b.nodes[id].Text)
		}
	}
	if len(rules) > 0 {
		m := NewOMap().Set("import", NewOMap().Set("rules", rules))
		rep.Mapping = EmitYAML(m)
	} else {
		rep.Mapping = "# every file matched a configured rule\n"
	}
	rep.Status = b.status("")
	if opt.Progress != nil {
		opt.Progress(len(files), len(files), "")
	}
	return rep, nil
}

func (b *Brain) unlockedResolveEntities(types []string) {
	for _, t := range types {
		b.mu.Unlock()
		_, _ = b.ResolveEntities(t, 2, false)
		b.mu.Lock()
	}
}

func (im *importer) suggestRule(rel, kind string) any {
	m := NewOMap().Set("match", rel).Set("importer", kind)
	if t, ok := im.mapping.Vals[rel]; ok {
		m.Set("tables", t)
	}
	return m
}

// upsert creates a node or handles an existing one with the same src (skip unchanged, supersede on update).
// Returns the id and "created" | "updated" | "skipped".
func (im *importer) upsert(n *Node, src, content string) (string, string) {
	b := im.b
	h := sha(content)
	if _, exists := im.bySrc[src]; !exists && b.purpose != nil && b.purpose.mode() != "off" && b.purpose.applies("import") {
		var c PurposeCheck
		switch {
		case n.Kind == "chunk" && n.Parent != "":
			pc, ok := im.parentD[n.Parent]
			if !ok || (b.purpose.Admission.Chunks == "each") {
				pc, _ = b.gateText(n.Text, n.Kind, n.Section, "import", nil)
			}
			c = pc
		case im.group != "" && b.purpose.Admission.Records != "each":
			c = im.groupDe[im.group]
		default:
			c, _ = b.gateText(n.Text+"\n"+short(content, 4000), n.Kind, n.Section, "import", n.Fields)
		}
		if !b.applyDecision(n, c) {
			if im.fr != nil {
				im.fr.Rejected++
				im.addReason(c)
			}
			return "", "rejected"
		}
		if im.fr != nil && n.Kind != "chunk" {
			if isQuarantined(n) {
				im.fr.Quarantined++
				im.addReason(c)
			} else {
				im.fr.Admitted++
			}
		}
		defer func() { im.parentD[n.ID] = c }()
	}
	n.Src, n.Hash = src, h
	if old, ok := im.bySrc[src]; ok && b.nodes[old] != nil {
		if b.nodes[old].Hash == h || !im.opt.Update {
			return old, "skipped"
		}
		if n.ID == "" || b.nodes[n.ID] != nil {
			n.ID = b.newID(n.Kind)
		}
		n.Touched = b.nowMs()
		b.addNode(n)
		b.nodes[old].Sup = n.ID
		b.addEdge(old, n.ID, .5, EdgeSupersede)
		im.bySrc[src] = n.ID
		im.fresh = append(im.fresh, n.ID)
		return n.ID, "updated"
	}
	if n.ID == "" || b.nodes[n.ID] != nil {
		n.ID = b.newID(n.Kind)
	}
	n.Touched = b.nowMs()
	if n.Base == 0 {
		n.Base = b.kind(n.Kind).Base
	}
	b.addNode(n)
	b.addSection(n.Section)
	im.bySrc[src] = n.ID
	im.fresh = append(im.fresh, n.ID)
	return n.ID, "created"
}

func worst(a, s string) string {
	rank := map[string]int{"": 0, "skipped": 1, "created": 2, "updated": 3}
	if rank[s] > rank[a] {
		return s
	}
	return a
}

var tplVar = regexp.MustCompile(`\{([\w.\-]+)\}`)

func renderRecordText(tpl string, fs Fields) string {
	if tpl == "" {
		parts := []string{}
		for i, f := range fs {
			if i >= 8 {
				break
			}
			if f.Value != "" {
				parts = append(parts, f.Name+": "+f.Value)
			}
		}
		return strings.Join(parts, " · ")
	}
	return tplVar.ReplaceAllStringFunc(tpl, func(m string) string {
		v, _ := fs.Get(m[1 : len(m)-1])
		return v
	})
}

func (im *importer) conceptsFor(id string, names []string, fs Fields) {
	for _, c := range names {
		v, ok := fs.Get(c)
		if !ok {
			// a literal concept label
			im.b.addEdge(id, im.b.findConcept(c, true), .7, EdgeConcept)
			continue
		}
		for _, part := range strings.Split(v, ", ") {
			if part = strings.TrimSpace(part); part != "" && len(part) <= 60 {
				im.b.addEdge(id, im.b.findConcept(part, true), .6, EdgeConcept)
			}
		}
	}
}

func (im *importer) record(src string, kind, section, text string, fs Fields, concepts []string, labels []string, parent string, idx int) (string, string) {
	b := im.b
	if kind == "" {
		kind = "record"
	}
	if _, ok := b.kindM[kind]; !ok {
		kind = "record"
	}
	if section == "" {
		section = b.kind(kind).Section
	}
	content, _ := fs.MarshalJSON()
	n := &Node{Kind: kind, Section: section, Text: text, Fields: fs, Parent: parent, Idx: idx,
		Labels: uniqSorted(append(append([]string{}, labels...), b.defaultLabels(section, kind, "")...))}
	id, st := im.upsert(n, src, string(content)+"\x00"+text)
	if id == "" {
		return "", st
	}
	if st != "skipped" {
		im.conceptsFor(id, concepts, fs)
		if parent != "" {
			b.addEdge(parent, id, .9, EdgePart)
		}
	}
	return id, st
}

// fileNode creates the container node of an imported file.
func (im *importer) fileNode(rel string, data []byte, kind, section, text string, labels []string) (string, string) {
	if kind == "" {
		kind = "file"
	}
	if section == "" {
		section = "files"
	}
	n := &Node{Kind: kind, Section: section, Text: text, Meta: &FileMeta{Name: filepath.Base(rel), Mime: mimeOf(rel), Size: int64(len(data))}, Labels: labels}
	return im.upsert(n, rel, string(data))
}

func mimeOf(rel string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(rel)), ".")
	switch {
	case ext == "pdf":
		return "application/pdf"
	case imgExt.MatchString(rel):
		return "image/" + ext
	}
	return "text/" + ext
}

func (im *importer) importFile(f InFile, rel, kind string, r *ImportRule) (string, int, string, error) {
	labels := im.b.defaultLabels("", "", rel)
	if r != nil {
		labels = uniqSorted(append(labels, r.Labels...))
	}
	switch kind {
	case "markdown":
		return im.markdown(f, rel, r, labels)
	case "csv":
		return im.csv(f, rel, r, labels)
	case "json", "jsonl", "yaml":
		return im.json(f, rel, kind, r, labels)
	case "sql":
		return im.sql(f, rel, r, labels)
	case "sqlite":
		return im.sqlite(f, rel, r, labels)
	}
	text := ""
	switch kind {
	case "docx":
		t, err := docxText(f.Data)
		if err != nil {
			return "", 0, "", err
		}
		text = t
	case "html":
		text = htmlText(string(f.Data))
	case "pdf":
		t, err := pdfText(f)
		if err != nil {
			return "skipped", 0, "pdftotext missing or PDF without text", nil
		}
		text = t
	case "image":
	default:
		text = string(f.Data)
	}
	if len(im.b.RedactOnIngest) > 0 {
		text = im.b.patOpts().Redact(text, im.b.RedactOnIngest)
	}
	k, sec := "file", "files"
	if r != nil {
		if r.Kind != "" {
			k = r.Kind
		}
		if r.Section != "" {
			sec = r.Section
		}
	}
	title := filepath.Base(rel)
	if fl := firstLine(text); fl != "" {
		title += " — " + short(fl, 80)
	}
	fid, st := im.fileNode(rel, f.Data, k, sec, title, uniqSorted(append(labels, im.b.defaultLabels(sec, k, "")...)))
	if st == "skipped" || st == "rejected" {
		return st, 0, "", nil
	}
	n := im.chunks(fid, rel, text, labels)
	return st, 0, fmt.Sprintf("%d chunks", n), nil
}

func (im *importer) chunks(fid, rel, text string, labels []string) int {
	if text == "" {
		return 0
	}
	max := im.cfg.Import.ChunkSize
	if max <= 0 {
		max = 650
	}
	cs := chunkText(text, max)
	lim := im.cfg.Import.Limits.Chunks
	if lim <= 0 {
		lim = 200
	}
	if len(cs) > lim {
		cs = cs[:lim]
	}
	prev := ""
	for i, c := range cs {
		sec := im.b.nodes[fid].Section
		if pm := im.b.nodes[fid].Purpose; pm != nil && pm.Section != "" {
			sec = pm.Section
		}
		n := &Node{Kind: "chunk", Section: sec, Text: c, Parent: fid, Idx: i, Labels: filterStr(im.b.nodes[fid].Labels, func(l string) bool { return l != QuarantineLabel })}
		id, _ := im.upsert(n, fmt.Sprintf("%s#chunk/%d", rel, i), c)
		if id == "" {
			continue
		}
		im.b.addEdge(fid, id, .9, EdgePart)
		if prev != "" {
			im.b.addEdge(prev, id, .5, EdgeSeq)
		}
		prev = id
	}
	return len(cs)
}

// ---------------------------------------------------------------- markdown

var (
	wikiLink = regexp.MustCompile(`\[\[([^\]|#]+)(?:[#|][^\]]*)?\]\]`)
	mdLinkRe = regexp.MustCompile(`\]\(([^)\s]+\.(?:md|markdown|mdx))(?:#[^)]*)?\)`)
	headRe   = regexp.MustCompile(`(?m)^(#{1,2})\s+(.+?)\s*#*\s*$`)
)

func (im *importer) markdown(f InFile, rel string, r *ImportRule, labels []string) (string, int, string, error) {
	b := im.b
	text := string(f.Data)
	fs := Fields{}
	var fm *OMap
	if strings.HasPrefix(text, "---\n") || strings.HasPrefix(text, "---\r\n") {
		rest := text[4:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			v, err := ParseYAML(rest[:end])
			if err == nil {
				fm, _ = v.(*OMap)
			}
			text = rest[end+4:]
			if i := strings.Index(text, "\n"); i >= 0 {
				text = text[i+1:]
			}
		}
	}
	kind, section, title := "file", "", ""
	concepts := []string{}
	if r != nil {
		if r.Kind != "" {
			kind = r.Kind
		}
		section = r.Section
		concepts = append(concepts, r.Concepts...)
	}
	if fm != nil {
		for _, k := range fm.Keys {
			v := fm.Vals[k]
			switch k {
			case "tags":
				for _, t := range anyList(v) {
					concepts = append(concepts, t)
				}
				continue
			case "kind":
				if _, ok := b.kindM[scalarString(v)]; ok {
					kind = scalarString(v)
				}
				continue
			case "section":
				section = scalarString(v)
				continue
			case "title":
				title = scalarString(v)
			}
			if l, ok := v.([]any); ok {
				parts := []string{}
				for _, e := range l {
					parts = append(parts, scalarString(e))
				}
				fs = append(fs, Field{k, strings.Join(parts, ", ")})
			} else if m, ok := v.(*OMap); ok {
				d, _ := json.Marshal(m)
				fs = append(fs, Field{k, string(d)})
			} else {
				fs = append(fs, Field{k, scalarString(v)})
			}
		}
	}
	if title == "" {
		title = firstLine(text)
	}
	if section == "" {
		section = b.kind(kind).Section
	}
	if len(b.RedactOnIngest) > 0 {
		text = b.patOpts().Redact(text, b.RedactOnIngest)
	}
	labels = uniqSorted(append(labels, b.defaultLabels(section, kind, "")...))
	summary := title
	if p := firstParagraph(text); p != "" && p != title {
		summary += " — " + short(p, 160)
	}
	n := &Node{Kind: kind, Section: section, Text: summary, Fields: fs, Meta: &FileMeta{Name: filepath.Base(rel), Mime: "text/markdown", Size: int64(len(f.Data))}, Labels: labels}
	id, st := im.upsert(n, rel, string(f.Data))
	if id == "" {
		return st, 0, "", nil
	}
	im.docs[strings.ToLower(strings.TrimSuffix(rel, filepath.Ext(rel)))] = id
	im.docs[strings.ToLower(strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)))] = id
	if st == "skipped" {
		return st, 0, "unchanged", nil
	}
	for _, c := range concepts {
		b.addEdge(id, b.findConcept(c, true), .75, EdgeConcept)
	}
	hac := true
	if r != nil && r.HeadingsAsConcepts != nil {
		hac = *r.HeadingsAsConcepts
	}
	if hac {
		for _, m := range headRe.FindAllStringSubmatch(text, -1) {
			h := strings.TrimSpace(m[2])
			if h != "" && jsLen(h) <= 60 && !strings.EqualFold(h, title) {
				b.addEdge(id, b.findConcept(h, true), .6, EdgeConcept)
			}
		}
	}
	for _, m := range wikiLink.FindAllStringSubmatch(text, -1) {
		im.links = append(im.links, mdLink{id, strings.TrimSpace(m[1]), filepath.Dir(rel)})
	}
	for _, m := range mdLinkRe.FindAllStringSubmatch(text, -1) {
		if !strings.Contains(m[1], "://") {
			im.links = append(im.links, mdLink{id, m[1], filepath.Dir(rel)})
		}
	}
	c := im.chunks(id, rel, text, labels)
	return st, 0, fmt.Sprintf("%d chunks, %d fields", c, len(fs)), nil
}

func firstParagraph(t string) string {
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(t, -1) {
		p = strings.TrimSpace(p)
		if p != "" && !strings.HasPrefix(p, "#") {
			return flat(p)
		}
	}
	return ""
}

func (im *importer) resolveLinks() {
	for _, l := range im.links {
		t := strings.ToLower(strings.TrimSuffix(l.target, filepath.Ext(l.target)))
		cands := []string{t, strings.ToLower(filepath.ToSlash(filepath.Join(l.dir, t))), strings.ToLower(filepath.Base(t)), slug(t, "-")}
		for _, c := range cands {
			if id, ok := im.docs[c]; ok && id != l.from {
				im.b.addEdge(l.from, id, .7, EdgeAssoc)
				break
			}
		}
	}
}

// ---------------------------------------------------------------- csv / json

func (im *importer) csv(f InFile, rel string, r *ImportRule, labels []string) (string, int, string, error) {
	recs, ok := parseCSV(string(f.Data), strings.HasSuffix(strings.ToLower(rel), ".tsv"))
	if !ok {
		return "", 0, "", fmt.Errorf("not a CSV with a header row")
	}
	rows := []Fields{}
	for _, row := range recs[1:] {
		fs := Fields{}
		for c, h := range recs[0] {
			v := ""
			if c < len(row) {
				v = strings.TrimSpace(row[c])
			}
			fs = append(fs, Field{h, v})
		}
		rows = append(rows, fs)
	}
	stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	return im.rows(f, rel, stem, rows, r, labels, fmt.Sprintf("%d columns", len(recs[0])))
}

func (im *importer) rows(f InFile, rel, stem string, rows []Fields, r *ImportRule, labels []string, info string) (string, int, string, error) {
	kind, section, text := "record", stem, ""
	concepts := []string{}
	if r != nil {
		if r.Kind != "" {
			kind = r.Kind
		}
		if r.Section != "" {
			section = r.Section
		}
		text, concepts = r.Text, r.Concepts
	}
	lim := im.cfg.Import.Limits.Records
	if lim <= 0 {
		lim = 100000
	}
	if len(rows) > lim {
		rows = rows[:lim]
	}
	head := ""
	if len(rows) > 0 {
		names := []string{}
		for _, x := range rows[0] {
			names = append(names, x.Name)
		}
		head = strings.Join(firstN(names, 12), ", ")
	}
	im.startGroup(rel, stem, rows, kind, section)
	fid, st := im.fileNode(rel, f.Data, "file", "files", fmt.Sprintf("%s — %d rows: %s", filepath.Base(rel), len(rows), head), uniqSorted(append(labels, im.b.defaultLabels("files", "file", "")...)))
	status := ""
	if st != "skipped" {
		status = st
	}
	prev := ""
	for i, fs := range rows {
		id, s := im.record(fmt.Sprintf("%s#%d", rel, i), kind, section, renderRecordText(text, fs), fs, concepts, labels, fid, i)
		status = worst(status, s)
		if prev != "" && s != "skipped" {
			im.b.addEdge(prev, id, .5, EdgeSeq)
		}
		prev = id
	}
	if status == "" {
		status = "skipped"
		if st == "rejected" {
			status = "rejected"
		}
	}
	cols := NewOMap().Set("kind", kind).Set("section", section)
	im.mapping.Set(rel, cols)
	ti := TableInfo{Name: section, File: rel, Rows: len(rows), Columns: []string{}}
	if len(rows) > 0 {
		for _, f := range rows[0] {
			ti.Columns = append(ti.Columns, f.Name)
		}
	}
	im.report.Tables = append(im.report.Tables, ti)
	return status, len(rows), info, nil
}

// flatten turns nested objects into dotted field names (depth ≤ 3); scalar arrays are joined with ", ".
func flatten(prefix string, v any, depth int, out *Fields) {
	switch x := v.(type) {
	case *OMap:
		if depth >= 3 {
			d, _ := json.Marshal(x)
			*out = append(*out, Field{prefix, string(d)})
			return
		}
		for _, k := range x.Keys {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flatten(p, x.Vals[k], depth+1, out)
		}
	case []any:
		scal := true
		parts := []string{}
		for _, e := range x {
			switch e.(type) {
			case *OMap, []any:
				scal = false
			}
			parts = append(parts, scalarString(e))
		}
		if scal {
			*out = append(*out, Field{prefix, strings.Join(parts, ", ")})
		} else {
			d, _ := json.Marshal(x)
			*out = append(*out, Field{prefix, string(d)})
		}
	case nil:
		*out = append(*out, Field{prefix, ""})
	default:
		*out = append(*out, Field{prefix, scalarString(x)})
	}
}

func jsonPath(v any, path string) any {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "$"), ".")
	if path == "" {
		return v
	}
	for _, p := range strings.Split(path, ".") {
		m, ok := v.(*OMap)
		if !ok {
			return nil
		}
		v = m.Vals[p]
	}
	return v
}

func (im *importer) json(f InFile, rel, kind string, r *ImportRule, labels []string) (string, int, string, error) {
	var docs []any
	switch kind {
	case "jsonl":
		for i, line := range strings.Split(string(f.Data), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			v, err := DecodeOrderedJSON([]byte(line))
			if err != nil {
				return "", 0, "", fmt.Errorf("line %d: %v", i+1, err)
			}
			docs = append(docs, v)
		}
	case "yaml":
		v, err := ParseYAML(string(f.Data))
		if err != nil {
			return "", 0, "", err
		}
		docs = []any{v}
	default:
		v, err := DecodeOrderedJSON(f.Data)
		if err != nil {
			return "", 0, "", err
		}
		docs = []any{v}
	}
	var root any = docs
	if kind != "jsonl" {
		root = docs[0]
		if m, ok := root.(*OMap); ok {
			if _, has := m.Vals["memories"]; has {
				if _, c := m.Vals["concepts"]; c {
					return im.seedFile(f, rel, labels)
				}
			}
			if items, ok := m.Vals["items"].([]any); ok && (r == nil || r.JSON.Path == "") && looksLikeCtxc(items) {
				var j struct {
					Items []ctxItem `json:"items"`
				}
				if err := json.Unmarshal(f.Data, &j); err == nil {
					ids := im.b.importItems(j.Items, labels...)
					im.fresh = append(im.fresh, ids...)
					return "created", len(ids), "ctxc state.json items", nil
				}
			}
		}
		path := ""
		if r != nil {
			path = r.JSON.Path
		}
		if path != "" {
			root = jsonPath(root, path)
		} else if m, ok := root.(*OMap); ok {
			// pick the first array of objects
			for _, k := range m.Keys {
				if l, ok := m.Vals[k].([]any); ok && len(l) > 0 {
					if _, isObj := l[0].(*OMap); isObj {
						root = l
						break
					}
				}
			}
		}
	}
	list, ok := root.([]any)
	if !ok {
		if m, isMap := root.(*OMap); isMap {
			list = []any{m}
		} else {
			return "", 0, "", fmt.Errorf("no array of objects found (set json.path)")
		}
	}
	rows := []Fields{}
	for _, it := range list {
		fs := Fields{}
		flatten("", it, 0, &fs)
		if len(fs) == 1 && fs[0].Name == "" {
			fs[0].Name = "value"
		}
		rows = append(rows, fs)
	}
	stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	return im.rows(f, rel, stem, rows, r, labels, fmt.Sprintf("%d objects", len(rows)))
}

func looksLikeCtxc(items []any) bool {
	if len(items) == 0 {
		return false
	}
	m, ok := items[0].(*OMap)
	if !ok {
		return false
	}
	_, t := m.Vals["text"]
	_, k := m.Vals["kind"]
	return t && k
}

func (im *importer) seedFile(f InFile, rel string, labels []string) (string, int, string, error) {
	var s Seed
	if err := json.Unmarshal(f.Data, &s); err != nil {
		return "", 0, "", err
	}
	items := []ctxItem{}
	for _, m := range s.Memories {
		items = append(items, ctxItem{ID: m.ID, Kind: m.Kind, Section: m.Section, Text: m.Text, Pinned: m.Pinned, Tags: m.Concepts})
	}
	for _, c := range s.Concepts {
		im.b.findConcept(firstNonEmpty(c.Label, c.ID), true)
	}
	ids := im.b.importItems(items, labels...)
	im.fresh = append(im.fresh, ids...)
	return "created", len(ids), "brainDB seed: memories imported as items", nil
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// ---------------------------------------------------------------- anchors from config

func (im *importer) anchors() []string {
	b := im.b
	created := []string{}
	for _, a := range im.cfg.Connectors.Anchors {
		groups := map[string][]string{}
		label := map[string]string{}
		order := []string{}
		for _, id := range b.order {
			n := b.nodes[id]
			if n.Kind == Concept || n.Conn != nil || n.Sup != "" {
				continue
			}
			for mi, m := range a.Match {
				tbl, col, _ := strings.Cut(m, ".")
				if tbl != "fields" && tbl != n.Section && tbl != n.Kind && !strings.Contains(n.Src, "#"+tbl+"/") {
					continue
				}
				v, ok := n.Fields.Get(col)
				if !ok || strings.TrimSpace(v) == "" {
					continue
				}
				for _, part := range strings.Split(v, ", ") {
					part = lower(strings.TrimSpace(part))
					if part == "" {
						continue
					}
					if _, ok := groups[part]; !ok {
						order = append(order, part)
					}
					if !contains(groups[part], id) {
						groups[part] = append(groups[part], id)
					}
					if mi == 0 && label[part] == "" {
						if nm, ok := n.Fields.Get("name"); ok && nm != "" {
							label[part] = nm
						} else {
							label[part] = short(n.Text, 60)
						}
					}
				}
			}
		}
		for _, v := range order {
			ids := groups[v]
			if len(ids) < 2 {
				continue
			}
			l := label[v]
			if l == "" {
				l = a.Key + " " + v
			}
			existed := b.connectorByKey("anchor", a.Key+":"+v) != ""
			h, err := b.addConnector(ConnectorSpec{Type: "anchor", Label: l, Key: a.Key + ":" + v})
			if err != nil {
				continue
			}
			if existed {
				for _, id := range ids {
					b.addEdge(id, h, .9, EdgeConnector)
				}
				continue
			}
			for _, id := range ids {
				b.addEdge(id, h, .9, EdgeConnector)
			}
			created = append(created, h)
		}
	}
	return created
}

// ---------------------------------------------------------------- docx / html

var (
	tagRe    = regexp.MustCompile(`(?s)<[^>]*>`)
	scriptRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	hTagRe   = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	blockRe  = regexp.MustCompile(`(?i)</?(p|div|br|li|tr|section|article|ul|ol|table|blockquote)[^>]*>`)
)

func htmlText(s string) string {
	s = scriptRe.ReplaceAllString(s, "")
	s = hTagRe.ReplaceAllStringFunc(s, func(m string) string {
		x := hTagRe.FindStringSubmatch(m)
		return "\n\n" + strings.Repeat("#", int(x[1][0]-'0')) + " " + strings.TrimSpace(tagRe.ReplaceAllString(x[2], "")) + "\n\n"
	})
	s = blockRe.ReplaceAllString(s, "\n\n")
	s = tagRe.ReplaceAllString(s, "")
	return xmlUnesc(strings.NewReplacer("&nbsp;", " ").Replace(s))
}

func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("not a DOCX (zip): %v", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(rc)
		rc.Close()
		x := buf.String()
		x = regexp.MustCompile(`</w:p>`).ReplaceAllString(x, "\n\n")
		x = regexp.MustCompile(`<w:tab/>`).ReplaceAllString(x, "\t")
		return xmlUnesc(tagRe.ReplaceAllString(x, "")), nil
	}
	return "", fmt.Errorf("DOCX without word/document.xml")
}

// ---------------------------------------------------------------- sqlite (via the sqlite3 CLI)

func (im *importer) sqlite(f InFile, rel string, r *ImportRule, labels []string) (string, int, string, error) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return "skipped", 0, "sqlite3 CLI not found", nil
	}
	p := f.Path
	if p == "" {
		tmp, err := os.CreateTemp("", "braindb-*.sqlite")
		if err != nil {
			return "", 0, "", err
		}
		defer os.Remove(tmp.Name())
		_, _ = tmp.Write(f.Data)
		tmp.Close()
		p = tmp.Name()
	}
	out, err := exec.Command("sqlite3", "-json", p, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Output()
	if err != nil {
		return "", 0, "", fmt.Errorf("sqlite3: %v", err)
	}
	var tables []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(out, &tables)
	total := 0
	status := ""
	for _, t := range tables {
		if tc, ok := tableCfg(r, t.Name); ok && tc.Skip {
			continue
		}
		d, err := exec.Command("sqlite3", "-json", p, fmt.Sprintf(`SELECT * FROM "%s"`, strings.ReplaceAll(t.Name, `"`, `""`))).Output()
		if err != nil {
			continue
		}
		v, err := DecodeOrderedJSON(bytes.TrimSpace(append(d, []byte{}...)))
		if err != nil || v == nil {
			continue
		}
		list, _ := v.([]any)
		tb := &sqlTable{name: t.Name}
		for _, it := range list {
			fs := Fields{}
			flatten("", it, 0, &fs)
			tb.rows = append(tb.rows, fs)
		}
		if len(tb.rows) > 0 {
			for _, x := range tb.rows[0] {
				tb.cols = append(tb.cols, x.Name)
			}
		}
		s, n := im.sqlTableRecords(rel, tb, r, labels)
		status = worst(status, s)
		total += n
	}
	im.sqlForeignKeys(rel)
	if status == "" {
		status = "skipped"
	}
	return status, total, fmt.Sprintf("%d tables", len(tables)), nil
}

func tableCfg(r *ImportRule, name string) (TableConfig, bool) {
	if r == nil || r.Tables == nil {
		return TableConfig{}, false
	}
	t, ok := r.Tables[name]
	return t, ok
}

var _ = sort.Strings
var _ = time.Now

// CollectFiles reads files from paths: directories (walked; paths relative to the directory), files, and globs with **.
func CollectFiles(paths []string, maxFiles int) ([]InFile, error) {
	if maxFiles <= 0 {
		maxFiles = 100000
	}
	out := []InFile{}
	add := func(p, rel string) error {
		if len(out) >= maxFiles {
			return nil
		}
		d, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, InFile{Name: filepath.Base(p), Data: d, Path: p, Rel: filepath.ToSlash(rel)})
		return nil
	}
	for _, p := range paths {
		if strings.ContainsAny(p, "*?[") {
			root := p
			for strings.ContainsAny(root, "*?[") {
				root = filepath.Dir(root)
			}
			pat, _ := filepath.Rel(root, p)
			err := filepath.Walk(root, func(fp string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(root, fp)
				if globMatch(filepath.ToSlash(pat), filepath.ToSlash(rel)) {
					return add(fp, rel)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("does not exist: %s", p)
		}
		if !st.IsDir() {
			if err := add(p, filepath.Base(p)); err != nil {
				return nil, err
			}
			continue
		}
		err = filepath.Walk(p, func(fp string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			name := info.Name()
			if fp != p && (strings.HasPrefix(name, ".") || name == "node_modules") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(p, fp)
			return add(fp, rel)
		})
		if err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// Text renders an import report for the CLI / MCP.
func (r *ImportReport) Text() string {
	var b strings.Builder
	for _, f := range r.Files {
		fmt.Fprintf(&b, "%-8s %-9s %s · %d nodes, %d edges, %d records", f.Status, f.Importer, f.Path, f.Nodes, f.Edges, f.Records)
		if f.Quarantined+f.Rejected > 0 {
			fmt.Fprintf(&b, " · %d quarantined, %d rejected", f.Quarantined, f.Rejected)
		}
		if f.Message != "" {
			b.WriteString(" · " + f.Message)
		}
		b.WriteString("\n")
	}
	t := r.Totals
	fmt.Fprintf(&b, "totals: %d files (%d created, %d updated, %d skipped, %d errors) · %d nodes, %d edges, %d records", t.Files, t.Created, t.Updated, t.Skipped, t.Errors, t.Nodes, t.Edges, t.Records)
	if len(r.Connectors) > 0 {
		fmt.Fprintf(&b, " · %d connectors", len(r.Connectors))
	}
	if t.Quarantined+t.Rejected > 0 {
		fmt.Fprintf(&b, " · purpose: %d admitted, %d quarantined, %d rejected", t.Admitted, t.Quarantined, t.Rejected)
		for _, f := range r.Files {
			for _, x := range f.Reasons {
				fmt.Fprintf(&b, "\n  %s: %s", f.Path, x)
			}
		}
	}
	if r.DryRun {
		b.WriteString(" · dry run, nothing stored")
	}
	b.WriteString("\ndetected mapping (paste into braindb.yaml):\n" + r.Mapping)
	return b.String()
}

func (im *importer) addReason(c PurposeCheck) {
	r := fmt.Sprintf("%s (score %.2f, top match %q", c.Decision, c.Score, c.TopMatch)
	if c.TopConflict != "" {
		r += fmt.Sprintf(", conflict %q", c.TopConflict)
	}
	r += ")"
	if len(c.Rules) > 0 {
		r += ": " + strings.Join(c.Rules, "; ")
	}
	if !contains(im.fr.Reasons, r) && len(im.fr.Reasons) < 10 {
		im.fr.Reasons = append(im.fr.Reasons, r)
	}
}

// startGroup scores a table or record file once on its name, columns and up to 20 sample rows.
func (im *importer) startGroup(key, name string, rows []Fields, kind, section string) {
	im.group = key
	if _, ok := im.groupDe[key]; ok {
		return
	}
	cols := []string{}
	if len(rows) > 0 {
		for _, f := range rows[0] {
			cols = append(cols, strings.ReplaceAll(f.Name, "_", " "))
		}
	}
	parts := []string{name, strings.Join(cols, " ")}
	for i, r := range rows {
		if i >= 20 {
			break
		}
		parts = append(parts, renderRecordText("", r))
	}
	c, _ := im.b.gateText(strings.Join(parts, "\n"), kind, section, "import", nil)
	im.groupDe[key] = c
}
