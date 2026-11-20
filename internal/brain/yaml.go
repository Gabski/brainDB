// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// A stdlib-only YAML subset: block maps, block lists, plain / single / double quoted scalars, flow maps and lists,
// `#` comments and `|` / `>` block scalars. Maps decode to *OMap (key order and line numbers kept), lists to []any,
// numbers to json.Number, plus bool and nil.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// YAMLError is a parse error with a 1-based line number.
type YAMLError struct {
	Line int
	Msg  string
}

func (e *YAMLError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

type yline struct {
	n      int // 1-based
	indent int
	raw    string // full raw line
	text   string // content after indent, comments stripped, right-trimmed
}

type yparser struct {
	lines []yline
	i     int
}

// stripComment removes a trailing comment outside quotes/flow strings.
func stripComment(s string) string {
	inS, inD := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inD:
			if c == '\\' {
				i++
			} else if c == '"' {
				inD = false
			}
		case inS:
			if c == '\'' {
				inS = false
			}
		case c == '"':
			inD = true
		case c == '\'':
			inS = true
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return strings.TrimRight(s[:i], " \t")
		}
	}
	return strings.TrimRight(s, " \t")
}

// ParseYAML parses a YAML document.
func ParseYAML(src string) (any, error) {
	p := &yparser{}
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		if strings.Contains(raw, "\t") && strings.TrimLeft(raw, " ") != "" && strings.HasPrefix(strings.TrimLeft(raw, " "), "\t") {
			return nil, &YAMLError{i + 1, "tabs are not allowed for indentation"}
		}
		ind := len(raw) - len(strings.TrimLeft(raw, " "))
		p.lines = append(p.lines, yline{n: i + 1, indent: ind, raw: raw, text: stripComment(raw[ind:])})
	}
	p.skipBlank()
	if p.i >= len(p.lines) {
		return nil, nil
	}
	if p.lines[p.i].text == "---" {
		p.i++
		p.skipBlank()
	}
	if p.i >= len(p.lines) {
		return nil, nil
	}
	v, err := p.parseBlock(p.lines[p.i].indent)
	if err != nil {
		return nil, err
	}
	p.skipBlank()
	if p.i < len(p.lines) && p.lines[p.i].text != "..." && p.lines[p.i].text != "---" {
		return nil, &YAMLError{p.lines[p.i].n, "unexpected content (check indentation)"}
	}
	return v, nil
}

func (p *yparser) skipBlank() {
	for p.i < len(p.lines) && p.lines[p.i].text == "" {
		p.i++
	}
}

func isListItem(t string) bool { return t == "-" || strings.HasPrefix(t, "- ") }

func (p *yparser) parseBlock(indent int) (any, error) {
	p.skipBlank()
	if p.i >= len(p.lines) {
		return nil, nil
	}
	l := p.lines[p.i]
	if isListItem(l.text) {
		return p.parseList(indent)
	}
	if _, _, ok := splitKey(l.text); ok {
		return p.parseMap(indent)
	}
	// a lone scalar (possibly multi-line flow)
	p.i++
	return p.scalarOrFlow(l.text, l)
}

// splitKey finds "key: value" / "key:" outside quotes and brackets.
func splitKey(t string) (string, string, bool) {
	if t == "" || t[0] == '[' || t[0] == '{' || isListItem(t) {
		return "", "", false
	}
	if t[0] == '"' || t[0] == '\'' {
		q := t[0]
		for i := 1; i < len(t); i++ {
			if q == '"' && t[i] == '\\' {
				i++
				continue
			}
			if t[i] == q {
				if q == '\'' && i+1 < len(t) && t[i+1] == '\'' {
					i++
					continue
				}
				rest := t[i+1:]
				if rest == ":" || strings.HasPrefix(rest, ": ") {
					k, err := unquote(t[:i+1])
					if err != nil {
						return "", "", false
					}
					return k, strings.TrimSpace(strings.TrimPrefix(rest, ":")), true
				}
				return "", "", false
			}
		}
		return "", "", false
	}
	for i := 0; i < len(t); i++ {
		if t[i] == ':' && (i == len(t)-1 || t[i+1] == ' ') {
			return strings.TrimSpace(t[:i]), strings.TrimSpace(t[i+1:]), true
		}
	}
	return "", "", false
}

