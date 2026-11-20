// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Connector types and their defaults (gain, transparent).
var connDefaults = map[string]struct {
	Gain        float64
	Transparent bool
}{
	"anchor": {.95, true}, "bridge": {.85, true}, "hub": {.8, false}, "alias": {.9, true}, "time": {.7, false},
}

// ConnectorJSON is a connector in State.
type ConnectorJSON struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Key         string   `json:"key,omitempty"`
	Gain        float64  `json:"gain"`
	Transparent bool     `json:"transparent"`
	Terms       []string `json:"terms,omitempty"`
	Source      string   `json:"source,omitempty"`
	Degree      int      `json:"degree"`
}

// ConnectorSpec creates a connector.
type ConnectorSpec struct {
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Key         string   `json:"key,omitempty"`
	Gain        *float64 `json:"gain,omitempty"`
	Transparent *bool    `json:"transparent,omitempty"`
	Terms       []string `json:"terms,omitempty"`
	Source      string   `json:"source,omitempty"`
	ID          string   `json:"id,omitempty"`
}

func (b *Brain) connectorJSON(n *Node) ConnectorJSON {
	return ConnectorJSON{ID: n.ID, Type: n.Conn.Type, Label: n.Text, Key: n.Conn.Key, Gain: n.Conn.Gain, Transparent: n.Conn.Transparent,
		Terms: n.Conn.Terms, Source: n.Conn.Source, Degree: len(b.adj[n.ID])}
}

func (b *Brain) connectors() []string {
	return filterStr(b.order, func(id string) bool { return isConn(b.nodes[id]) })
}

// connectorByKey finds a connector with the given type and key.
func (b *Brain) connectorByKey(typ, key string) string {
	if key == "" {
		return ""
	}
	for _, id := range b.connectors() {
		if c := b.nodes[id].Conn; c.Key == key && (typ == "" || c.Type == typ) {
			return id
		}
	}
	return ""
}

// addConnector creates (or returns the existing, by key) connector node.
func (b *Brain) addConnector(s ConnectorSpec) (string, error) {
	d, ok := connDefaults[s.Type]
	if !ok {
		return "", errf("connector type must be anchor, bridge, hub, alias or time")
	}
	if s.Label == "" {
		s.Label = s.Key
	}
	if s.Label == "" {
		return "", errf("connector needs a label")
	}
	if id := b.connectorByKey(s.Type, s.Key); id != "" {
		return id, nil
	}
	id := s.ID
	if id == "" {
		base := slug(s.Label, "-")
		if s.Key != "" {
			base = slug(s.Key, "-")
		}
		if len(base) > 32 {
			base = base[:32]
		}
		if base == "" {
			base = "c" + itoa(b.nextID)
			b.nextID++
		}
		id = "H:" + base
		for b.nodes[id] != nil {
			id += "_"
		}
	} else if !strings.HasPrefix(id, "H:") {
		id = "H:" + id
	}
	if b.nodes[id] != nil {
		if isConn(b.nodes[id]) {
			return id, nil
		}
		return "", errf("%s exists and is not a connector", id)
	}
	c := &ConnInfo{Type: s.Type, Key: s.Key, Gain: d.Gain, Transparent: d.Transparent, Terms: s.Terms, Source: s.Source}
	if s.Gain != nil {
		c.Gain = clamp(*s.Gain)
	}
	if s.Transparent != nil {
		c.Transparent = *s.Transparent
	}
	b.addNode(&Node{ID: id, Kind: ConnectorKind, Section: "connectors", Text: s.Label, Base: c.Gain, Conn: c})
	return id, nil
}

