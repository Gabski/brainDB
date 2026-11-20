// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// Helpers that mirror JavaScript semantics used by the reference algorithm (UTF-16 lengths, toFixed rounding,
// number formatting, whitespace classes), so the Go port produces identical numbers and strings.

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"
)

func clamp(x float64) float64 { return clampRange(x, 0, 1) }

func clampRange(x, a, b float64) float64 { return math.Max(a, math.Min(b, x)) }

// mulberry32 is a port of the reference deterministic PRNG.
func mulberry32(seed uint32) func() float64 {
	a := seed
	return func() float64 {
		a += 0x6D2B79F5
		t := (a ^ a>>15) * (1 | a)
		t = (t + (t^t>>7)*(61|t)) ^ t
		return float64(t^t>>14) / 4294967296
	}
}

// hash is the reference FNV-1a: for each code point it xors the first UTF-16 unit.
func hash(s string) uint32 {
	h := uint32(2166136261)
	for _, r := range s {
		u := uint32(r)
		if r >= 0x10000 {
			u = uint32((r-0x10000)>>10) + 0xD800
		}
		h ^= u
		h *= 16777619
	}
	return h
}

// u16 returns the UTF-16 encoding of s (JS string units).
func u16(s string) []uint16 { return utf16.Encode([]rune(s)) }

func fromU16(u []uint16) string { return string(utf16.Decode(u)) }

// jsLen is String length.
func jsLen(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// short mirrors the reference short(): truncate to n UTF-16 units with an ellipsis.
func short(s string, n int) string {
	if jsLen(s) <= n {
		return s
	}
	return fromU16(u16(s)[:n-1]) + "…"
}

// isJSSpace matches the JavaScript \s class (and the set trimmed by String.trim).
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func jsTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

// collapseSpace is s.replace(/\s+/g, ' ').
func collapseSpace(s string) string {
	var b strings.Builder
	sp := false
	for _, r := range s {
		if isJSSpace(r) {
			if !sp {
				b.WriteByte(' ')
			}
			sp = true
			continue
		}
		sp = false
		b.WriteRune(r)
	}
	return b.String()
}

// flat is String(t).replace(/\s+/g, ' ').trim().
func flat(s string) string { return jsTrim(collapseSpace(s)) }

// toFixed mirrors Number.toFixed (ties round away from zero on the exact binary value).
func toFixed(x float64, digits int) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.IsInf(x, 0) {
		if x > 0 {
			return "Infinity"
		}
		return "-Infinity"
	}
	neg := x < 0
	ax := math.Abs(x)
	s := strconv.FormatFloat(ax, 'f', digits, 64)
	// detect an exact tie, which strconv rounds half-to-even but JS rounds up
	m := new(big.Float).SetPrec(256).SetFloat64(ax)
	m.Mul(m, new(big.Float).SetPrec(256).SetFloat64(math.Pow10(digits)))
	fl, _ := m.Int(nil)
	frac := new(big.Float).SetPrec(256).Sub(m, new(big.Float).SetPrec(256).SetInt(fl))
	if frac.Cmp(big.NewFloat(0.5)) == 0 {
		fl.Add(fl, big.NewInt(1))
		ds := fl.String()
		if digits > 0 {
			for len(ds) <= digits {
				ds = "0" + ds
			}
			s = ds[:len(ds)-digits] + "." + ds[len(ds)-digits:]
		} else {
			s = ds
		}
	}
	if neg && strings.Trim(s, "0.") != "" {
		return "-" + s
	}
	return s
}

// round2 is +x.toFixed(2).
func round2(x float64) float64 {
	v, _ := strconv.ParseFloat(toFixed(x, 2), 64)
	return v
}

func roundN(x float64, n int) float64 {
	v, _ := strconv.ParseFloat(toFixed(x, n), 64)
	return v
}

// fmtNum mirrors `${x}` for numbers in the usual range.
func fmtNum(x float64) string {
	if math.IsInf(x, 1) {
		return "Infinity"
	}
	if x == 0 {
		return "0"
	}
	return strconv.FormatFloat(x, 'f', -1, 64)
}

func f2(x float64) string { return toFixed(x, 2) }

// jsNumber mirrors Number(s) for the cases that matter (trimmed decimal; "" is 0; NaN otherwise).
func jsNumber(s string) (float64, bool) {
	s = jsTrim(s)
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN(), false
	}
	return v, true
}

// deacc removes diacritics like the reference algorithm: NFD, drop U+0300..U+036F, ł → l.
func deacc(s string) string {
	var b strings.Builder
	for _, r := range s {
		if m, ok := deaccTable[r]; ok {
			b.WriteString(m)
			continue
		}
		if r >= 0x300 && r <= 0x36f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// words splits lowercased, deaccented text on [^a-z0-9]+.
func asciiWords(s string) []string {
	t := deacc(strings.ToLower(s))
	return strings.FieldsFunc(t, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
}

// stems mirrors the reference stems(): 5-letter prefixes of words with ≥ 3 letters.
func stems(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range asciiWords(s) {
		if len(w) >= 3 {
			if len(w) > 5 {
				w = w[:5]
			}
			out[w] = true
		}
	}
	return out
}

func slug(s string, sep string) string {
	t := deacc(strings.ToLower(s))
	var b strings.Builder
	pend := false
	for _, r := range t {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if pend && b.Len() > 0 {
				b.WriteString(sep)
			}
			pend = false
			b.WriteRune(r)
		} else {
			pend = true
		}
	}
	return b.String()
}
