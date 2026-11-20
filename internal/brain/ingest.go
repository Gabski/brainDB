// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

// chunkText splits text into chunks of at most max UTF-16 units (≤ 40 chunks), like the reference algorithm.
func chunkText(t string, max int) []string {
	t = strings.ReplaceAll(t, "\r", "")
	rs := []rune(t)
	parts := []string{}
	start := 0
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\n' {
			continue
		}
		// alternative 1: \n\s*\n (greedy: up to the last \n in the following whitespace run)
		k := i + 1
		for k < len(rs) && isJSSpace(rs[k]) {
			k++
		}
		last := -1
		for j := k - 1; j > i; j-- {
			if rs[j] == '\n' {
				last = j
				break
			}
		}
		if last > 0 {
			parts = append(parts, string(rs[start:i]))
			start = last + 1
			i = last
			continue
		}
		// alternative 2: \n(?=#{1,6} )
		h := 0
		for i+1+h < len(rs) && rs[i+1+h] == '#' {
			h++
		}
		if h >= 1 && h <= 6 && i+1+h < len(rs) && rs[i+1+h] == ' ' {
			parts = append(parts, string(rs[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, string(rs[start:]))
	out := []string{}
	cur := []uint16(nil)
	for _, ps := range parts {
		ps = jsTrim(collapseSpace(ps))
		if jsLen(ps) <= 2 {
			continue
		}
		p := u16(ps)
		for len(p) > max {
			dot := lastIndexU16(p, []uint16{'.', ' '}, max)
			cut := max
			if float64(dot) > float64(max)*.4 {
				cut = dot + 1
			}
			if len(cur) > 0 {
				out = append(out, fromU16(cur))
				cur = nil
			}
			out = append(out, jsTrim(fromU16(p[:cut])))
			p = u16(jsTrim(fromU16(p[cut:])))
		}
		if len(cur) > 0 && len(cur)+1+len(p) > max {
			out = append(out, fromU16(cur))
			cur = p
		} else if len(cur) > 0 {
			cur = append(append(cur, ' '), p...)
		} else {
			cur = p
		}
	}
	if len(cur) > 0 {
		out = append(out, fromU16(cur))
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func lastIndexU16(s, sub []uint16, from int) int {
	if from > len(s)-len(sub) {
		from = len(s) - len(sub)
	}
	for i := from; i >= 0; i-- {
		ok := true
		for j := range sub {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

var (
	textExt = regexp.MustCompile(`(?i)\.(md|markdown|txt|csv|tsv|json|xml|html?|js|mjs|ts|go|py|php|ya?ml|sql|log|ini|toml|sh|puml|rst)$`)
	imgExt  = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp|bmp|tiff?|svg|heic)$`)
	brainRe = regexp.MustCompile(`<brain[\s>]`)
	ctxRe   = regexp.MustCompile(`<context[\s>]`)
	hashRe  = regexp.MustCompile(`^#+\s*`)
)

// IngestResult is the result of an ingest.
type IngestResult struct {
	Report []string   `json:"report"`
	Fresh  []string   `json:"fresh"`
	Links  []AutoLink `json:"links"`
	Status string     `json:"status"`
}

// InFile is an in-memory file to ingest (e.g. a multipart upload).
type InFile struct {
	Name string
	Data []byte
	Path string // optional, used for pdftotext
	Rel  string // path relative to the ingest root (for import rules and default labels)
}

type ctxItem struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Section   string            `json:"section"`
	Text      string            `json:"text"`
	Pinned    bool              `json:"pinned"`
	Weight    *float64          `json:"weight"`
	Tags      []string          `json:"tags"`
	Refs      []string          `json:"refs"`
	UpdatedAt string            `json:"updated_at"`
	CreatedAt string            `json:"created_at"`
	Blob      *FileMeta         `json:"blob"`
	Fields    map[string]string `json:"fields"`
}

func (b *Brain) linkTags(id string, tags []string) {
	for _, t := range tags {
		ts := stems(t)
		for _, A := range b.concepts() {
			for x := range stems(b.nodes[A].Text) {
				if ts[x] {
					b.addEdge(id, A, .7, EdgeConcept)
					break
				}
			}
		}
	}
}

func (b *Brain) importItems(items []ctxItem, labels ...string) []string {
	ids := []string{}
	m := map[string]string{}
	for _, it := range items {
		kind := it.Kind
		if _, ok := b.kindM[kind]; !ok || kind == Concept {
			kind = "note"
		}
		id := it.ID
		if id == "" || b.nodes[id] != nil {
			id = b.newID(kind)
		}
		ts := it.UpdatedAt
		if ts == "" {
			ts = it.CreatedAt
		}
		age := 0.0
		if t, ok := parseISO(ts); ok && ts != "" {
			age = float64(b.nowMs()-t) / dayMs
			if age < 0 {
				age = 0
			}
		}
		text := it.Text
		if text == "" && it.Blob != nil {
			text = it.Blob.Name
		}
		if text == "" {
			text = id
		}
		sec := it.Section
		if sec == "" {
			sec = b.kind(kind).Section
		}
		n := &Node{ID: id, Kind: kind, Section: sec, Text: text, Age: roundN(age, 1), Pin: it.Pinned, Base: b.kind(kind).Base,
			Labels: uniqSorted(append(append([]string{}, labels...), b.defaultLabels(sec, kind, "")...))}
		if it.Weight != nil {
			n.Base = *it.Weight
		}
		if it.Blob != nil {
			meta := *it.Blob
			n.Meta = &meta
		}
		for _, k := range sortedKeys(it.Fields) {
			n.Fields = append(n.Fields, Field{k, it.Fields[k]})
		}
		b.addNode(n)
		b.addSection(sec)
		if it.ID != "" {
			m[it.ID] = id
		}
		b.linkTags(id, it.Tags)
		ids = append(ids, id)
	}
	for _, it := range items {
		for _, r := range it.Refs {
			a := m[it.ID]
			t := r
			if x, ok := m[r]; ok {
				t = x
			}
			if a != "" && b.nodes[t] != nil {
				b.addEdge(a, t, .6, EdgeAssoc)
			}
		}
	}
	return ids
}

func (b *Brain) importContextXML(txt string) ([]string, error) {
	root := ParseXML(txt)
	if root == nil {
		return nil, errf("invalid XML")
	}
	items := []ctxItem{}
	for _, sec := range root.All("section", nil) {
		for _, it := range sec.Kids("item") {
			text := jsTrim(it.Text)
			if text == "" {
				text = it.A("name")
			}
			items = append(items, ctxItem{ID: it.A("id"), Kind: it.A("kind"), Section: sec.A("name"), Text: text, Pinned: it.A("pinned") == "1",
				Tags: splitIDs(it.A("tags")), Refs: splitIDs(it.A("refs"))})
		}
	}
	return b.importItems(items), nil
}

func firstLine(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if s := jsTrim(hashRe.ReplaceAllString(l, "")); s != "" {
			return s
		}
	}
	return ""
}

func (b *Brain) ingestFile(f InFile, report, fresh *[]string) {
	name := filepath.Base(f.Name)
	rel := f.Rel
	if rel == "" {
		rel = name
	}
	lbls := b.defaultLabels("", "", filepath.ToSlash(rel))
	inherit := func(sec, kind string) []string {
		return uniqSorted(append(append([]string{}, lbls...), b.defaultLabels(sec, kind, "")...))
	}
	ext := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		ext = lower(name[i+1:])
	}
	size := int64(len(f.Data))
	if size > 5e6 {
		*report = append(*report, fmt.Sprintf("warn: %s: > 5 MB, skipped", name))
		return
	}
	if ext == "json" {
		var j struct {
			Items []ctxItem `json:"items"`
			Goal  string    `json:"goal"`
		}
		if json.Unmarshal(f.Data, &j) == nil && j.Items != nil {
			ids := b.importItems(j.Items, lbls...)
			*fresh = append(*fresh, ids...)
			g := ""
			if j.Goal != "" {
				g = fmt.Sprintf(" · goal %q", j.Goal)
			}
			*report = append(*report, fmt.Sprintf("ctxc workspace %s: %d context items%s", name, len(ids), g))
			return
		}
	}
	if ext == "xml" {
		t := string(f.Data)
		if brainRe.MatchString(t) {
			*report = append(*report, fmt.Sprintf("warn: %s: <brain> replaces the whole brain; load it through the UI (Brain XML) or --brain", name))
			return
		}
		if ctxRe.MatchString(t) {
			ids, err := b.importContextXML(t)
			if err == nil {
				*fresh = append(*fresh, ids...)
				*report = append(*report, fmt.Sprintf("ctxc context %s: %d items from <context>", name, len(ids)))
				return
			}
		}
	}
	text, mime := "", ext
	switch {
	case imgExt.MatchString(name):
		mime = "image/" + ext
	case ext == "pdf":
		mime = "application/pdf"
		t, err := pdfText(f)
		if err != nil {
			*report = append(*report, fmt.Sprintf("warn: %s: pdftotext missing or PDF has no text", name))
		}
		text = t
	case textExt.MatchString(name):
		text = string(f.Data)
		mime = "text/" + ext
	}
	if len(b.RedactOnIngest) > 0 && text != "" {
		text = b.patOpts().Redact(text, b.RedactOnIngest)
	}
	// purpose filter: one decision per file (records and chunks inherit it)
	dec, gated := b.gateText(name+"\n"+short(text, 4000), "file", "files", "ingest", nil)
	if gated && b.purpose.mode() == "enforce" && dec.Decision == "rejected" {
		*report = append(*report, fmt.Sprintf("rejected %s by the purpose filter (score %.2f; %s)", name, dec.Score, strings.Join(dec.Rules, "; ")))
		return
	}
	mark := func(n *Node) *Node {
		if gated {
			b.applyDecision(n, dec)
		}
		return n
	}
	if gated && dec.Decision == "quarantine" && b.purpose.mode() == "enforce" {
		*report = append(*report, fmt.Sprintf("quarantined %s for review (score %.2f)", name, dec.Score))
	}
	now := b.nowMs()
	fid := b.newID("file")
	label := name
	if first := firstLine(text); first != "" {
		label += " — " + short(first, 80)
	}
	if ext == "csv" || ext == "tsv" {
		if recs, ok := parseCSV(text, ext == "tsv"); ok {
			b.addNode(mark(&Node{ID: fid, Kind: "file", Section: "files", Touched: now, Meta: &FileMeta{name, mime, size},
				Text: fmt.Sprintf("%s — %d rows: %s", name, len(recs)-1, strings.Join(recs[0], ", ")), Base: b.kind("file").Base, Labels: inherit("files", "file")}))
			*fresh = append(*fresh, fid)
			sec := strings.TrimSuffix(name, filepath.Ext(name))
			b.addSection(sec)
			prev := ""
			rows := recs[1:]
			if len(rows) > 500 {
				rows = rows[:500]
			}
			for i, row := range rows {
				rid := b.newID("record")
				fs := Fields{}
				parts := []string{}
				for c, h := range recs[0] {
					v := ""
					if c < len(row) {
						v = strings.TrimSpace(row[c])
					}
					fs = append(fs, Field{h, v})
					if v != "" {
						parts = append(parts, h+": "+v)
					}
				}
				b.addNode(mark(&Node{ID: rid, Kind: "record", Section: sec, Text: strings.Join(parts, " · "), Touched: now, Parent: fid, Idx: i, Fields: fs, Base: b.kind("record").Base,
					Labels: inherit(sec, "record")}))
				b.addEdge(fid, rid, .9, EdgePart)
				if prev != "" {
					b.addEdge(prev, rid, .5, EdgeSeq)
				}
				prev = rid
				*fresh = append(*fresh, rid)
			}
			*report = append(*report, fmt.Sprintf("records %s: %d records → section %s · %.1f KB → %s", name, len(rows), sec, float64(size)/1024, fid))
			return
		}
	}
	b.addNode(mark(&Node{ID: fid, Kind: "file", Section: "files", Touched: now, Meta: &FileMeta{name, mime, size}, Text: label, Base: b.kind("file").Base, Labels: inherit("files", "file")}))
	*fresh = append(*fresh, fid)
	chunks := []string{}
	if text != "" {
		chunks = chunkText(text, 650)
	}
	prev := ""
	for i, c := range chunks {
		cid := b.newID("chunk")
		b.addNode(mark(&Node{ID: cid, Kind: "chunk", Section: "files", Text: c, Touched: now, Parent: fid, Idx: i, Base: b.kind("chunk").Base, Labels: inherit("files", "chunk")}))
		b.addEdge(fid, cid, .9, EdgePart)
		if prev != "" {
			b.addEdge(prev, cid, .5, EdgeSeq)
		}
		prev = cid
		*fresh = append(*fresh, cid)
	}
	desc := "no text, file node only"
	if len(chunks) > 0 {
		desc = fmt.Sprintf("%d chunks", len(chunks))
	}
	*report = append(*report, fmt.Sprintf("file %s: %s · %.1f KB → %s", name, desc, float64(size)/1024, fid))
}

func parseCSV(text string, tsv bool) ([][]string, bool) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	if tsv {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil || len(recs) < 2 || len(recs[0]) < 2 {
		return nil, false
	}
	for i := range recs[0] {
		recs[0][i] = strings.TrimSpace(recs[0][i])
	}
	return recs, true
}

func pdfText(f InFile) (string, error) {
	p := f.Path
	if p == "" {
		tmp, err := os.CreateTemp("", "braindb-*.pdf")
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(f.Data); err != nil {
			tmp.Close()
			return "", err
		}
		tmp.Close()
		p = tmp.Name()
	}
	var out bytes.Buffer
	cmd := exec.Command("pdftotext", "-layout", p, "-")
	cmd.Stdout = &out
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		return "", errf("pdftotext timed out")
	}
}

// IngestOptions controls an ingest.
type IngestOptions struct {
	Focus    bool
	MaxFiles int
}

// IngestPath ingests a file or a directory (ctxc workspace if it holds state.json).
func (b *Brain) IngestPath(p string, opt IngestOptions) (*IngestResult, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, errf("does not exist: %s", p)
	}
	if opt.MaxFiles <= 0 {
		opt.MaxFiles = 200
	}
	paths := []string{}
	if st.IsDir() {
		sj := filepath.Join(p, "state.json")
		if _, err := os.Stat(sj); err == nil {
			paths = append(paths, sj)
		} else {
			var walk func(d string)
			walk = func(d string) {
				es, _ := os.ReadDir(d)
				for _, e := range es {
					if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" || len(paths) >= opt.MaxFiles {
						continue
					}
					f := filepath.Join(d, e.Name())
					if e.IsDir() {
						walk(f)
					} else {
						paths = append(paths, f)
					}
				}
			}
			walk(p)
		}
	} else {
		paths = append(paths, p)
	}
	files := []InFile{}
	pre := []string{}
	for _, f := range paths {
		d, err := os.ReadFile(f)
		if err != nil {
			pre = append(pre, fmt.Sprintf("warn: %s: %s", filepath.Base(f), err.Error()))
			continue
		}
		rel, err := filepath.Rel(p, f)
		if err != nil || rel == "." {
			rel = filepath.Base(f)
		}
		files = append(files, InFile{Name: filepath.Base(f), Data: d, Path: f, Rel: filepath.ToSlash(rel)})
	}
	r, err := b.Ingest(files, opt)
	if r != nil {
		r.Report = append(pre, r.Report...)
	}
	return r, err
}