func connFromXML(e *XMLEl) *Node {
	d := connDefaults[e.A("type")]
	c := &ConnInfo{Type: e.A("type"), Key: e.A("key"), Gain: atof(e.A("gain"), d.Gain), Transparent: e.A("transparent") == "1", Source: e.A("source")}
	for _, t := range strings.Split(e.A("terms"), "|") {
		if t = strings.TrimSpace(t); t != "" {
			c.Terms = append(c.Terms, t)
		}
	}
	return &Node{ID: e.A("id"), Kind: ConnectorKind, Section: "connectors", Text: e.A("label"), Base: c.Gain, Conn: c}
}

func (b *Brain) connectorsXML() string {
	ids := b.connectors()
	if len(ids) == 0 {
		return ""
	}
	var x strings.Builder
	x.WriteString("<connectors>\n")
	for _, id := range ids {
		n := b.nodes[id]
		c := n.Conn
		t := "0"
		if c.Transparent {
			t = "1"
		}
		x.WriteString(`  <connector id="` + esc(id) + `" type="` + esc(c.Type) + `" label="` + esc(n.Text) + `"` + xa("key", c.Key) +
			` gain="` + fmtNum(c.Gain) + `" transparent="` + t + `"` + xa("terms", strings.Join(c.Terms, "|")) + xa("source", c.Source) + xa("labels", strings.Join(n.Labels, ",")) + "/>\n")
	}
	x.WriteString("</connectors>\n")
	return x.String()
}

// expandQuery appends alias connector terms when the query mentions an alias label or term.
func (b *Brain) expandQuery(q string) string {
	lq := " " + strings.Join(asciiWords(q), " ") + " "
	add := []string{}
	for _, id := range b.connectors() {
		n := b.nodes[id]
		if n.Conn.Type != "alias" {
			continue
		}
		all := append([]string{n.Text}, n.Conn.Terms...)
		hit := false
		for _, t := range all {
			w := strings.Join(asciiWords(t), " ")
			if w != "" && strings.Contains(lq, " "+w+" ") {
				hit = true
				break
			}
		}
		if hit {
			for _, t := range all {
				if w := strings.Join(asciiWords(t), " "); w != "" && !strings.Contains(lq, " "+w+" ") {
					add = append(add, t)
				}
			}
		}
	}
	if len(add) == 0 {
		return q
	}
	return q + " " + strings.Join(add, " ")
}

// routesUsed counts how many lit items were reached through each connector.
func (b *Brain) routesUsed(L lit) ([]string, map[string]int) {
	used := map[string]int{}
	order := []string{}
	for _, id := range L.r.order {
		if isConn(b.nodes[id]) {
			continue
		}
		for _, h := range L.r.route[id] {
			if used[h] == 0 {
				order = append(order, h)
			}
			used[h]++
		}
	}
	return order, used
}

// ConnectorSuggestion proposes a connector with evidence.
type ConnectorSuggestion struct {
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Key      string   `json:"key,omitempty"`
	Members  []string `json:"members"`
	Evidence string   `json:"evidence"`
	Score    float64  `json:"score"`
}

