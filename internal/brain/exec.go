// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// AutoLink is an edge added automatically from embeddings.
type AutoLink struct {
	A    string  `json:"a"`
	B    string  `json:"b"`
	Cos  float64 `json:"cos"`
	Type string  `json:"type"` // concept | similar
}

// LinkSpec is a link in a remember op: a bare id or {to, w}.
type LinkSpec struct {
	To string
	W  *float64
}

func (l *LinkSpec) UnmarshalJSON(d []byte) error {
	var s string
	if json.Unmarshal(d, &s) == nil {
		l.To = s
		return nil
	}
	var o struct {
		To string   `json:"to"`
		W  *float64 `json:"w"`
	}
	if err := json.Unmarshal(d, &o); err != nil {
		return err
	}
	l.To, l.W = o.To, o.W
	return nil
}

// StrList accepts a string or an array of strings.
type StrList []string

func (s *StrList) UnmarshalJSON(d []byte) error {
	var one string
	if json.Unmarshal(d, &one) == nil {
		if one == "" {
			*s = nil
		} else {
			*s = splitIDs(one)
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(d, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

// Op is one exec operation.
type Op struct {
	Op           string          `json:"op"`
	Kind         string          `json:"kind,omitempty"`
	Text         *string         `json:"text,omitempty"`
	Section      string          `json:"section,omitempty"`
	Concepts     StrList         `json:"concepts,omitempty"`
	Abstractions StrList         `json:"abstractions,omitempty"`
	Links        []LinkSpec      `json:"links,omitempty"`
	Supersedes   string          `json:"supersedes,omitempty"`
	Pin          bool            `json:"pin,omitempty"`
	Focus        json.RawMessage `json:"focus,omitempty"`
	Tmp          string          `json:"tmp,omitempty"`
	Autolink     *bool           `json:"autolink,omitempty"`
	W            *float64        `json:"w,omitempty"`
	Label        string          `json:"label,omitempty"`
	Members      StrList         `json:"members,omitempty"`
	Related      StrList         `json:"related,omitempty"`
	A            string          `json:"a,omitempty"`
	B            string          `json:"b,omitempty"`
	Type         string          `json:"type,omitempty"`
	ID           string          `json:"id,omitempty"`
	Old          string          `json:"old,omitempty"`
	New          string          `json:"new,omitempty"`
	Force        bool            `json:"force,omitempty"`
	IDs          StrList         `json:"ids,omitempty"`
	Add          StrList         `json:"add,omitempty"`
	Remove       StrList         `json:"remove,omitempty"`
	Fields       Fields          `json:"fields,omitempty"`
	// connectors
	CType       string   `json:"ctype,omitempty"`
	Key         string   `json:"key,omitempty"`
	Gain        *float64 `json:"gain,omitempty"`
	Transparent *bool    `json:"transparent,omitempty"`
	Terms       StrList  `json:"terms,omitempty"`
	Connect     StrList  `json:"connect,omitempty"`
	By          StrList  `json:"by,omitempty"`
	Min         int      `json:"min,omitempty"`
	DryRun      bool     `json:"dryRun,omitempty"`
	Labels      StrList  `json:"labels,omitempty"`
	Override    string   `json:"override,omitempty"` // purpose override reason
	Exempt      bool     `json:"-"`                  // the writer is exempt from the purpose filter
	Origin      string   `json:"-"`                  // remember | insert (purpose admission.apply)
}

// PurposeOutcome reports a write the purpose filter quarantined or rejected.
type PurposeOutcome struct {
	Op       int     `json:"op"`
	Tmp      string  `json:"tmp,omitempty"`
	ID       string  `json:"id,omitempty"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
	Decision string  `json:"decision"`
	Reasons  string  `json:"reasons"`
}

// ExecResult is the result of an atomic batch.
type ExecResult struct {
	Changes []string   `json:"changes"`
	Touched []string   `json:"touched"`
	Auto    []AutoLink `json:"auto"`
	Status  string     `json:"status"`
	Created []string   `json:"created,omitempty"` // ids created by remember / concept / connector ops, in op order
	// Purpose filter outcomes (Addendum O).
	Quarantined []PurposeOutcome `json:"quarantined,omitempty"`
	Rejected    []PurposeOutcome `json:"rejected,omitempty"`
}

var edgeTypes = map[string]bool{EdgeConnector: true, EdgeConcept: true, EdgeAssoc: true, EdgeSimilar: true, EdgeSupersede: true, EdgePart: true, EdgeSeq: true, EdgeNew: true, EdgeExternal: true}

func normEdgeType(t string) string {
	if m, ok := legacyEdge[t]; ok {
		return m
	}
	return t
}

// findConcept resolves a concept by id, "A:"+label, exact label or label prefix; creates it when allowed.
func (b *Brain) findConcept(a string, create bool) string {
	if n := b.nodes[a]; n != nil && n.Kind == Concept {
		return a
	}
	if b.nodes["A:"+a] != nil && b.nodes["A:"+a].Kind == Concept {
		return "A:" + a
	}
	norm := func(x string) string { return jsTrim(lowerDeacc(x)) }
	na := norm(a)
	for _, id := range b.concepts() {
		if norm(b.nodes[id].Text) == na {
			return id
		}
	}
	if jsLen(na) >= 4 {
		for _, id := range b.concepts() {
			if strings.HasPrefix(norm(b.nodes[id].Text), na) {
				return id
			}
		}
	}
	if !create {
		return ""
	}
	s := slug(na, "-")
	if len(s) > 24 {
		s = s[:24]
	}
	if s == "" {
		s = "c" + itoa(b.nextID)
		b.nextID++
	}
	id := "A:" + s
	for b.nodes[id] != nil {
		id += "_"
	}
	b.addNode(&Node{ID: id, Kind: Concept, Section: "concepts", Text: a, Base: .8})
	return id
}

// Exec applies an atomic batch of operations. On error nothing changes.
func (b *Brain) Exec(ops []Op) (*ExecResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exec(ops)
}

func (b *Brain) exec(ops []Op) (res *ExecResult, err error) {
	b.touch()
	if len(ops) == 0 {
		return nil, errf("ops: a non-empty list of operations is required")
	}
	snap := b.clone()
	defer func() {
		if err != nil {
			b.restore(snap)
		}
	}()
	tmp := map[string]string{}
	rejectedTmp := map[string]string{}
	var quarantined, rejected []PurposeOutcome
	created := []string{}
	changes := []string{}
	touchedL := []string{}
	touch := func(id string) {
		if !contains(touchedL, id) {
			touchedL = append(touchedL, id)
		}
	}
	fresh := []string{}
	R := func(x string) (string, error) {
		if strings.HasPrefix(x, "@") {
			id, ok := tmp[x[1:]]
			if !ok {
				if why, rej := rejectedTmp[x[1:]]; rej {
					return "", errf("%s is unset: its remember was rejected by the purpose filter (%s)", x, why)
				}
				return "", errf("unknown %s", x)
			}
			return id, nil
		}
		return x, nil
	}
	need := func(x string) (string, error) {
		id, err := R(x)
		if err != nil {
			return "", err
		}
		if b.nodes[id] == nil {
			if b.isExternalID(id) {
				if err := b.ensureExternal(id); err != nil {
					return "", err
				}
				return id, nil
			}
			return "", errf("no such item %s", x)
		}
		return id, nil
	}
	mem := func(x string) (string, error) {
		id, err := need(x)
		if err != nil {
			return "", err
		}
		if b.nodes[id].Kind == Concept {
			return "", errf("%s is a concept", id)
		}
		if b.nodes[id].Ext != nil {
			return "", errf("%s is external (read-only)", id)
		}
		if isConn(b.nodes[id]) {
			return "", errf("%s is a connector", id)
		}
		return id, nil
	}
	needAll := func(xs []string) ([]string, error) {
		out := []string{}
		for _, x := range xs {
			id, err := need(x)
			if err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, nil
	}
	now := b.nowMs()
	for i, o := range ops {
		if err := func() error {
			switch o.Op {
			case "remember":
				k, ok := b.kindM[o.Kind]
				if !ok || o.Kind == Concept {
					names := []string{}
					for _, kk := range b.kinds {
						if kk.Name != Concept {
							names = append(names, kk.Name)
						}
					}
					return errf("kind must be one of: %s", strings.Join(names, ", "))
				}
				if o.Text == nil || *o.Text == "" {
					return errf("missing text")
				}
				id := b.newID(o.Kind)
				sec := o.Section
				if sec == "" {
					sec = k.Section
				}
				base := k.Base
				if o.W != nil {
					base = *o.W
				}
				b.addNode(&Node{ID: id, Kind: o.Kind, Section: sec, Text: *o.Text, Touched: now, Pin: o.Pin, Base: base, Fields: o.Fields,
					Labels: uniqSorted(append(append([]string{}, o.Labels...), b.defaultLabels(sec, o.Kind, "")...))})
				b.addSection(sec)
				if o.Tmp != "" {
					tmp[o.Tmp] = id
				}
				for _, a := range append(append([]string(nil), o.Concepts...), o.Abstractions...) {
					A := b.findConcept(a, true)
					b.addEdge(id, A, .75, EdgeConcept)
					touch(A)
				}
				for _, l := range o.Links {
					to, err := need(l.To)
					if err != nil {
						return err
					}
					w := .6
					if l.W != nil {
						w = *l.W
					}
					typ := EdgeAssoc
					if b.nodes[to].Ext != nil {
						typ = EdgeExternal
					}
					if isConn(b.nodes[to]) {
						typ = EdgeConnector
					}
					b.addEdge(id, to, w, typ)
					touch(to)
				}
				if o.Supersedes != "" {
					old, err := mem(o.Supersedes)
					if err != nil {
						return err
					}
					b.nodes[old].Sup = id
					b.addEdge(old, id, .5, EdgeSupersede)
					changes = append(changes, "⇄"+old)
					touch(old)
				}
				op := o.Origin
				if op == "" {
					op = "remember"
				}
				pc, ok := b.admit(b.nodes[id], op, o.Exempt, o.Override)
				out := PurposeOutcome{Op: i + 1, Tmp: o.Tmp, ID: id, Text: short(*o.Text, 80), Score: pc.Score, Decision: pc.Decision}
				if b.nodes[id].Purpose != nil {
					out.Reasons = b.nodes[id].Purpose.Reasons
				}
				if !ok {
					b.removeNode(id, false)
					if o.Tmp != "" {
						delete(tmp, o.Tmp)
						rejectedTmp[o.Tmp] = fmt.Sprintf("score %.2f", pc.Score)
					}
					out.ID, out.Decision = "", "rejected"
					rejected = append(rejected, out)
					changes = append(changes, fmt.Sprintf("rejected:%q", short(*o.Text, 40)))
					return nil
				}
				if isQuarantined(b.nodes[id]) {
					quarantined = append(quarantined, out)
					changes = append(changes, "quarantine:"+id)
					touch(id)
					return nil
				}
				if f := string(o.Focus); f == "true" {
					b.seeds = append(b.seeds, id)
				}
				changes = append(changes, "+"+id)
				created = append(created, id)
				touch(id)
				if o.Autolink == nil || *o.Autolink {
					fresh = append(fresh, id)
				}
			case "connector":
				typ := o.CType
				if typ == "" {
					typ = o.Type
				}
				h, err := b.addConnector(ConnectorSpec{Type: typ, Label: o.Label, Key: o.Key, Gain: o.Gain, Transparent: o.Transparent, Terms: o.Terms, ID: o.ID})
				if err != nil {
					return err
				}
				if o.Tmp != "" {
					tmp[o.Tmp] = h
				}
				created = append(created, h)
				w := .9
				if o.W != nil {
					w = *o.W
				}
				for _, x := range o.Connect {
					id, err := need(x)
					if err != nil {
						return err
					}
					if isConn(b.nodes[id]) {
						return errf("cannot connect a connector to a connector (%s)", id)
					}
					b.addEdge(id, h, w, EdgeConnector)
					touch(id)
				}
				changes = append(changes, "+⧉"+h)
				touch(h)
			case "connect", "disconnect":
				a, err := need(o.A)
				if err != nil {
					return err
				}
				h, err := need(o.B)
				if err != nil {
					return err
				}
				if !isConn(b.nodes[h]) {
					a, h = h, a
				}
				if !isConn(b.nodes[h]) {
					return errf("%s is not a connector", o.B)
				}
				if o.Op == "connect" {
					w := .9
					if o.W != nil {
						w = *o.W
					}
					b.addEdge(a, h, w, EdgeConnector)
					changes = append(changes, "~"+a+"⧉"+h)
				} else {
					e := b.edgeBetween(a, h)
					if e == nil {
						return errf("%s is not connected to %s", a, h)
					}
					b.removeEdge(e)
					changes = append(changes, "-"+a+"⧉"+h)
				}
				touch(a)
			case "resolve":
				if o.Label == "" || len(o.By) == 0 {
					return errf("resolve needs label (the entity name) and by (fields)")
				}
				r, err := b.resolve(o.Label, o.By, o.Min, o.DryRun)
				if err != nil {
					return err
				}
				for _, h := range r.Created {
					changes = append(changes, "+⧉"+h)
				}
				if o.DryRun {
					changes = append(changes, fmt.Sprintf("resolve dry run: %d anchors", len(r.Anchors)))
				}
			case "label":
				ids := []string{}
				for _, x := range o.IDs {
					id, err := need(x)
					if err != nil {
						return err
					}
					ids = append(ids, id)
				}
				for _, id := range ids {
					n := b.nodes[id]
					set := append([]string{}, n.Labels...)
					set = append(set, o.Add...)
					rm := map[string]bool{}
					for _, l := range o.Remove {
						rm[l] = true
					}
					n.Labels = uniqSorted(filterStr(set, func(l string) bool { return !rm[l] }))
					changes = append(changes, "label:"+id+"="+strings.Join(n.Labels, ","))
					touch(id)
				}
			case "concept":
				if o.Label == "" {
					return errf("missing label")
				}
				A := b.findConcept(o.Label, true)
				created = append(created, A)
				changes = append(changes, "+◆"+A)
				touch(A)
				if o.Tmp != "" {
					tmp[o.Tmp] = A
				}
				w := .7
				if o.W != nil {
					w = *o.W
				}
				for _, m := range o.Members {
					id, err := mem(m)
					if err != nil {
						return err
					}
					b.addEdge(id, A, w, EdgeConcept)
				}
				for _, c := range o.Related {
					b.addEdge(A, b.findConcept(c, true), .6, EdgeConcept)
				}
			case "link":
				a, err := need(o.A)
				if err != nil {
					return err
				}
				c, err := need(o.B)
				if err != nil {
					return err
				}
				typ := normEdgeType(o.Type)
				if typ == "" {
					typ = EdgeAssoc
					if b.nodes[a].Ext != nil || b.nodes[c].Ext != nil {
						typ = EdgeExternal
					}
					if isConn(b.nodes[a]) || isConn(b.nodes[c]) {
						typ = EdgeConnector
					}
				}
				if !edgeTypes[typ] {
					return errf("unknown link type %s", o.Type)
				}
				w := .6
				if o.W != nil {
					w = *o.W
				}
				if b.addEdge(a, c, w, typ) == nil {
					return errf("cannot link %s to itself", a)
				}
				if o.W != nil {
					b.edgeBetween(a, c).W = round2(clamp(*o.W))
				}
				changes = append(changes, "~"+a+"↔"+c)
				touch(a)
				touch(c)
			case "unlink":
				a, err := need(o.A)
				if err != nil {
					return err
				}
				c, err := need(o.B)
				if err != nil {
					return err
				}
				e := b.edgeBetween(a, c)
				if e == nil {
					return errf("no synapse %s↔%s", a, c)
				}
				b.removeEdge(e)
				changes = append(changes, "-"+a+"↔"+c)
			case "boost":
				id, err := mem(o.ID)
				if err != nil {
					return err
				}
				n := b.nodes[id]
				n.Base = round2(clamp(n.Base + .15))
				n.Age, n.Touched = 0, now
				changes = append(changes, "▲"+id)
				touch(id)
			case "weaken":
				id, err := mem(o.ID)
				if err != nil {
					return err
				}
				n := b.nodes[id]
				n.Base = round2(clamp(n.Base - .2))
				changes = append(changes, "▼"+id)
				touch(id)
			case "pin", "unpin":
				id, err := mem(o.ID)
				if err != nil {
					return err
				}
				b.nodes[id].Pin = o.Op == "pin"
				changes = append(changes, o.Op+":"+id)
				touch(id)
			case "supersede":
				old, err := mem(o.Old)
				if err != nil {
					return err
				}
				nw, err := mem(o.New)
				if err != nil {
					return err
				}
				b.nodes[old].Sup = nw
				b.addEdge(old, nw, .5, EdgeSupersede)
				changes = append(changes, "⇄"+old+"→"+nw)
				touch(old)
				touch(nw)
			case "edit":
				id, err := need(o.ID)
				if err != nil {
					return err
				}
				n := b.nodes[id]
				if n.Ext != nil {
					return errf("%s is external (read-only)", id)
				}
				if o.Text != nil {
					n.Text = *o.Text
				}
				if o.Section != "" {
					n.Section = o.Section
					b.addSection(o.Section)
				}
				if o.Kind != "" {
					if _, ok := b.kindM[o.Kind]; !ok {
						return errf("unknown kind %s", o.Kind)
					}
					n.Kind = o.Kind
				}
				for _, f := range o.Fields {
					n.Fields = n.Fields.Set(f.Name, f.Value)
				}
				if o.CType != "" {
					if n.Conn == nil {
						return errf("%s is not a connector", id)
					}
					d, ok := connDefaults[o.CType]
					if !ok {
						return errf("connector type must be anchor, bridge, hub, alias or time")
					}
					n.Conn.Type, n.Conn.Gain, n.Conn.Transparent, n.Base = o.CType, d.Gain, d.Transparent, d.Gain
				}
				if o.Key != "" && n.Conn != nil {
					n.Conn.Key = o.Key
				}
				n.Age, n.Touched = 0, now
				changes = append(changes, "~"+id)
				touch(id)
			case "forget":
				id, err := need(o.ID)
				if err != nil {
					return err
				}
				if b.nodes[id].Pin && !o.Force {
					return errf("%s is pinned (use force: true)", id)
				}
				b.removeNode(id, true)
				changes = append(changes, "-"+id)
			case "focus":
				if o.IDs != nil {
					ids, err := needAll(o.IDs)
					if err != nil {
						return err
					}
					b.seeds = ids
				}
				if o.Add != nil {
					ids, err := needAll(o.Add)
					if err != nil {
						return err
					}
					b.seeds = uniq(append(append([]string(nil), b.seeds...), ids...))
				}
				if o.Remove != nil {
					rm := map[string]bool{}
					for _, x := range o.Remove {
						id, err := R(x)
						if err != nil {
							return err
						}
						rm[id] = true
					}
					b.seeds = filterStr(b.seeds, func(x string) bool { return !rm[x] })
				}
				changes = append(changes, "focus="+strings.Join(b.seeds, ","))
			case "goal":
				if o.Text != nil && *o.Text != "" {
					b.Goal = *o.Text
				}
				if len(o.Focus) > 0 && string(o.Focus) != "null" {
					var f StrList
					if err := json.Unmarshal(o.Focus, &f); err != nil {
						return errf("focus must be a list of ids")
					}
					ids, err := needAll(f)
					if err != nil {
						return err
					}
					b.seeds = ids
					b.goal = append([]string(nil), ids...)
				}
				changes = append(changes, "goal")
			default:
				return errf("unknown operation (remember, concept, link, unlink, boost, weaken, pin, unpin, supersede, edit, forget, focus, goal, connector, connect, disconnect, resolve)")
			}
			return nil
		}(); err != nil {
			return nil, errf("op#%d %s: %s", i+1, o.Op, err.Error())
		}
	}
	b.embedAll()
	auto := b.autoLink(filterStr(fresh, func(id string) bool { return b.nodes[id] != nil && !isQuarantined(b.nodes[id]) }))
	for _, a := range auto {
		touch(a.B)
	}
	b.embedAll()
	b.Step++
	b.refresh()
	return &ExecResult{Changes: changes, Touched: filterStr(touchedL, func(id string) bool { return b.nodes[id] != nil }), Auto: auto, Status: b.status(""), Created: created,
		Quarantined: quarantined, Rejected: rejected}, nil
}

// autoLink links new memories to ≤ 2 concepts (cos > 0.14, only without a concept edge) and ≤ 3 similar memories (cos > 0.28).
func (b *Brain) autoLink(ids []string) []AutoLink {
	out := []AutoLink{}
	abs := append([]string(nil), b.concepts()...)
	mem := filterStr(b.order, func(id string) bool {
		n := b.nodes[id]
		return n.Kind != Concept && n.Sup == "" && n.Ext == nil && n.Conn == nil
	})
	type sc struct {
		id string
		c  float64
	}
	for _, id := range ids {
		n := b.nodes[id]
		v := b.raw[id]
		if v == nil || n.Kind == Concept {
			continue
		}
		hasC := false
		for _, e := range b.adj[id] {
			if e.Type == EdgeConcept {
				hasC = true
			}
		}
		if !hasC {
			cs := []sc{}
			for _, a := range abs {
				cs = append(cs, sc{a, vcos(v, b.raw[a])})
			}
			stableSortDesc(cs, func(x sc) float64 { return x.c })
			for i, x := range cs {
				if i >= 2 {
					break
				}
				if x.c > .14 {
					b.addEdge(id, x.id, clamp(.35+x.c), EdgeConcept)
					out = append(out, AutoLink{id, x.id, x.c, EdgeConcept})
				}
			}
		}
		sibling := func(m string) bool {
			nm := b.nodes[m]
			return m == id || m == n.Parent || nm.Parent == id || (n.Parent != "" && nm.Parent == n.Parent)
		}
		cs := []sc{}
		for _, m := range mem {
			if !sibling(m) && b.edgeBetween(id, m) == nil {
				cs = append(cs, sc{m, vcos(v, b.raw[m])})
			}
		}
		stableSortDesc(cs, func(x sc) float64 { return x.c })
		for i, x := range cs {
			if i >= 3 {
				break
			}
			if x.c > .28 {
				b.addEdge(id, x.id, clamp(x.c), EdgeSimilar)
				out = append(out, AutoLink{id, x.id, x.c, EdgeSimilar})
			}
		}
	}
	return out
}

// SetContext changes focus and activation params (used by the API and brain_view).
type ContextChange struct {
	Focus          *StrList `json:"focus"`
	AddFocus       StrList  `json:"addFocus"`
	RemoveFocus    StrList  `json:"removeFocus"`
	Depth          *int     `json:"depth"`
	Threshold      *float64 `json:"threshold"`
	Hop            *float64 `json:"hop"`
	ShowSuperseded *bool    `json:"showSuperseded"`
	Goal           *string  `json:"goal"`
	GoalFocus      bool     `json:"goalFocus"`
}

// SetContext applies a context change. Returns whether anything changed.
func (b *Brain) SetContext(c ContextChange) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.setContext(c)
}

func (b *Brain) setContext(c ContextChange) (bool, error) {
	check := func(xs []string) error {
		for _, id := range xs {
			if b.nodes[id] == nil {
				return errf("no such item %s", id)
			}
		}
		return nil
	}
	changed := false
	if c.Focus != nil {
		f := []string(*c.Focus)
		if len(f) == 1 && f[0] == "goal" {
			f = append([]string(nil), b.goal...)
		}
		if err := check(f); err != nil {
			return false, err
		}
		b.seeds, changed = uniq(f), true
	}
	if c.AddFocus != nil {
		if err := check(c.AddFocus); err != nil {
			return false, err
		}
		b.seeds, changed = uniq(append(append([]string(nil), b.seeds...), c.AddFocus...)), true
	}
	if c.RemoveFocus != nil {
		rm := map[string]bool{}
		for _, x := range c.RemoveFocus {
			rm[x] = true
		}
		b.seeds, changed = filterStr(b.seeds, func(x string) bool { return !rm[x] }), true
	}
	if c.Depth != nil {
		d := *c.Depth
		if d < 0 {
			d = 0
		}
		if d > 8 {
			d = 8
		}
		b.P.Depth, changed = d, true
	}
	if c.Threshold != nil {
		b.P.Threshold, changed = clampRange(*c.Threshold, .001, 1), true
	}
	if c.Hop != nil {
		b.P.Hop, changed = clampRange(*c.Hop, .05, 1), true
	}
	if c.ShowSuperseded != nil {
		b.P.ShowSuperseded, changed = *c.ShowSuperseded, true
	}
	if c.Goal != nil {
		b.Goal, changed = *c.Goal, true
	}
	if c.GoalFocus {
		b.goal, changed = append([]string(nil), b.seeds...), true
	}
	b.refresh()
	return changed, nil
}

// EmbeddingChange updates the embedding settings.
type EmbeddingChange struct {
	Alpha  *float64 `json:"alpha"`
	Hops   *int     `json:"hops"`
	Use    string   `json:"use"`
	Lambda *float64 `json:"lambda"`
}

// SetEmbedding applies embedding settings and re-enriches vectors.
func (b *Brain) SetEmbedding(c EmbeddingChange) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c.Alpha != nil {
		b.Emb.Alpha = clampRange(*c.Alpha, 0, 1)
	}
	if c.Hops != nil {
		h := *c.Hops
		if h < 0 {
			h = 0
		}
		if h > 6 {
			h = 6
		}
		b.Emb.Hops = h
	}
	switch c.Use {
	case "":
	case "raw", "enriched":
		b.Emb.Use = c.Use
	case "enr":
		b.Emb.Use = "enriched"
	default:
		return errf("use must be raw or enriched")
	}
	if c.Lambda != nil {
		b.Emb.Lambda = clampRange(*c.Lambda, 0, 1)
	}
	b.enrich()
	b.refresh()
	return nil
}

// NewKind is the input of AddKind.
type NewKind struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Color    string   `json:"color"`
	HalfLife float64  `json:"halfLife"`
	Floor    *float64 `json:"floor"`
	Base     *float64 `json:"base"`
	Prefix   string   `json:"prefix"`
	Section  string   `json:"section"`
}

// AddKind registers a custom kind (key is derived from the name like the reference addKind).
func (b *Brain) AddKind(k NewKind) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := slug(k.Name, "_")
	if key == "" {
		return "", errf("missing kind name")
	}
	if _, ok := b.kindM[key]; ok {
		return key, nil
	}
	hl := k.HalfLife
	if hl <= 0 {
		hl = 14
	}
	base := .5
	if k.Base != nil {
		base = clamp(*k.Base)
	}
	label := k.Label
	if label == "" {
		label = k.Name
	}
	floor := 0.0
	if k.Floor != nil {
		floor = clamp01(*k.Floor)
	}
	b.addKind(Kind{Name: key, Label: label, Color: k.Color, HalfLife: fp(hl), Floor: floor, Base: base, Prefix: k.Prefix, Section: k.Section})
	b.addSection(b.kindM[key].Section)
	return key, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Snapshot captures the brain state for a later Restore (script transactions).
func (b *Brain) Snapshot() *Brain {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.clone()
}

// Restore rolls the brain back to a snapshot.
func (b *Brain) Restore(s *Brain) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restore(s)
	b.embedAll()
	b.refresh()
}

// IDs returns all node ids in order.
func (b *Brain) IDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.order...)
}

// KindNames returns kind names (without concept/connector).
func (b *Brain) KindNames() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []string{}
	for _, k := range b.kinds {
		if k.Name != Concept && k.Name != ConnectorKind {
			out = append(out, k.Name)
		}
	}
	return out
}

// KindDecay is one entry of SetKindDecay.
type KindDecay struct {
	Kind     string
	HalfLife float64 // days; ≤ 0 or +Inf means "never decays"
	Floor    float64 // fraction of the base weight decay can never remove (0..1)
}

// SetKindDecay overrides the decay curve of existing kinds. It is the supported way to opt a brain out of the
// durable-knowledge defaults (for example to reproduce the pre-0.0.2 behaviour, where every kind decayed to zero).
func (b *Brain) SetKindDecay(ds ...KindDecay) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, d := range ds {
		k, ok := b.kindM[d.Kind]
		if !ok {
			return errf("no such kind: %s", d.Kind)
		}
		if d.HalfLife <= 0 || math.IsInf(d.HalfLife, 1) {
			k.HalfLife = nil
		} else {
			k.HalfLife = fp(d.HalfLife)
		}
		k.Floor = clamp01(d.Floor)
	}
	b.refresh()
	return nil
}

// LegacyDecay is the pre-0.0.2 kind decay policy: every kind fades to zero, with no floor. It is kept so the parity
// suite can pin the reference implementation's numbers while the product defaults treat knowledge as durable.
func LegacyDecay() []KindDecay {
	return []KindDecay{
		{"fact", 30, 0}, {"decision", 60, 0}, {"task", 14, 0}, {"question", 7, 0}, {"note", 2, 0},
		{"summary", 30, 0}, {"file", 14, 0}, {"idea", 7, 0}, {"chunk", 21, 0}, {"record", 30, 0},
	}
}