func (p *yparser) parseMap(indent int) (any, error) {
	m := NewOMap()
	m.Lines = map[string]int{}
	for {
		p.skipBlank()
		if p.i >= len(p.lines) {
			break
		}
		l := p.lines[p.i]
		if l.indent < indent || l.text == "---" || l.text == "..." {
			break
		}
		if l.indent > indent {
			return nil, &YAMLError{l.n, "unexpected indentation"}
		}
		if isListItem(l.text) {
			return nil, &YAMLError{l.n, "a list item where a mapping key was expected"}
		}
		k, rest, ok := splitKey(l.text)
		if !ok {
			return nil, &YAMLError{l.n, fmt.Sprintf("expected \"key: value\", got %q", l.text)}
		}
		if _, dup := m.Vals[k]; dup {
			return nil, &YAMLError{l.n, fmt.Sprintf("duplicate key %q", k)}
		}
		p.i++
		v, err := p.value(rest, l, indent)
		if err != nil {
			return nil, err
		}
		m.Set(k, v)
		m.Lines[k] = l.n
	}
	return m, nil
}

// value parses what follows "key:" or "- ": inline scalar/flow, block scalar, or a nested block.
func (p *yparser) value(rest string, l yline, indent int) (any, error) {
	if rest == "" {
		p.skipBlank()
		if p.i < len(p.lines) {
			nl := p.lines[p.i]
			if nl.indent > indent || (nl.indent == indent && isListItem(nl.text)) {
				return p.parseBlock(nl.indent)
			}
		}
		return nil, nil
	}
	if rest[0] == '|' || rest[0] == '>' {
		return p.blockScalar(rest, indent, l)
	}
	return p.scalarOrFlow(rest, l)
}

func (p *yparser) blockScalar(h string, indent int, l yline) (any, error) {
	folded := h[0] == '>'
	chomp := byte(0)
	if len(h) > 1 {
		chomp = h[1]
	}
	lines := []string{}
	ci := -1
	for p.i < len(p.lines) {
		nl := p.lines[p.i]
		blank := strings.TrimSpace(nl.raw) == ""
		if !blank && nl.indent <= indent {
			break
		}
		if !blank && ci < 0 {
			ci = nl.indent
		}
		if blank {
			lines = append(lines, "")
		} else {
			if nl.indent < ci {
				return nil, &YAMLError{nl.n, "bad indentation in block scalar"}
			}
			lines = append(lines, nl.raw[ci:])
		}
		p.i++
	}
	// trailing blank lines
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	trail := len(lines) - end
	body := lines[:end]
	var s string
	if folded {
		var b strings.Builder
		for i, x := range body {
			if i > 0 {
				if x == "" || body[i-1] == "" {
					b.WriteString("\n")
				} else {
					b.WriteString(" ")
				}
			}
			b.WriteString(x)
		}
		s = b.String()
	} else {
		s = strings.Join(body, "\n")
	}
	switch chomp {
	case '-':
	case '+':
		s += strings.Repeat("\n", trail+1)
	default:
		if len(body) > 0 {
			s += "\n"
		}
	}
	return s, nil
}

