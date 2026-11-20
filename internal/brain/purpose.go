// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// Purpose and the admission pre-filter (deterministic: embeddings + rules, explainable).

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Purpose is the purpose block of braindb.yaml.
type Purpose struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	InScope     struct {
		Topics   []string `json:"topics,omitempty"`
		Concepts []string `json:"concepts,omitempty"`
		Kinds    []string `json:"kinds,omitempty"`
		Sections []string `json:"sections,omitempty"`
	} `json:"inScope"`
	OutOfScope struct {
		Topics   []string `json:"topics,omitempty"`
		Entities []string `json:"entities,omitempty"`
	} `json:"outOfScope"`
	Examples struct {
		Accept []string `json:"accept,omitempty"`
		Reject []string `json:"reject,omitempty"`
	} `json:"examples"`
	Admission struct {
		Mode    string   `json:"mode,omitempty"` // off | advise | enforce
		Accept  *float64 `json:"accept,omitempty"`
		Review  *float64 `json:"review,omitempty"`
		Apply   []string `json:"apply,omitempty"`
		Exempt  []string `json:"exempt,omitempty"`
		Chunks  string   `json:"chunks,omitempty"`  // file | each
		Records string   `json:"records,omitempty"` // table | each
	} `json:"admission"`
}

// PurposeMark records the admission outcome on a node.
type PurposeMark struct {
	Score    float64
	Decision string // admitted | quarantine | rejected
	Flag     string // low | override | admitted
	Section  string // intended section while quarantined
	Reasons  string
}

// PurposeCheck is an explainable admission score.
type PurposeCheck struct {
	Score       float64  `json:"score"`
	Decision    string   `json:"decision"`
	Pos         float64  `json:"pos"`
	Neg         float64  `json:"neg"`
	Bonus       float64  `json:"bonus"`
	TopMatch    string   `json:"topMatch,omitempty"`
	TopConflict string   `json:"topConflict,omitempty"`
	Rules       []string `json:"rules"`
	Mode        string   `json:"mode"`
}

// QuarantineLabel hides quarantined items from everyone without the review grant.
const QuarantineLabel = "quarantine"

func isQuarantined(n *Node) bool {
	if n == nil {
		return false
	}
	for _, l := range n.Labels {
		if l == QuarantineLabel {
			return true
		}
	}
	return false
}

type purposeVecs struct {
	sig       string
	centroid  []float32
	topics    [][]float32
	conflicts [][]float32
	topicTxt  []string
	confTxt   []string
}

func (p *Purpose) mode() string {
	if p == nil {
		return "off"
	}
	// Default to advise, never enforce. Silently dropping knowledge because a similarity score fell below a
	// threshold is the worst failure this database can have: the loss is invisible and unrecoverable. Enforcement
	// is opt-in, and a brain should earn it by first running in advise mode and having its quarantine reviewed.
	if p.Admission.Mode == "" {
		return "advise"
	}
	return p.Admission.Mode
}

func (p *Purpose) thresholds() (float64, float64) {
	a, r := .35, .20
	if p.Admission.Accept != nil {
		a = *p.Admission.Accept
	}
	if p.Admission.Review != nil {
		r = *p.Admission.Review
	}
	return a, r
}

func (p *Purpose) applies(op string) bool {
	if len(p.Admission.Apply) == 0 {
		return true
	}
	return contains(p.Admission.Apply, op)
}

func (b *Brain) purposeVectors() *purposeVecs {
	p := b.purpose
	sig := fmt.Sprintf("%p|%s|%v|%v|%v|%v", p, p.Description, p.InScope.Topics, p.OutOfScope.Topics, p.Examples, b.Emb)
	if b.pvecs != nil && b.pvecs.sig == sig {
		return b.pvecs
	}
	v := &purposeVecs{sig: sig}
	pos := append(append([]string{p.Description}, p.InScope.Topics...), p.Examples.Accept...)
	c := make([]float32, b.emb().Dim())
	for _, t := range pos {
		if strings.TrimSpace(t) == "" {
			continue
		}
		e := b.embed(t)
		for i := range c {
			c[i] += e[i]
		}
	}
	v.centroid = vnorm(c)
	for _, t := range p.InScope.Topics {
		v.topics = append(v.topics, b.embed(t))
		v.topicTxt = append(v.topicTxt, t)
	}
	for _, t := range append(append([]string{}, p.OutOfScope.Topics...), p.Examples.Reject...) {
		v.conflicts = append(v.conflicts, b.embed(t))
		v.confTxt = append(v.confTxt, t)
	}
	b.pvecs = v
	return v
}

