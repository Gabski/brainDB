// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Package bdql implements BDQL, the brainDB Query Language: a hand-written lexer, a recursive-descent parser
// producing an AST, and an executor that maps statements onto brain operations inside script transactions.
package bdql

import (
	"fmt"
	"strings"
	"unicode"
)

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tString
	tNumber
	tVar
	tOp
	tPunct
)

func (k tokKind) String() string {
	return [...]string{"end of input", "word", "string", "number", "variable", "operator", "punctuation"}[k]
}

type token struct {
	kind      tokKind
	text      string // raw text (identifier, operator) or decoded string
	off, end  int    // byte offsets in the source
	line, col int
}

// Error is a positioned BDQL error.
type Error struct {
	Stmt, Line, Col int
	Msg             string
	Excerpt         string
}

func (e *Error) Error() string {
	s := ""
	if e.Stmt > 0 {
		s = fmt.Sprintf("statement %d, ", e.Stmt)
	}
	s += fmt.Sprintf("line %d, col %d: %s", e.Line, e.Col, e.Msg)
	if e.Excerpt != "" {
		s += "\n" + e.Excerpt
	}
	return s
}

func excerpt(src string, line, col int) string {
	ls := strings.Split(src, "\n")
	if line < 1 || line > len(ls) {
		return ""
	}
	l := ls[line-1]
	if col < 1 {
		col = 1
	}
	pad := strings.Repeat(" ", len([]rune(safePrefix(l, col-1))))
	return "  " + l + "\n  " + pad + "^"
}

func safePrefix(s string, n int) string {
	if n > len(s) {
		return s
	}
	return s[:n]
}

type lexer struct {
	src       string
	i         int
	line, col int
	toks      []token
}

func isIdentStart(r byte) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= 0x80
}

func isIdentChar(r byte) bool { return isIdentStart(r) || r >= '0' && r <= '9' }

func lex(src string) ([]token, error) {
	l := &lexer{src: src, line: 1, col: 1}
	for {
		l.skip()
		if l.i >= len(l.src) {
			l.toks = append(l.toks, token{kind: tEOF, off: l.i, end: l.i, line: l.line, col: l.col})
			return l.toks, nil
		}
		if err := l.next(); err != nil {
			return nil, err
		}
	}
}

func (l *lexer) adv(n int) {
	for k := 0; k < n && l.i < len(l.src); k++ {
		if l.src[l.i] == '\n' {
			l.line++
			l.col = 1
		} else if l.src[l.i]&0xC0 != 0x80 {
			l.col++
		}
		l.i++
	}
}

func (l *lexer) skip() {
	for l.i < len(l.src) {
		c := l.src[l.i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			l.adv(1)
		case c == '-' && l.i+1 < len(l.src) && l.src[l.i+1] == '-':
			for l.i < len(l.src) && l.src[l.i] != '\n' {
				l.adv(1)
			}
		default:
			return
		}
	}
}

func (l *lexer) fail(msg string) error {
	return &Error{Line: l.line, Col: l.col, Msg: msg, Excerpt: excerpt(l.src, l.line, l.col)}
}

func (l *lexer) emit(k tokKind, text string, start, line, col int) {
	l.toks = append(l.toks, token{kind: k, text: text, off: start, end: l.i, line: line, col: col})
}

func (l *lexer) prevAllowsNegative() bool {
	if len(l.toks) == 0 {
		return true
	}
	p := l.toks[len(l.toks)-1]
	switch p.kind {
	case tOp:
		return true
	case tPunct:
		return p.text != ")"
	case tIdent:
		u := strings.ToUpper(p.text)
		return keywords[u]
	}
	return false
}