// Ingest ingests in-memory files atomically.
func (b *Brain) Ingest(files []InFile, opt IngestOptions) (res *IngestResult, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	snap := b.clone()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ingest failed: %v", r)
		}
		if err != nil {
			b.restore(snap)
		}
	}()
	report, fresh := []string{}, []string{}
	for _, f := range files {
		b.ingestFile(f, &report, &fresh)
	}
	links := []AutoLink{}
	if len(fresh) > 0 {
		b.embedAll()
		links = b.autoLink(fresh)
		b.embedAll()
		chunksOf := func(id string) int {
			c := 0
			for _, e := range b.adj[id] {
				if e.Type == EdgePart {
					c++
				}
			}
			return c
		}
		light, big := []string{}, []string{}
		for _, id := range fresh {
			if b.nodes[id].Parent != "" {
				continue
			}
			if chunksOf(id) <= 8 {
				light = append(light, id)
			} else {
				big = append(big, id)
			}
		}
		if len(light) > 6 {
			light = light[:6]
		}
		if opt.Focus {
			b.seeds = uniq(append(append([]string(nil), b.seeds...), light...))
			if len(big) > 0 {
				report = append(report, fmt.Sprintf("info: not lighting large files (%s): brain_search finds their chunks", strings.Join(big, ", ")))
			}
		}
		b.Step++
		b.refresh()
	}
	return &IngestResult{Report: report, Fresh: fresh, Links: links, Status: b.status("")}, nil
}
