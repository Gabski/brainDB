// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// lit is a lit slice: seeds + params + activation result.
type lit struct {
	seeds []string
	P     Params
	r     *actResult
	warn  []string
}

func (b *Brain) curLit() lit { return lit{seeds: b.seeds, P: b.P, r: b.cur} }

func (b *Brain) stamp() string { return b.Now().Format("2006-01-02 15:04") }

func capFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// sectionOrder: declared sections, then any others used by nodes (never "concepts").
func (b *Brain) sectionOrder() []string {
	out := append([]string(nil), b.secs...)
	for _, id := range b.order {
		if s := b.nodes[id].Section; !contains(out, s) {
			out = append(out, s)
		}
	}
	return filterStr(out, func(s string) bool {
		return s != "concepts" && s != "connectors" && s != "external" || s == "external" && b.hasExternal()
	})
}

func (b *Brain) litConcepts(L lit) []string {
	out := filterStr(b.concepts(), L.r.has)
	stableSortDesc(out, func(id string) float64 { return L.r.act[id] })
	return out
}

func (b *Brain) litItems(L lit, sec string, skip func(string) bool) []*Node {
	items := []*Node{}
	for _, id := range L.r.order {
		n := b.nodes[id]
		if n.Section == sec && n.Kind != Concept && n.Conn == nil && (skip == nil || !skip(id)) {
			items = append(items, n)
		}
	}
	stableSortDesc(items, func(n *Node) float64 { return L.r.act[n.ID] })
	return items
}

func (b *Brain) itemXML(n *Node, L lit, withLinks bool) string {
	var x strings.Builder
	x.WriteString("<item" + xa("id", n.ID) + xa("kind", n.Kind))
	if n.Pin {
		x.WriteString(` pinned="1"`)
	}
	x.WriteString(xa("w", f2(b.nodeW(n))))
	if a, ok := L.r.act[n.ID]; ok {
		x.WriteString(xa("act", f2(a)) + xa("depth", strconv.Itoa(L.r.depth[n.ID])))
	}
	if p := L.r.parent[n.ID]; p != "" {
		x.WriteString(xa("via", p))
	}
	if rt := L.r.route[n.ID]; len(rt) > 0 {
		x.WriteString(xa("route", strings.Join(rt, ",")))
	}
	x.WriteString(xa("superseded-by", n.Sup) + xa("age", fmtNum(n.Age)+"d"))
	if n.Ext != nil {
		x.WriteString(xa("source", n.Ext.Source))
	}
	x.WriteString(">" + esc(n.Text))
	if withLinks {
		for _, e := range b.adj[n.ID] {
			x.WriteString("\n  <link to=\"" + esc(other(e, n.ID)) + `" w="` + f2(e.W) + `" type="` + e.Type + `"/>`)
		}
		if len(b.adj[n.ID]) > 0 {
			x.WriteString("\n")
		}
	}
	x.WriteString("</item>")
	return x.String()
}

func (b *Brain) refsOf(id string, L lit) []string {
	out := []string{}
	for _, e := range b.adj[id] {
		v := other(e, id)
		if L.r.has(v) && b.nodes[v].Kind != Concept {
			out = append(out, v)
		}
	}
	return out
}

func (b *Brain) conceptTags(id string) []string {
	out := []string{}
	for _, e := range b.adj[id] {
		if e.Type == EdgeConcept {
			out = append(out, strings.TrimPrefix(other(e, id), "A:")+":"+toFixed(e.W, 1))
		}
	}
	return out
}