func (p *yparser) parseList(indent int) (any, error) {
	out := []any{}
	for {
		p.skipBlank()
		if p.i >= len(p.lines) {
			break
		}
		l := p.lines[p.i]
		if l.indent != indent || !isListItem(l.text) {
			if l.indent > indent {
				return nil, &YAMLError{l.n, "unexpected indentation in list"}
			}
			break
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		if rest == "" {
			p.i++
			v, err := p.value("", l, indent)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		if _, _, ok := splitKey(rest); ok && rest[0] != '{' && rest[0] != '[' {
			// map inside a list item: rewrite the current line as the first key at indent+2
			off := strings.Index(l.raw, rest)
			p.lines[p.i] = yline{n: l.n, indent: off, raw: l.raw, text: rest}
			v, err := p.parseMap(off)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		if isListItem(rest) {
			off := strings.Index(l.raw[l.indent+1:], "-") + l.indent + 1
			p.lines[p.i] = yline{n: l.n, indent: off, raw: l.raw, text: rest}
			v, err := p.parseList(off)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		p.i++
		v, err := p.value(rest, l, indent)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// scalarOrFlow parses an inline value; flow collections may continue on following lines.
func (p *yparser) scalarOrFlow(s string, l yline) (any, error) {
	if s[0] == '[' || s[0] == '{' {
		for !flowBalanced(s) && p.i < len(p.lines) {
			s += " " + strings.TrimSpace(p.lines[p.i].text)
			p.i++
		}
		f := &flowP{s: s, line: l.n}
		v, err := f.value()
		if err != nil {
			return nil, err
		}
		f.ws()
		if f.i < len(f.s) {
			return nil, &YAMLError{l.n, "unexpected characters after flow collection"}
		}
		return v, nil
	}
	if s[0] == '"' || s[0] == '\'' {
		v, err := unquote(s)
		if err != nil {
			return nil, &YAMLError{l.n, err.Error()}
		}
		return v, nil
	}
	return plainScalar(s), nil
}

func flowBalanced(s string) bool {
	d := 0
	inS, inD := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inD:
			if c == '\\' {
				i++
			} else if c == '"' {
				inD = false
			}
		case inS:
			if c == '\'' {
				inS = false
			}
		case c == '"':
			inD = true
		case c == '\'':
			inS = true
		case c == '[' || c == '{':
			d++
		case c == ']' || c == '}':
			d--
		}
	}
	return d <= 0
}

var yNumRe = regexp.MustCompile(`^[-+]?(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?$`)

func plainScalar(s string) any {
	switch s {
	case "null", "Null", "NULL", "~":
		return nil
	case "true", "True", "TRUE":
		return true
	case "false", "False", "FALSE":
		return false
	}
	if yNumRe.MatchString(s) {
		return json.Number(strings.TrimPrefix(s, "+"))
	}
	return s
}

func unquote(s string) (string, error) {
	if len(s) < 2 || s[len(s)-1] != s[0] {
		return "", errf("unterminated quoted string")
	}
	if s[0] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	var b strings.Builder
	in := s[1 : len(s)-1]
	for i := 0; i < len(in); i++ {
		c := in[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(in) {
			return "", errf("bad escape")
		}
		switch in[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '0':
			b.WriteByte(0)
		case '"', '\\', '/', ' ':
			b.WriteByte(in[i])
		case 'u', 'U', 'x':
			n := map[byte]int{'u': 4, 'U': 8, 'x': 2}[in[i]]
			if i+n >= len(in)+0 && i+n > len(in)-1+1 {
				return "", errf("bad escape")
			}
			v, err := strconv.ParseUint(in[i+1:i+1+n], 16, 32)
			if err != nil {
				return "", errf("bad escape")
			}
			b.WriteRune(rune(v))
			i += n
		default:
			return "", errf("unknown escape \\%c", in[i])
		}
	}
	return b.String(), nil
}

type flowP struct {
	s    string
	i    int
	line int
}

func (f *flowP) ws() {
	for f.i < len(f.s) && (f.s[f.i] == ' ' || f.s[f.i] == '\t') {
		f.i++
	}
}

func (f *flowP) fail(msg string) error { return &YAMLError{f.line, msg} }

func (f *flowP) value() (any, error) {
	f.ws()
	if f.i >= len(f.s) {
		return nil, f.fail("unexpected end of flow collection")
	}
	switch f.s[f.i] {
	case '[':
		f.i++
		out := []any{}
		for {
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == ']' {
				f.i++
				return out, nil
			}
			v, err := f.value()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == ',' {
				f.i++
				continue
			}
			if f.i < len(f.s) && f.s[f.i] == ']' {
				f.i++
				return out, nil
			}
			return nil, f.fail("expected , or ] in flow list")
		}
	case '{':
		f.i++
		m := NewOMap()
		for {
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == '}' {
				f.i++
				return m, nil
			}
			k, err := f.scalar(true)
			if err != nil {
				return nil, err
			}
			ks := scalarString(k)
			f.ws()
			var v any
			if f.i < len(f.s) && f.s[f.i] == ':' {
				f.i++
				if v, err = f.value(); err != nil {
					return nil, err
				}
			}
			if _, dup := m.Vals[ks]; dup {
				return nil, f.fail(fmt.Sprintf("duplicate key %q", ks))
			}
			m.Set(ks, v)
			f.ws()
			if f.i < len(f.s) && f.s[f.i] == ',' {
				f.i++
				continue
			}
			if f.i < len(f.s) && f.s[f.i] == '}' {
				f.i++
				return m, nil
			}
			return nil, f.fail("expected , or } in flow map")
		}
	}
	return f.scalar(false)
}

func (f *flowP) scalar(key bool) (any, error) {
	f.ws()
	if f.i < len(f.s) && (f.s[f.i] == '"' || f.s[f.i] == '\'') {
		q := f.s[f.i]
		j := f.i + 1
		for j < len(f.s) {
			if q == '"' && f.s[j] == '\\' {
				j += 2
				continue
			}
			if f.s[j] == q {
				if q == '\'' && j+1 < len(f.s) && f.s[j+1] == '\'' {
					j += 2
					continue
				}
				break
			}
			j++
		}
		if j >= len(f.s) {
			return nil, f.fail("unterminated quoted string")
		}
		v, err := unquote(f.s[f.i : j+1])
		if err != nil {
			return nil, f.fail(err.Error())
		}
		f.i = j + 1
		return v, nil
	}
	j := f.i
	for j < len(f.s) {
		c := f.s[j]
		if c == ',' || c == ']' || c == '}' || (key && c == ':' && (j+1 >= len(f.s) || f.s[j+1] == ' ' || f.s[j+1] == ',' || f.s[j+1] == '}')) ||
			(!key && c == ':' && j+1 < len(f.s) && f.s[j+1] == ' ' && false) {
			break
		}
		j++
	}
	s := strings.TrimSpace(f.s[f.i:j])
	f.i = j
	if key {
		return s, nil
	}
	return plainScalar(s), nil
}

// ---------------------------------------------------------------- emitter

// EmitYAML renders a value (OMap / []any / scalars / any JSON-marshalable value) as block YAML.
func EmitYAML(v any) string {
	switch v.(type) {
	case *OMap, []any, nil, string, bool, json.Number, float64, int, int64:
	default:
		v = toOrdered(v)
	}
	var b strings.Builder
	emitNode(&b, v, 0, false)
	s := b.String()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func emitNode(b *strings.Builder, v any, ind int, inline bool) {
	pad := strings.Repeat(" ", ind)
	switch x := v.(type) {
	case *OMap:
		if len(x.Keys) == 0 {
			b.WriteString("{}\n")
			return
		}
		for i, k := range x.Keys {
			if !(inline && i == 0) {
				b.WriteString(pad)
			}
			b.WriteString(yamlScalar(k) + ":")
			emitChild(b, x.Vals[k], ind)
		}
	case []any:
		if len(x) == 0 {
			b.WriteString("[]\n")
			return
		}
		for i, it := range x {
			if !(inline && i == 0) {
				b.WriteString(pad)
			}
			b.WriteString("-")
			switch y := it.(type) {
			case *OMap:
				if len(y.Keys) == 0 {
					b.WriteString(" {}\n")
				} else {
					b.WriteString(" ")
					emitNode(b, y, ind+2, true)
				}
			case []any:
				if len(y) == 0 {
					b.WriteString(" []\n")
				} else {
					b.WriteString(" ")
					emitNode(b, y, ind+2, true)
				}
			default:
				emitChild(b, it, ind)
			}
		}
	default:
		b.WriteString(yamlScalar(v) + "\n")
	}
}

func emitChild(b *strings.Builder, v any, ind int) {
	switch y := v.(type) {
	case *OMap:
		if len(y.Keys) == 0 {
			b.WriteString(" {}\n")
			return
		}
		b.WriteString("\n")
		emitNode(b, y, ind+2, false)
	case []any:
		if len(y) == 0 {
			b.WriteString(" []\n")
			return
		}
		b.WriteString("\n")
		emitNode(b, y, ind+2, false)
	case string:
		if strings.Contains(y, "\n") && !strings.HasPrefix(y, " ") && !strings.ContainsAny(y, "\r\t") {
			body := strings.TrimRight(y, "\n")
			trail := len(y) - len(body)
			h := "|"
			if trail == 0 {
				h = "|-"
			} else if trail > 1 {
				h = "|+"
			}
			b.WriteString(" " + h + "\n")
			pad := strings.Repeat(" ", ind+2)
			for _, l := range strings.Split(body, "\n") {
				if l == "" {
					b.WriteString("\n")
				} else {
					b.WriteString(pad + l + "\n")
				}
			}
			if trail > 1 {
				b.WriteString(strings.Repeat("\n", trail-1))
			}
			return
		}
		b.WriteString(" " + yamlScalar(y) + "\n")
	default:
		b.WriteString(" " + yamlScalar(v) + "\n")
	}
}

func yamlScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return `".nan"`
		}
		return fmtNum(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		if plainSafe(x) {
			return x
		}
		d, _ := json.Marshal(x)
		return string(d)
	default:
		d, _ := json.Marshal(x)
		return string(d)
	}
}

func plainSafe(s string) bool {
	if s == "" || s != strings.TrimSpace(s) {
		return false
	}
	if strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") {
		return false
	}
	if strings.Contains(s, ": ") || strings.Contains(s, " #") || strings.HasSuffix(s, ":") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	if _, ok := plainScalar(s).(string); !ok {
		return false
	}
	return true
}

// ---------------------------------------------------------------- in-place editing

// indentOf returns the number of leading spaces, and whether the line is blank or a comment.
func indentOf(line string) (int, bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	rest := strings.TrimSpace(line[i:])
	return i, rest == "" || strings.HasPrefix(rest, "#")
}

// blockEnd returns the index (exclusive) of the last line belonging to the key that starts at
// lines[start] with the given indent. Blank and comment lines that trail the block are left to the
// next key, which is where a reader would expect the comment documenting it to sit.
func blockEnd(lines []string, start, indent int) int {
	end := start + 1
	for i := start + 1; i < len(lines); i++ {
		ind, skip := indentOf(lines[i])
		if skip {
			continue // decided by whatever follows
		}
		// A sequence may sit at the key's own indentation ("key:" then "- item").
		if ind > indent || (ind == indent && strings.HasPrefix(strings.TrimSpace(lines[i]), "- ")) {
			end = i + 1
			continue
		}
		break
	}
	return end
}

// mapAt walks path through nested maps, returning the map holding the last element.
func mapAt(root *OMap, path []string) *OMap {
	m := root
	for _, k := range path {
		next, ok := m.Vals[k].(*OMap)
		if !ok {
			return nil
		}
		m = next
	}
	return m
}

// SpliceYAML replaces the value at path in src, rewriting only the lines that hold it. Every other
// line — comments, blank lines, key order, inline flow style — survives byte for byte, which a
// parse-and-re-emit round trip cannot promise: the parser drops comments.
//
// When the path does not exist yet, the key is appended to its parent (or the parent to the file).
// When src cannot be parsed, the caller gets a freshly emitted document instead.
func SpliceYAML(src string, path []string, value any) string {
	if len(path) == 0 {
		return EmitYAML(value)
	}
	build := func() string {
		v := value
		for i := len(path) - 1; i >= 0; i-- {
			v = NewOMap().Set(path[i], v)
		}
		return EmitYAML(v)
	}
	if strings.TrimSpace(src) == "" {
		return build()
	}
	parsed, err := ParseYAML(src)
	if err != nil {
		return build()
	}
	root, ok := parsed.(*OMap)
	if !ok {
		return build()
	}
	parent := mapAt(root, path[:len(path)-1])
	if parent == nil {
		// The parent block is missing: append the whole path at the end of the file.
		return strings.TrimRight(src, "\n") + "\n" + build()
	}
	key := path[len(path)-1]
	indent := 2 * (len(path) - 1)
	block := EmitYAML(NewOMap().Set(key, value))
	if indent > 0 {
		pad := strings.Repeat(" ", indent)
		out := []string{}
		for _, l := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
			if l == "" {
				out = append(out, l)
			} else {
				out = append(out, pad+l)
			}
		}
		block = strings.Join(out, "\n") + "\n"
	}
	lines := strings.Split(src, "\n")
	ln, found := parent.Lines[key]
	if !found || ln < 1 || ln > len(lines) {
		// New key: put it at the end of the parent's own block.
		if len(path) == 1 {
			return strings.TrimRight(src, "\n") + "\n" + block
		}
		pln, ok := mapAtLine(root, path[:len(path)-1])
		if !ok || pln < 1 || pln > len(lines) {
			return strings.TrimRight(src, "\n") + "\n" + build()
		}
		at := blockEnd(lines, pln-1, 2*(len(path)-2))
		rest := append([]string{}, lines[at:]...)
		return strings.Join(append(append(lines[:at:at], strings.Split(strings.TrimRight(block, "\n"), "\n")...), rest...), "\n")
	}
	start := ln - 1
	keyIndent, _ := indentOf(lines[start])
	end := blockEnd(lines, start, keyIndent)
	rest := append([]string{}, lines[end:]...)
	return strings.Join(append(append(lines[:start:start], strings.Split(strings.TrimRight(block, "\n"), "\n")...), rest...), "\n")
}

// mapAtLine returns the source line of the map at path.
func mapAtLine(root *OMap, path []string) (int, bool) {
	if len(path) == 0 {
		return 0, false
	}
	m := mapAt(root, path[:len(path)-1])
	if m == nil {
		return 0, false
	}
	ln, ok := m.Lines[path[len(path)-1]]
	return ln, ok
}
