// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"strings"
)

// SQL dump import (PostgreSQL / MySQL / SQLite dialects) with a stdlib tokenizer: CREATE TABLE (columns, PRIMARY KEY,
// REFERENCES / FOREIGN KEY), ALTER TABLE … ADD CONSTRAINT, INSERT INTO … VALUES (…), (…) and COPY … FROM stdin.

type sqlFK struct{ col, refTable, refCol string }

type sqlTable struct {
	name string
	cols []string
	pk   []string
	fks  []sqlFK
	rows []Fields
	ids  map[string]string // pk value → node id
}

// splitSQL splits a dump into statements, returning COPY data blocks attached to their statement.
func splitSQL(src string) []string {
	out := []string{}
	var cur strings.Builder
	i := 0
	n := len(src)
	for i < n {
		c := src[i]
		switch {
		case c == '-' && i+1 < n && src[i+1] == '-':
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		case c == '#' && (i == 0 || src[i-1] == '\n'):
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		case c == '/' && i+1 < n && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				i = n
			} else {
				i += end + 4
			}
			continue
		case c == '\'' || c == '"' || c == '`':
			q := c
			cur.WriteByte(c)
			i++
			for i < n {
				ch := src[i]
				cur.WriteByte(ch)
				i++
				if ch == '\\' && q == '\'' && i < n {
					cur.WriteByte(src[i])
					i++
					continue
				}
				if ch == q {
					if i < n && src[i] == q {
						cur.WriteByte(src[i])
						i++
						continue
					}
					break
				}
			}
			continue
		case c == '$' && i+1 < n && src[i+1] == '$':
			end := strings.Index(src[i+2:], "$$")
			if end < 0 {
				end = n - i - 2
			}
			cur.WriteString(src[i : i+4+end])
			i += end + 4
			continue
		case c == ';':
			st := strings.TrimSpace(cur.String())
			cur.Reset()
			i++
			if strings.HasPrefix(strings.ToUpper(st), "COPY ") && strings.Contains(strings.ToUpper(st), "FROM STDIN") {
				// data follows until a line with \.
				if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
					i += j + 1
				}
				end := strings.Index(src[i:], "\n\\.")
				data := ""
				if strings.HasPrefix(src[i:], "\\.") {
					end = -2
				}
				if end >= 0 {
					data = src[i : i+end]
					i += end + 3
				} else if end == -2 {
					i += 2
				} else {
					data = src[i:]
					i = n
				}
				st += "\x00" + data
			}
			if st != "" {
				out = append(out, st)
			}
			continue
		}
		cur.WriteByte(c)
		i++
	}
	if st := strings.TrimSpace(cur.String()); st != "" {
		out = append(out, st)
	}
	return out
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "."); i >= 0 && !strings.HasSuffix(s, `"`) || strings.Count(s, ".") > 0 && strings.HasPrefix(s, `"`) {
		parts := strings.Split(s, ".")
		s = parts[len(parts)-1]
	}
	return strings.Trim(s, "`\"[]")
}