func (b *Brain) contextXML(L lit) string {
	st := b.contextStats(L.r)
	var x strings.Builder
	fmt.Fprintf(&x, `<context now="%s" step="%d" tokens="~%d" depth="%d" threshold="%s" hop="%s" order="items sorted by activation">`+"\n",
		b.stamp(), b.Step, st.Tok, L.P.Depth, f2(L.P.Threshold), f2(L.P.Hop))
	x.WriteString("<goal>" + esc(b.Goal) + "</goal>\n")
	if p := b.purpose; p != nil && (p.Name != "" || p.Description != "") {
		x.WriteString(`<purpose` + xa("name", p.Name) + `>` + esc(oneLine(p.Description)) + "</purpose>\n")
	}
	x.WriteString(`<focus refs="` + esc(strings.Join(L.seeds, ",")) + "\"/>\n")
	if abs := b.litConcepts(L); len(abs) > 0 {
		x.WriteString("<concepts note=\"concepts lit by the context\">\n")
		for _, id := range abs {
			fmt.Fprintf(&x, `  <concept id="%s" act="%s" depth="%d">%s</concept>`+"\n", esc(id), f2(L.r.act[id]), L.r.depth[id], esc(b.nodes[id].Text))
		}
		x.WriteString("</concepts>\n")
	}
	for _, sec := range b.sectionOrder() {
		items := b.litItems(L, sec, nil)
		if len(items) == 0 {
			continue
		}
		x.WriteString(`<section name="` + esc(sec) + "\">\n")
		for _, n := range items {
			s := b.itemXML(n, L, false)
			extra := xa("refs", strings.Join(b.refsOf(n.ID, L), ",")) + xa("concepts", strings.Join(b.conceptTags(n.ID), " "))
			if len(n.Fields) > 0 {
				d, _ := n.Fields.MarshalJSON()
				extra += xa("data", string(d))
			}
			s = strings.Replace(s, ">", extra+">", 1)
			x.WriteString("  " + s + "\n")
		}
		x.WriteString("</section>\n")
	}
	if ro, used := b.routesUsed(L); len(ro) > 0 {
		x.WriteString("<routes note=\"connectors that carried activation (not knowledge)\">\n")
		for _, h := range ro {
			n := b.nodes[h]
			fmt.Fprintf(&x, "  <route id=\"%s\" type=\"%s\" label=\"%s\"%s used=\"%d\"/>\n", esc(h), esc(n.Conn.Type), esc(n.Text), xa("key", n.Conn.Key), used[h])
		}
		x.WriteString("</routes>\n")
	}
	faded := 0
	for _, id := range b.order {
		if n := b.nodes[id]; n.Sup != "" && !L.r.has(id) {
			faded++
		}
	}
	nc, _ := b.connStats()
	fmt.Fprintf(&x, `<memory note="not in context: faded or out of depth" total="%d" lit="%d" superseded-faded="%d"/>`+"\n</context>", len(b.order)-nc, len(L.r.litIDs(b)), faded)
	return x.String()
}

func (b *Brain) kindLabel(k string) string { return b.kind(k).Label }