func (l *lexer) next() error {
	start, line, col := l.i, l.line, l.col
	c := l.src[l.i]
	switch {
	case c == '$' && l.i+1 < len(l.src) && l.src[l.i+1] == '$':
		end := strings.Index(l.src[l.i+2:], "$$")
		if end < 0 {
			return l.fail("unterminated $$ string")
		}
		body := l.src[l.i+2 : l.i+2+end]
		l.adv(end + 4)
		l.emit(tString, body, start, line, col)
	case c == '$':
		l.adv(1)
		j := l.i
		for j < len(l.src) && isIdentChar(l.src[j]) {
			j++
		}
		if j == l.i {
			return l.fail("expected a variable name after $")
		}
		name := l.src[l.i:j]
		l.adv(j - l.i)
		l.emit(tVar, name, start, line, col)
	case c == '\'' || c == '"':
		q := c
		l.adv(1)
		var b strings.Builder
		for {
			if l.i >= len(l.src) {
				return &Error{Line: line, Col: col, Msg: "unterminated string", Excerpt: excerpt(l.src, line, col)}
			}
			ch := l.src[l.i]
			if ch == '\\' && l.i+1 < len(l.src) {
				n := l.src[l.i+1]
				switch n {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case '\\', '\'', '"':
					b.WriteByte(n)
				default:
					// keep unknown escapes verbatim (regexes such as '\bSKU-\d{5}\b')
					b.WriteByte('\\')
					b.WriteByte(n)
				}
				l.adv(2)
				continue
			}
			if ch == q {
				if l.i+1 < len(l.src) && l.src[l.i+1] == q {
					b.WriteByte(q)
					l.adv(2)
					continue
				}
				l.adv(1)
				break
			}
			b.WriteByte(ch)
			l.adv(1)
		}
		l.emit(tString, b.String(), start, line, col)
	case c >= '0' && c <= '9' || (c == '-' || c == '+') && l.i+1 < len(l.src) && l.src[l.i+1] >= '0' && l.src[l.i+1] <= '9' && l.prevAllowsNegative():
		j := l.i + 1
		for j < len(l.src) && (l.src[j] >= '0' && l.src[j] <= '9' || l.src[j] == '.' && j+1 < len(l.src) && l.src[j+1] >= '0' && l.src[j+1] <= '9') {
			j++
		}
		// a number glued to letters is an id-like word (e.g. 2026q4) unless it is a unit suffix
		k := j
		for k < len(l.src) && (isIdentChar(l.src[k]) || l.src[k] == '%') {
			k++
		}
		txt := l.src[l.i:k]
		l.adv(k - l.i)
		if k > j && !numberSuffix(l.src[j:k]) {
			l.emit(tIdent, txt, start, line, col)
		} else {
			l.emit(tNumber, txt, start, line, col)
		}
	case isIdentStart(c):
		j := l.i
		for j < len(l.src) {
			ch := l.src[j]
			if isIdentChar(ch) {
				j++
				continue
			}
			if j+1 < len(l.src) {
				nx := l.src[j+1]
				switch ch {
				case ':':
					if isIdentChar(nx) {
						j++
						continue
					}
				case '-':
					if isIdentChar(nx) && nx != '-' {
						j++
						continue
					}
				case '.', '@':
					if isIdentStart(nx) || nx == '*' {
						j++
						continue
					}
				case '*':
				}
			}
			if ch == '*' && j > 0 && (l.src[j-1] == '.' || l.src[j-1] == '@') {
				j++
				continue
			}
			break
		}
		txt := l.src[l.i:j]
		l.adv(j - l.i)
		l.emit(tIdent, txt, start, line, col)
	default:
		two := ""
		if l.i+1 < len(l.src) {
			two = l.src[l.i : l.i+2]
		}
		switch two {
		case ">=", "<=", "!=", "<>":
			l.adv(2)
			l.emit(tOp, two, start, line, col)
			return nil
		}
		switch c {
		case '=', '>', '<', '~':
			l.adv(1)
			l.emit(tOp, string(c), start, line, col)
		case '(', ')', ',', ';', '*':
			l.adv(1)
			l.emit(tPunct, string(c), start, line, col)
		default:
			r := []rune(l.src[l.i:])[0]
			if unicode.IsSpace(r) {
				l.adv(len(string(r)))
				return nil
			}
			return l.fail(fmt.Sprintf("unexpected character %q", r))
		}
	}
	return nil
}

func numberSuffix(s string) bool {
	switch strings.ToLower(s) {
	case "%", "k", "m", "bn":
		return true
	}
	return false
}