// checkPurpose scores an item against the purpose (O2).
func (b *Brain) checkPurpose(text, kind, section string, links int, fields Fields) PurposeCheck {
	p := b.purpose
	res := PurposeCheck{Decision: "admitted", Rules: []string{}, Mode: p.mode()}
	if p == nil || p.mode() == "off" {
		res.Score = 1
		res.Rules = append(res.Rules, "no purpose: filter off")
		return res
	}
	full := text
	for _, f := range fields {
		full += " " + f.Value
	}
	if strings.TrimSpace(full) == "" {
		res.Decision = "rejected"
		res.Rules = append(res.Rules, "empty text")
		return res
	}
	// An empty out-of-scope list forbids nothing. ExtractTypes treats nil as "every type",
	// so without this guard any item carrying a date, an amount or a version is rejected.
	if len(p.OutOfScope.Entities) > 0 {
		for _, e := range b.patOpts().ExtractTypes(full, p.OutOfScope.Entities) {
			res.Decision = "rejected"
			res.Rules = append(res.Rules, "forbidden entity "+e.Type)
			return res
		}
	}
	pv := b.purposeVectors()
	tv := b.embed(full)
	res.Pos = vcos(tv, pv.centroid)
	res.TopMatch = "description"
	for i, v := range pv.topics {
		if c := vcos(tv, v); c > res.Pos {
			res.Pos, res.TopMatch = c, pv.topicTxt[i]
		}
	}
	for _, c := range p.InScope.Concepts {
		if id := b.findConcept(c, false); id != "" {
			if x := vcos(tv, b.raw[id]); x > res.Pos {
				res.Pos, res.TopMatch = x, "concept "+b.nodes[id].Text
			}
		}
	}
	for i, v := range pv.conflicts {
		if c := vcos(tv, v); c > res.Neg {
			res.Neg, res.TopConflict = c, pv.confTxt[i]
		}
	}
	base := clamp(res.Pos - .6*math.Max(0, res.Neg-res.Pos*.8))
	if contains(p.InScope.Kinds, kind) {
		res.Bonus += .10
		res.Rules = append(res.Rules, "kind "+kind+" in scope (+0.10)")
	}
	if contains(p.InScope.Sections, section) {
		res.Bonus += .05
		res.Rules = append(res.Rules, "section "+section+" in scope (+0.05)")
	}
	if links > 0 {
		lb := math.Min(.15, .05*float64(links))
		res.Bonus += lb
		res.Rules = append(res.Rules, fmt.Sprintf("%d links to in-scope knowledge (+%.2f)", links, lb))
	}
	res.Score = roundN(clamp(base+res.Bonus), 4)
	res.Pos, res.Neg, res.Bonus = roundN(res.Pos, 4), roundN(res.Neg, 4), roundN(res.Bonus, 4)
	acc, rev := p.thresholds()
	switch {
	case res.Score >= acc:
		res.Decision = "admitted"
	case res.Score >= rev:
		res.Decision = "quarantine"
	default:
		res.Decision = "rejected"
	}
	if res.TopConflict != "" && res.Neg > res.Pos*.8 {
		res.Rules = append(res.Rules, fmt.Sprintf("conflicts with %q (%.2f)", res.TopConflict, res.Neg))
	}
	return res
}

// CheckPurpose scores text against the purpose without storing anything.
func (b *Brain) CheckPurpose(text, kind, section string, fields Fields) PurposeCheck {
	return b.CheckPurposeWith(text, kind, section, fields, nil, 0)
}