// suggestConnectors proposes anchors (shared field values across ≥ 2 places) and hubs (clique-like clusters).
func (b *Brain) suggestConnectors(limit int) []ConnectorSuggestion {
	out := []ConnectorSuggestion{}
	type occ struct {
		ids    []string
		places map[string]bool
		field  string
	}
	vals := map[string]*occ{}
	keys := []string{}
	for _, id := range b.memories() {
		n := b.nodes[id]
		place := n.Section
		if n.Parent != "" {
			place = n.Parent
		}
		for _, f := range n.Fields {
			v := strings.TrimSpace(f.Value)
			if len(v) < 2 || len(v) > 80 {
				continue
			}
			if _, isNum := parseNum(v); isNum && len(v) < 3 {
				continue
			}
			k := lower(v)
			o := vals[k]
			if o == nil {
				o = &occ{places: map[string]bool{}, field: f.Name}
				vals[k] = o
				keys = append(keys, k)
			}
			if !contains(o.ids, id) {
				o.ids = append(o.ids, id)
			}
			o.places[place] = true
		}
	}
	for _, k := range keys {
		o := vals[k]
		if len(o.places) < 2 || len(o.ids) < 2 || len(o.ids) > 50 {
			continue
		}
		if b.connectorByKey("anchor", o.field+":"+k) != "" {
			continue
		}
		v, _ := b.nodes[o.ids[0]].Fields.Get(o.field)
		out = append(out, ConnectorSuggestion{Type: "anchor", Label: v, Key: o.field + ":" + k, Members: o.ids,
			Evidence: fmt.Sprintf("value %q shared by %d items in %d places", v, len(o.ids), len(o.places)), Score: float64(len(o.places)) + .1*float64(len(o.ids))})
	}
	// hubs: nodes whose neighbourhood is densely interlinked
	for _, id := range b.memories() {
		nb := []string{}
		for _, e := range b.adj[id] {
			if v := other(e, id); b.nodes[v].Kind != Concept && !isConn(b.nodes[v]) {
				nb = append(nb, v)
			}
		}
		if len(nb) < 4 {
			continue
		}
		links := 0
		for i := range nb {
			for j := i + 1; j < len(nb); j++ {
				if b.edgeBetween(nb[i], nb[j]) != nil {
					links++
				}
			}
		}
		dens := float64(links) / float64(len(nb)*(len(nb)-1)/2)
		if dens >= .5 {
			out = append(out, ConnectorSuggestion{Type: "hub", Label: short(b.nodes[id].Text, 40), Members: append([]string{id}, nb...),
				Evidence: fmt.Sprintf("%d items with %.0f%% pairwise links", len(nb)+1, dens*100), Score: dens * float64(len(nb))})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ResolveResult reports anchors created (or proposed in a dry run) by Resolve.
type ResolveResult struct {
	Anchors []ConnectorSuggestion `json:"anchors"`
	Created []string              `json:"created"`
}

// resolve builds anchors from shared values of the given fields (e.g. fields.customer_id, fields.account).
func (b *Brain) resolve(name string, fields []string, min int, dry bool) (*ResolveResult, error) {
	if min < 2 {
		min = 2
	}
	res := &ResolveResult{Anchors: []ConnectorSuggestion{}, Created: []string{}}
	groups := map[string][]string{}
	order := []string{}
	for _, id := range b.order {
		n := b.nodes[id]
		if n.Kind == Concept || isConn(n) {
			continue
		}
		for _, f := range fields {
			v, ok := n.Fields.Get(strings.TrimPrefix(f, "fields."))
			if v = strings.TrimSpace(v); !ok || v == "" {
				continue
			}
			k := lower(v)
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			if !contains(groups[k], id) {
				groups[k] = append(groups[k], id)
			}
		}
	}
	for _, k := range order {
		ids := groups[k]
		if len(ids) < min {
			continue
		}
		key := name + ":" + k
		label := name + " " + k
		for _, id := range ids {
			for _, fld := range []string{"name", "title", "account", "company"} {
				if v, ok := b.nodes[id].Fields.Get(fld); ok && v != "" {
					label = v
				}
			}
		}
		s := ConnectorSuggestion{Type: "anchor", Label: label, Key: key, Members: ids, Evidence: fmt.Sprintf("%d items share %s = %s", len(ids), name, k), Score: float64(len(ids))}
		res.Anchors = append(res.Anchors, s)
		if dry {
			continue
		}
		h, err := b.addConnector(ConnectorSpec{Type: "anchor", Label: label, Key: key})
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			b.addEdge(id, h, .9, EdgeConnector)
		}
		res.Created = append(res.Created, h)
	}
	return res, nil
}

func (b *Brain) connStats() (int, int) {
	edges := 0
	for _, e := range b.edges {
		if e.Type == EdgeConnector || isConn(b.nodes[e.A]) || isConn(b.nodes[e.B]) {
			edges++
		}
	}
	return len(b.connectors()), edges
}

var _ = strconv.Itoa