// splitTop splits on commas outside parentheses and quotes.
func splitTop(s string) []string {
	out := []string{}
	depth := 0
	var q byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if q != 0 {
			if c == '\\' && q == '\'' {
				i++
				continue
			}
			if c == q {
				if i+1 < len(s) && s[i+1] == q {
					i++
					continue
				}
				q = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			q = c
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func parenBody(s string) (string, string) {
	i := strings.IndexByte(s, '(')
	if i < 0 {
		return "", s
	}
	depth := 0
	var q byte
	for j := i; j < len(s); j++ {
		c := s[j]
		if q != 0 {
			if c == '\\' && q == '\'' {
				j++
				continue
			}
			if c == q {
				q = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			q = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[i+1 : j], s[j+1:]
			}
		}
	}
	return s[i+1:], ""
}

func identList(s string) []string {
	out := []string{}
	for _, p := range splitTop(s) {
		if p = unquoteIdent(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// sqlValue decodes one literal.
func sqlValue(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "NULL") {
		return "", false
	}
	if len(s) >= 2 && (s[0] == '\'' || (s[0] == 'E' || s[0] == 'e') && s[1] == '\'') {
		if s[0] != '\'' {
			s = s[1:]
		}
		body := s[1 : len(s)-1]
		var b strings.Builder
		for i := 0; i < len(body); i++ {
			c := body[i]
			if c == '\'' && i+1 < len(body) && body[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			if c == '\\' && i+1 < len(body) {
				i++
				switch body[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case 'r':
					b.WriteByte('\r')
				case '0':
				default:
					b.WriteByte(body[i])
				}
				continue
			}
			b.WriteByte(c)
		}
		return b.String(), true
	}
	if i := strings.Index(s, "::"); i > 0 {
		return sqlValue(s[:i])
	}
	switch strings.ToUpper(s) {
	case "TRUE":
		return "true", true
	case "FALSE":
		return "false", true
	}
	return s, true
}

func copyUnescape(s string) (string, bool) {
	if s == `\N` {
		return "", false
	}
	r := strings.NewReplacer(`\t`, "\t", `\n`, "\n", `\r`, "\r", `\\`, `\`)
	return r.Replace(s), true
}

func (im *importer) table(name string) *sqlTable {
	t := im.tables[name]
	if t == nil {
		t = &sqlTable{name: name, ids: map[string]string{}}
		im.tables[name] = t
	}
	return t
}

func parseColumnDefs(t *sqlTable, body string) {
	for _, def := range splitTop(body) {
		d := strings.TrimSpace(def)
		u := strings.ToUpper(d)
		switch {
		case strings.HasPrefix(u, "PRIMARY KEY"), strings.HasPrefix(u, "CONSTRAINT") && strings.Contains(u, "PRIMARY KEY"):
			b, _ := parenBody(d[strings.Index(u, "PRIMARY KEY"):])
			t.pk = identList(b)
		case strings.HasPrefix(u, "FOREIGN KEY"), strings.HasPrefix(u, "CONSTRAINT") && strings.Contains(u, "FOREIGN KEY"):
			rest := d[strings.Index(u, "FOREIGN KEY"):]
			cols, after := parenBody(rest)
			ua := strings.ToUpper(after)
			if k := strings.Index(ua, "REFERENCES"); k >= 0 {
				ref := strings.TrimSpace(after[k+10:])
				rt := ref
				rc := ""
				if p := strings.IndexByte(ref, '('); p >= 0 {
					rt = ref[:p]
					b, _ := parenBody(ref[p:])
					if l := identList(b); len(l) > 0 {
						rc = l[0]
					}
				}
				cl := identList(cols)
				if len(cl) > 0 {
					t.fks = append(t.fks, sqlFK{cl[0], unquoteIdent(strings.Fields(rt)[0]), rc})
				}
			}
		case strings.HasPrefix(u, "UNIQUE"), strings.HasPrefix(u, "KEY "), strings.HasPrefix(u, "INDEX"), strings.HasPrefix(u, "CHECK"), strings.HasPrefix(u, "CONSTRAINT"), strings.HasPrefix(u, "FULLTEXT"):
		default:
			f := strings.Fields(d)
			if len(f) == 0 {
				continue
			}
			col := unquoteIdent(f[0])
			t.cols = append(t.cols, col)
			if strings.Contains(u, "PRIMARY KEY") {
				t.pk = []string{col}
			}
			if k := strings.Index(u, "REFERENCES"); k >= 0 {
				ref := strings.TrimSpace(d[k+10:])
				rt, rc := ref, ""
				if p := strings.IndexByte(ref, '('); p >= 0 {
					rt = ref[:p]
					b, _ := parenBody(ref[p:])
					if l := identList(b); len(l) > 0 {
						rc = l[0]
					}
				}
				t.fks = append(t.fks, sqlFK{col, unquoteIdent(strings.Fields(rt)[0]), rc})
			}
		}
	}
}

func (im *importer) sql(f InFile, rel string, r *ImportRule, labels []string) (string, int, string, error) {
	im.tables = map[string]*sqlTable{}
	order := []string{}
	for _, st := range splitSQL(string(f.Data)) {
		data := ""
		if i := strings.IndexByte(st, 0); i >= 0 {
			st, data = st[:i], st[i+1:]
		}
		u := strings.ToUpper(st)
		switch {
		case strings.HasPrefix(u, "CREATE TABLE"):
			head := st[12:]
			hu := strings.ToUpper(head)
			if strings.HasPrefix(strings.TrimSpace(hu), "IF NOT EXISTS") {
				head = strings.TrimSpace(head)[13:]
			}
			p := strings.IndexByte(head, '(')
			if p < 0 {
				continue
			}
			name := unquoteIdent(strings.TrimSpace(head[:p]))
			t := im.table(name)
			if !contains(order, name) {
				order = append(order, name)
			}
			body, _ := parenBody(head)
			parseColumnDefs(t, body)
		case strings.HasPrefix(u, "ALTER TABLE"):
			rest := strings.TrimSpace(st[11:])
			if strings.HasPrefix(strings.ToUpper(rest), "ONLY ") {
				rest = strings.TrimSpace(rest[5:])
			}
			name := unquoteIdent(strings.Fields(rest)[0])
			t := im.table(name)
			ru := strings.ToUpper(rest)
			if k := strings.Index(ru, "ADD "); k >= 0 {
				parseColumnDefs(t, rest[k+4:])
			}
		case strings.HasPrefix(u, "INSERT INTO"), strings.HasPrefix(u, "INSERT IGNORE INTO"), strings.HasPrefix(u, "REPLACE INTO"):
			k := strings.Index(u, "INTO") + 4
			rest := strings.TrimSpace(st[k:])
			vi := strings.Index(strings.ToUpper(rest), "VALUES")
			if vi < 0 {
				continue
			}
			target := strings.TrimSpace(rest[:vi])
			var cols []string
			if p := strings.IndexByte(target, '('); p >= 0 {
				b, _ := parenBody(target[p:])
				cols = identList(b)
				target = target[:p]
			}
			name := unquoteIdent(target)
			t := im.table(name)
			if !contains(order, name) {
				order = append(order, name)
			}
			if cols == nil {
				cols = t.cols
			}
			vals := rest[vi+6:]
			for {
				body, after := parenBody(vals)
				if body == "" && strings.TrimSpace(after) == "" {
					break
				}
				fs := Fields{}
				for i, v := range splitTop(body) {
					val, ok := sqlValue(v)
					if i < len(cols) {
						if !ok {
							val = ""
						}
						fs = append(fs, Field{cols[i], val})
					}
				}
				t.rows = append(t.rows, fs)
				after = strings.TrimSpace(after)
				if !strings.HasPrefix(after, ",") {
					break
				}
				vals = after[1:]
			}
		case strings.HasPrefix(u, "COPY "):
			rest := strings.TrimSpace(st[5:])
			p := strings.IndexByte(rest, '(')
			fi := strings.Index(strings.ToUpper(rest), " FROM")
			var cols []string
			name := ""
			if p >= 0 && p < fi {
				name = unquoteIdent(strings.TrimSpace(rest[:p]))
				b, _ := parenBody(rest[p:])
				cols = identList(b)
			} else {
				name = unquoteIdent(strings.TrimSpace(rest[:fi]))
			}
			t := im.table(name)
			if !contains(order, name) {
				order = append(order, name)
			}
			if cols == nil {
				cols = t.cols
			}
			for _, line := range strings.Split(data, "\n") {
				if line == "" {
					continue
				}
				fs := Fields{}
				for i, v := range strings.Split(line, "\t") {
					val, ok := copyUnescape(v)
					if i < len(cols) {
						if !ok {
							val = ""
						}
						fs = append(fs, Field{cols[i], val})
					}
				}
				t.rows = append(t.rows, fs)
			}
		}
	}
	status := ""
	total := 0
	tm := NewOMap()
	for _, name := range order {
		t := im.tables[name]
		if tc, ok := tableCfg(r, name); ok && tc.Skip {
			continue
		}
		s, n := im.sqlTableRecords(rel, t, r, labels)
		status = worst(status, s)
		total += n
		d := NewOMap().Set("kind", "record").Set("key", strings.Join(t.pk, ","))
		if len(t.fks) > 0 {
			fk := []any{}
			for _, x := range t.fks {
				fk = append(fk, x.col+" -> "+x.refTable+"."+x.refCol)
			}
			d.Set("foreignKeys", fk)
		}
		tm.Set(name, d)
		ti := TableInfo{Name: name, File: rel, Columns: append([]string{}, t.cols...), Rows: len(t.rows)}
		for _, x := range t.fks {
			ti.ForeignKeys = append(ti.ForeignKeys, x.col+" -> "+x.refTable+"."+x.refCol)
		}
		im.report.Tables = append(im.report.Tables, ti)
	}
	im.mapping.Set(rel, tm)
	im.sqlForeignKeys(rel)
	if status == "" {
		status = "skipped"
	}
	return status, total, fmt.Sprintf("%d tables", len(order)), nil
}

// sqlTableRecords creates records for one table.
func (im *importer) sqlTableRecords(rel string, t *sqlTable, r *ImportRule, labels []string) (string, int) {
	if t.ids == nil {
		t.ids = map[string]string{}
	}
	tc, _ := tableCfg(r, t.name)
	kind, section := tc.Kind, tc.Section
	if kind == "" {
		kind = "record"
	}
	if _, ok := im.b.kindM[kind]; !ok {
		kind = "record"
	}
	if section == "" {
		if kind != "record" {
			section = im.b.kind(kind).Section
		} else {
			section = t.name
		}
	}
	key := tc.Key
	if key == "" && len(t.pk) > 0 {
		key = t.pk[0]
	}
	concepts := tc.Concepts
	if concepts == nil {
		concepts = lowCardinality(t, key)
	}
	tl := uniqSorted(append(append(append([]string{}, labels...), tc.Labels...), im.b.defaultLabels("", "", rel+"#"+t.name)...))
	lim := im.cfg.Import.Limits.Records
	if lim <= 0 {
		lim = 100000
	}
	im.startGroup(rel+"#"+t.name, t.name, t.rows, kind, section)
	defer func() { im.group = "" }()
	status := ""
	for i, fs := range t.rows {
		if i >= lim {
			break
		}
		loc := fmt.Sprint(i)
		if v, ok := fs.Get(key); ok && v != "" {
			loc = v
		}
		id, s := im.record(fmt.Sprintf("%s#%s/%s", rel, t.name, loc), kind, section, renderRecordText(tc.Text, fs), fs, concepts, tl, "", 0)
		if id == "" {
			continue
		}
		t.ids[loc] = id
		status = worst(status, s)
	}
	if status == "" {
		status = "skipped"
	}
	return status, len(t.rows)
}

// lowCardinality picks text columns with ≤ 30 distinct values, each used by ≥ 2 rows.
func lowCardinality(t *sqlTable, key string) []string {
	if len(t.rows) < 4 {
		return nil
	}
	fk := map[string]bool{}
	for _, f := range t.fks {
		fk[f.col] = true
	}
	out := []string{}
	for _, c := range t.cols {
		if c == key || fk[c] || strings.HasSuffix(c, "_id") || c == "id" {
			continue
		}
		cnt := map[string]int{}
		numeric := true
		for _, r := range t.rows {
			v, _ := r.Get(c)
			if v == "" {
				continue
			}
			cnt[v]++
			if _, ok := parseNum(v); !ok {
				numeric = false
			}
		}
		if numeric || len(cnt) < 2 || len(cnt) > 30 {
			continue
		}
		ok := true
		for _, n := range cnt {
			if n < 2 {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, c)
		}
	}
	return out
}

// sqlForeignKeys links records along foreign keys (assoc 0.7).
func (im *importer) sqlForeignKeys(rel string) {
	for _, t := range im.tables {
		for _, fk := range t.fks {
			ref := im.tables[fk.refTable]
			if ref == nil {
				continue
			}
			for i, fs := range t.rows {
				v, ok := fs.Get(fk.col)
				if !ok || v == "" {
					continue
				}
				key := fmt.Sprint(i)
				if len(t.pk) > 0 {
					if pv, ok := fs.Get(t.pk[0]); ok && pv != "" {
						key = pv
					}
				}
				a, b := t.ids[key], ref.ids[v]
				if a != "" && b != "" {
					im.b.addEdge(a, b, .7, EdgeAssoc)
				}
			}
		}
	}
}