// CheckPurposeWith scores an item that does not exist yet, as REMEMBER would once it is stored and
// linked. linkTo names the nodes it would link to and extraLinks adds links the caller knows about
// but cannot name. Without them the preview sees no links, and a real write of the same text can
// score higher: links to in-scope knowledge are worth up to +0.15, which is the difference between
// the quarantine band and admission.
func (b *Brain) CheckPurposeWith(text, kind, section string, fields Fields, linkTo []string, extraLinks int) PurposeCheck {
	b.mu.Lock()
	defer b.mu.Unlock()
	links := extraLinks
	for _, id := range linkTo {
		v := b.nodes[id]
		if v == nil {
			if c := b.findConcept(id, false); c != "" {
				v = b.nodes[c]
			}
		}
		if v == nil || isQuarantined(v) {
			continue
		}
		if v.Kind == Concept {
			if b.purpose != nil {
				for _, c := range b.purpose.InScope.Concepts {
					if b.findConcept(c, false) == v.ID {
						links++
						break
					}
				}
			}
			continue
		}
		links++
	}
	return b.checkPurpose(text, kind, section, links, fields)
}

// Purpose returns the configured purpose (nil = none).
func (b *Brain) Purpose() *Purpose {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.purpose
}

// SetPurpose replaces the purpose.
func (b *Brain) SetPurpose(p *Purpose) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.purpose = p
	b.pvecs = nil
}

// inScopeLinks counts links/concepts of a new node to admitted in-scope knowledge.
func (b *Brain) inScopeLinks(id string) int {
	n := 0
	for _, e := range b.adj[id] {
		v := b.nodes[other(e, id)]
		if v == nil || isQuarantined(v) {
			continue
		}
		if v.Kind == Concept {
			if b.purpose != nil {
				for _, c := range b.purpose.InScope.Concepts {
					if b.findConcept(c, false) == v.ID {
						n++
					}
				}
			}
			continue
		}
		n++
	}
	return n
}

// admit decides a new node's fate in place: quarantine moves it to section review with the quarantine label;
// rejected returns false (the caller removes it). exempt/override admit regardless (still scored).
func (b *Brain) admit(n *Node, op string, exempt bool, override string) (PurposeCheck, bool) {
	p := b.purpose
	if p == nil || p.mode() == "off" || !p.applies(op) {
		return PurposeCheck{Decision: "admitted", Score: 1}, true
	}
	c := b.checkPurpose(n.Text, n.Kind, n.Section, b.inScopeLinks(n.ID), n.Fields)
	mark := &PurposeMark{Score: c.Score, Decision: c.Decision, Reasons: strings.Join(append([]string{fmt.Sprintf("pos %.2f (%s)", c.Pos, c.TopMatch)}, c.Rules...), "; ")}
	n.Purpose = mark
	forbidden := false
	for _, r := range c.Rules {
		if strings.HasPrefix(r, "forbidden entity") || r == "empty text" {
			forbidden = true
		}
	}
	if override != "" && !forbidden {
		mark.Decision, mark.Flag = "admitted", "override"
		mark.Reasons += "; override: " + override
		c.Decision = "admitted"
		return c, true
	}
	if exempt && !forbidden {
		if c.Decision != "admitted" {
			mark.Flag = "low"
		}
		mark.Decision = "admitted"
		return c, true
	}
	switch p.mode() {
	case "advise":
		if c.Decision != "admitted" {
			mark.Flag = "low"
		}
		mark.Decision = "admitted"
		if forbidden {
			mark.Decision = "rejected"
			return c, false
		}
		return c, true
	}
	switch c.Decision {
	case "quarantine":
		mark.Section = n.Section
		n.Section = "review"
		n.Labels = uniqSorted(append(n.Labels, QuarantineLabel))
		return c, true
	case "rejected":
		return c, false
	}
	return c, true
}

// Quarantined lists quarantined node ids.
func (b *Brain) Quarantined() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return filterStr(b.order, func(id string) bool { return isQuarantined(b.nodes[id]) })
}

// QuarantineItem is one entry of the review queue, as the API and the UI exchange it.
type QuarantineItem struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Text            string   `json:"text"`
	Score           float64  `json:"score"`
	Reasons         []string `json:"reasons"`
	Section         string   `json:"section"`                   // where it sits now: always "review"
	IntendedSection string   `json:"intendedSection,omitempty"` // where admitting would put it
	TS              int64    `json:"ts,omitempty"`              // unix ms, when it was quarantined
}

