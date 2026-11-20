// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"regexp"
	"strconv"
	"strings"
)

// XMLAttr is one attribute (order preserved).
type XMLAttr struct{ Name, Value string }

// XMLEl is a lenient XML element tree, a port of the reference mini parser.
type XMLEl struct {
	Name     string
	Attrs    []XMLAttr
	Children []*XMLEl
	Text     string
}

func (e *XMLEl) Attr(k string) (string, bool) {
	if e == nil {
		return "", false
	}
	for _, a := range e.Attrs {
		if a.Name == k {
			return a.Value, true
		}
	}
	return "", false
}

func (e *XMLEl) A(k string) string { v, _ := e.Attr(k); return v }

func (e *XMLEl) Kids(name string) []*XMLEl {
	if e == nil {
		return nil
	}
	out := []*XMLEl{}
	for _, c := range e.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func (e *XMLEl) One(name string) *XMLEl {
	if k := e.Kids(name); len(k) > 0 {
		return k[0]
	}
	return nil
}

func (e *XMLEl) All(name string, out []*XMLEl) []*XMLEl {
	if e == nil {
		return out
	}
	for _, c := range e.Children {
		if c.Name == name {
			out = append(out, c)
		}
		out = c.All(name, out)
	}
	return out
}

var (
	xmlTokRe  = regexp.MustCompile(`(?s)<!--.*?-->|<\?.*?\?>|<!\[CDATA\[(.*?)\]\]>|<(/?)([\w:.-]+)((?:\s+[\w:.-]+\s*=\s*(?:"[^"]*"|'[^']*'))*)\s*(/?)>|([^<]+)|<`)
	xmlAttrRe = regexp.MustCompile(`([\w:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	xmlEntRe  = regexp.MustCompile(`(?i)&(lt|gt|quot|apos|amp|#\d+|#x[0-9a-f]+);`)
)

func xmlUnesc(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return xmlEntRe.ReplaceAllStringFunc(s, func(m string) string {
		e := m[1 : len(m)-1]
		switch strings.ToLower(e) {
		case "lt":
			return "<"
		case "gt":
			return ">"
		case "quot":
			return `"`
		case "apos":
			return "'"
		case "amp":
			return "&"
		}
		var v int64
		var err error
		if e[1] == 'x' || e[1] == 'X' {
			v, err = strconv.ParseInt(e[2:], 16, 32)
		} else {
			v, err = strconv.ParseInt(e[1:], 10, 32)
		}
		if err != nil {
			return m
		}
		return string(rune(v))
	})
}

// ParseXML parses src leniently and returns the first top-level element (nil if none).
func ParseXML(src string) *XMLEl {
	root := &XMLEl{Name: "#root"}
	stack := []*XMLEl{root}
	for _, m := range xmlTokRe.FindAllStringSubmatchIndex(src, -1) {
		top := stack[len(stack)-1]
		g := func(i int) (string, bool) {
			if m[2*i] < 0 {
				return "", false
			}
			return src[m[2*i]:m[2*i+1]], true
		}
		if s, ok := g(1); ok {
			top.Text += s
			continue
		}
		if s, ok := g(6); ok {
			top.Text += xmlUnesc(s)
			continue
		}
		name, ok := g(3)
		if !ok {
			continue
		}
		if cl, _ := g(2); cl != "" {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		el := &XMLEl{Name: name}
		attrs, _ := g(4)
		for _, a := range xmlAttrRe.FindAllStringSubmatch(attrs, -1) {
			v := a[2]
			if v == "" && a[3] != "" {
				v = a[3]
			}
			el.Attrs = append(el.Attrs, XMLAttr{a[1], xmlUnesc(v)})
		}
		top.Children = append(top.Children, el)
		if sc, _ := g(5); sc == "" {
			stack = append(stack, el)
		}
	}
	if len(root.Children) == 0 {
		return nil
	}
	return root.Children[0]
}

var xmlEscRepl = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// esc mirrors the reference esc().
func esc(s string) string { return xmlEscRepl.Replace(s) }

// xa renders an optional attribute (omitted when empty).
func xa(k, v string) string {
	if v == "" {
		return ""
	}
	return " " + k + `="` + esc(v) + `"`
}