func (b *Brain) contextMD(L lit) string {
	st := b.contextStats(L.r)
	N := len(b.memories())
	reach := L.r.maxDepth()
	lbl := func(id string) string {
		n := b.nodes[id]
		if n == nil {
			return id
		}
		if n.Kind == Concept {
			return "◆ " + n.Text
		}
		return flat(n.Text)
	}
	pct := 0
	if N > 0 {
		pct = int(math.Floor(float64(st.Mem)/float64(N)*100 + .5))
	}
	out := []string{"# Context: " + b.Goal, "",
		fmt.Sprintf("> %s · step %d · ~%d tok · depth %d/%d · threshold %s · hop %s  ", b.stamp(), b.Step, st.Tok, reach, L.P.Depth, fmtNum(L.P.Threshold), fmtNum(L.P.Hop)),
		fmt.Sprintf("> lit %d/%d memories (%d%%) + %d concepts; the rest of memory is faded", st.Mem, N, pct, st.Abs)}
	if p := b.purpose; p != nil && (p.Name != "" || p.Description != "") {
		out[len(out)-1] += "  "
		out = append(out, "> purpose: "+strings.TrimSpace(p.Name+" — "+oneLine(p.Description)))
	}
	out = append(out, "", "## Focus", "")
	seeds := filterStr(L.seeds, func(id string) bool { return b.nodes[id] != nil })
	for _, id := range seeds {
		n := b.nodes[id]
		pin := ""
		if n.Pin {
			pin = " · pinned"
		}
		out = append(out, fmt.Sprintf("- **%s** · %s · w %s%s: %s", id, b.kindLabel(n.Kind), f2(b.nodeW(n)), pin, lbl(id)))
	}
	if len(seeds) == 0 {
		out = append(out, "_no focus: nothing is lit_")
	}
	if abs := b.litConcepts(L); len(abs) > 0 {
		parts := []string{}
		for _, id := range abs {
			parts = append(parts, "◆ "+b.nodes[id].Text+" "+f2(L.r.act[id]))
		}
		out = append(out, "", "## Concepts (in context)", "", strings.Join(parts, " · "))
	}
	for _, sec := range b.sectionOrder() {
		items := b.litItems(L, sec, func(id string) bool { return contains(seeds, id) })
		if len(items) == 0 {
			continue
		}
		out = append(out, "", "## "+capFirst(sec), "")
		for _, n := range items {
			via := L.r.parent[n.ID]
			concepts, refs := []string{}, []string{}
			for _, e := range b.adj[n.ID] {
				v := other(e, n.ID)
				if e.Type == EdgeConcept {
					concepts = append(concepts, b.nodes[v].Text)
				} else if L.r.has(v) && v != via {
					refs = append(refs, v)
				}
			}
			d := fmt.Sprintf("d%d", L.r.depth[n.ID])
			if via != "" {
				d += " ← " + via
			}
			meta := []string{b.kindLabel(n.Kind), "act " + f2(L.r.act[n.ID]), d, "w " + f2(b.nodeW(n))}
			if n.Pin {
				meta = append(meta, "pinned")
			}
			if n.Meta != nil {
				meta = append(meta, "file "+n.Meta.Name)
			}
			if n.Parent != "" {
				meta = append(meta, fmt.Sprintf("chunk #%d of %s", n.Idx+1, n.Parent))
			}
			if n.Ext != nil {
				meta = append(meta, "source "+n.Ext.Source)
			}
			tail := ""
			if len(concepts) > 0 || len(refs) > 0 {
				tail = "  "
			}
			out = append(out, fmt.Sprintf("- **%s** · %s  ", n.ID, strings.Join(meta, " · ")), "  "+flat(n.Text)+tail)
			if rt := L.r.route[n.ID]; len(rt) > 0 {
				out[len(out)-1] = "  " + flat(n.Text) + "  "
				ls := []string{}
				for _, h := range rt {
					ls = append(ls, "⧉ "+h+" ("+b.nodes[h].Conn.Type+": "+b.nodes[h].Text+")")
				}
				out = append(out, "  Routes: "+strings.Join(ls, " → ")+tail)
			}
			if len(n.Fields) > 0 {
				fs := []string{}
				for _, f := range n.Fields {
					fs = append(fs, f.Name+": "+f.Value)
				}
				out[len(out)-1] = "  " + flat(n.Text) + "  "
				out = append(out, "  "+strings.Join(fs, " · ")+tail)
			}
			if tail != "" {
				line := "  "
				if len(concepts) > 0 {
					line += "◆ " + strings.Join(concepts, ", ")
				}
				if len(concepts) > 0 && len(refs) > 0 {
					line += " · "
				}
				if len(refs) > 0 {
					line += "↔ " + strings.Join(refs, ", ")
				}
				out = append(out, line)
			}
		}
	}
	kids := map[string][]string{}
	for _, v := range L.r.litIDs(b) {
		if p := L.r.parent[v]; p != "" {
			kids[p] = append(kids[p], v)
		}
	}
	out = append(out, "", "## Activation tree (where the signal came from)", "", "```")
	var walk func(id, pre string, last, root bool)
	walk = func(id, pre string, last, root bool) {
		if root {
			out = append(out, id+"  "+short(lbl(id), 64))
		} else {
			br := "├─"
			if last {
				br = "└─"
			}
			out = append(out, fmt.Sprintf("%s%s %s %s  %s", pre, br, id, f2(L.r.act[id]), short(lbl(id), 58)))
		}
		ks := append([]string(nil), kids[id]...)
		stableSortDesc(ks, func(k string) float64 { return L.r.act[k] })
		np := ""
		if !root {
			if last {
				np = pre + "   "
			} else {
				np = pre + "│  "
			}
		}
		for i, k := range ks {
			walk(k, np, i == len(ks)-1, false)
		}
	}
	for _, id := range seeds {
		if L.r.has(id) {
			walk(id, "", true, true)
		}
	}
	out = append(out, "```")
	hist := make([]int, reach+1)
	for _, id := range L.r.litIDs(b) {
		hist[L.r.depth[id]]++
	}
	hs := []string{}
	for i := 0; i <= reach; i++ {
		hs = append(hs, fmt.Sprintf("d%d: %d", i, hist[i]))
	}
	sup := 0
	replaced := []*Node{}
	for _, id := range b.order {
		n := b.nodes[id]
		if n.Sup != "" {
			sup++
			if L.r.has(n.Sup) {
				replaced = append(replaced, n)
			}
		}
	}
	out = append(out, "", "## Statistics", "", "| metric | value |", "|---|---|",
		fmt.Sprintf("| lit memories | %d / %d |", st.Mem, N), fmt.Sprintf("| active concepts | %d |", st.Abs), fmt.Sprintf("| tokens (est.) | ~%d |", st.Tok),
		fmt.Sprintf("| reach / depth | %d / %d |", reach, L.P.Depth), "| nodes by depth | "+strings.Join(hs, ", ")+" |",
		"| avg. weight in context | "+f2(st.AvgW)+" |", fmt.Sprintf("| superseded in memory | %d |", sup),
		fmt.Sprintf("| outside context | %d |", N-st.Mem))
	if len(replaced) > 0 {
		out = append(out, "", "### Outdated versions (superseded, do not use)", "")
		for _, n := range replaced {
			out = append(out, fmt.Sprintf("- ~~%s: %s~~ → **%s**", n.ID, flat(n.Text), n.Sup))
		}
	}
	if ro, used := b.routesUsed(L); len(ro) > 0 {
		out = append(out, "", "### Organising connectors", "")
		for _, h := range ro {
			n := b.nodes[h]
			k := ""
			if n.Conn.Key != "" {
				k = " · key " + n.Conn.Key
			}
			out = append(out, fmt.Sprintf("- ⧉ **%s** · %s · %s%s · used by %d items", h, n.Conn.Type, n.Text, k, used[h]))
		}
	}
	for _, w := range L.warn {
		out = append(out, "", "> warn: "+w)
	}
	return strings.Join(out, "\n") + "\n"
}