// QuarantineList returns the review queue, worst score first.
func (b *Brain) QuarantineList() []QuarantineItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []QuarantineItem{}
	for _, id := range b.order {
		n := b.nodes[id]
		if !isQuarantined(n) {
			continue
		}
		it := QuarantineItem{ID: n.ID, Kind: n.Kind, Text: b.maskText(n.Text), Section: n.Section, TS: b.touchOf(n), Reasons: []string{}}
		if n.Purpose != nil {
			it.Score = roundN(n.Purpose.Score, 4)
			it.IntendedSection = n.Purpose.Section
			for _, r := range strings.Split(n.Purpose.Reasons, "; ") {
				if r = strings.TrimSpace(r); r != "" {
					it.Reasons = append(it.Reasons, r)
				}
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score < out[j].Score })
	return out
}

// Admit releases quarantined items into their intended (or the given) section.
func (b *Brain) Admit(ids []string, section string) ([]string, error) {
	b.touch()
	b.mu.Lock()
	defer b.mu.Unlock()
	done := []string{}
	for _, id := range ids {
		n := b.nodes[id]
		if n == nil || !isQuarantined(n) {
			return nil, errf("%s is not in quarantine", id)
		}
		n.Labels = filterStr(n.Labels, func(l string) bool { return l != QuarantineLabel })
		sec := section
		if sec == "" && n.Purpose != nil {
			sec = n.Purpose.Section
		}
		if sec == "" {
			sec = b.kind(n.Kind).Section
		}
		n.Section = sec
		b.addSection(sec)
		if n.Purpose == nil {
			n.Purpose = &PurposeMark{}
		}
		n.Purpose.Decision, n.Purpose.Flag = "admitted", "admitted"
		done = append(done, id)
	}
	b.embedAll()
	b.autoLink(done)
	b.embedAll()
	b.Step++
	b.refresh()
	return done, nil
}

// RejectQuarantined deletes quarantined items.
func (b *Brain) RejectQuarantined(ids []string) error {
	b.touch()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range ids {
		if !isQuarantined(b.nodes[id]) {
			return errf("%s is not in quarantine", id)
		}
	}
	for _, id := range ids {
		b.removeNode(id, true)
	}
	b.Step++
	b.embedAll()
	b.refresh()
	return nil
}

// PurposeCounts counts admitted / quarantined / low / override items.
func (b *Brain) PurposeCounts() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]int{"quarantined": 0, "low": 0, "override": 0}
	for _, id := range b.order {
		n := b.nodes[id]
		if isQuarantined(n) {
			out["quarantined"]++
		}
		if n.Purpose != nil && n.Purpose.Flag != "" {
			out[n.Purpose.Flag]++
		}
	}
	return out
}

// gateText decides an item by text alone (used for files, tables and import groups).
func (b *Brain) gateText(text, kind, section, op string, fields Fields) (PurposeCheck, bool) {
	p := b.purpose
	if p == nil || p.mode() == "off" || !p.applies(op) {
		return PurposeCheck{Decision: "admitted", Score: 1, Mode: p.mode()}, false
	}
	return b.checkPurpose(text, kind, section, 0, fields), true
}

// applyDecision applies a precomputed decision to a new node: false means the node must not be stored.
func (b *Brain) applyDecision(n *Node, c PurposeCheck) bool {
	p := b.purpose
	if p == nil || p.mode() == "off" {
		return true
	}
	mark := &PurposeMark{Score: c.Score, Decision: c.Decision, Reasons: strings.Join(append([]string{fmt.Sprintf("pos %.2f (%s)", c.Pos, c.TopMatch)}, c.Rules...), "; ")}
	n.Purpose = mark
	if p.mode() == "advise" {
		if c.Decision != "admitted" {
			mark.Flag = "low"
		}
		mark.Decision = "admitted"
		return true
	}
	switch c.Decision {
	case "rejected":
		return false
	case "quarantine":
		mark.Section = n.Section
		n.Section = "review"
		n.Labels = uniqSorted(append(n.Labels, QuarantineLabel))
	}
	return true
}