// ContextXML renders the lit slice as context XML.
func (b *Brain) ContextXML() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maskText(b.contextXML(b.curLit()))
}

// ContextMD renders the lit slice as Markdown.
func (b *Brain) ContextMD() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maskText(b.contextMD(b.curLit()))
}

// Status returns the status line used by MCP results.
func (b *Brain) Status(extra string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status(extra)
}

func (b *Brain) status(extra string) string {
	st := b.contextStats(b.cur)
	N := len(b.memories())
	focus := strings.Join(b.seeds, ",")
	if focus == "" {
		focus = "—"
	}
	s := fmt.Sprintf("[ok] step %d · lit %d/%d memories + %d concepts · ~%d tok · depth %d/%d thr %s · focus %s",
		b.Step, st.Mem, N, st.Abs, st.Tok, b.cur.maxDepth(), b.P.Depth, fmtNum(b.P.Threshold), focus)
	if st.Tok > 4000 {
		s += " · warn: context > 4k tokens: narrow the focus, lower depth or raise threshold"
	}
	for _, w := range b.fedWarnings() {
		s += " · warn: " + w
	}
	if extra != "" {
		s += " · " + extra
	}
	return s
}

func (b *Brain) hasExternal() bool {
	for _, id := range b.order {
		if b.nodes[id].Ext != nil {
			return true
		}
	}
	return false
}

func oneLine(s string) string {
	s = flat(s)
	if i := strings.Index(s, ". "); i > 0 && i < 200 {
		return s[:i+1]
	}
	return short(s, 200)
}
