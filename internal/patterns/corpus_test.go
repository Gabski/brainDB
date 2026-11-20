// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package patterns

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// corpus: per type, positive texts with the expected normalised value, and hard negatives that must not match.
type sample struct {
	text, want string
}

var corpus = map[string]struct {
	pos []sample
	neg []string
}{
	"date": {pos: []sample{{"due 2026-12-15", "2026-12-15"}, {"on 15.12.2026 we", "2026-12-15"}, {"15 Dec 2026", "2026-12-15"}, {"December 15, 2026", "2026-12-15"},
		{"15 grudnia 2026", "2026-12-15"}, {"Q4 2026", "2026-Q4"}, {"week 2026-W47", "2026-W47"}, {"1st March 2027", "2027-03-01"}, {"3. März 2027", "2027-03-03"},
		{"12 octobre 2026", "2026-10-12"}, {"5 de mayo 2026", ""}, {"20 Nov 2026", "2026-11-20"}, {"Nov 20, 2026", "2026-11-20"}, {"2027-01-31", "2027-01-31"},
		{"31.01.2027", "2027-01-31"}, {"7 januar 2027", "2027-01-07"}, {"Q1 2027", "2027-Q1"}, {"28 Feb 2026", "2026-02-28"}, {"01/31/2027", "2027-01-31"}, {"9 September 2026", "2026-09-09"}},
		neg: []string{"version 3.11.174", "2026-13-45", "31.02.2026", "order 1234-56", "build 2026.1", "price 15.12", "call 600-100-200", "v1.2.0", "30.02.2026", "PO 12/34"}},
	"money": {pos: []sample{{"€48,000", "EUR 48000.00"}, {"48 000 EUR", "EUR 48000.00"}, {"$12.5k", "USD 12500.00"}, {"PLN 1 200,00", "PLN 1200.00"}, {"USD 3.2M", "USD 3200000.00"},
		{"12,50 zł", "PLN 12.50"}, {"€1,395,000 TCV", "EUR 1395000.00"}, {"costs 900 EUR", "EUR 900.00"}, {"£250", "GBP 250.00"}, {"NOK 4 500", "NOK 4500.00"},
		{"€189 per pack", "EUR 189.00"}, {"46 200,00 zł", "PLN 46200.00"}, {"USD 3.2k", "USD 3200.00"}, {"€14", "EUR 14.00"}, {"CHF 99", "CHF 99.00"},
		{"$1,150", "USD 1150.00"}, {"3000 USD", "USD 3000.00"}, {"€9,500 one-off", "EUR 9500.00"}, {"SEK 120", "SEK 120.00"}, {"€0.5k", "EUR 500.00"}},
		neg: []string{"48000 units", "value 12500", "3,000 cycles", "price 15.12", "version 1.2", "page 12", "room 101", "12% off", "4.5 V", "2026"}},
	"percent": {pos: []sample{{"12%", "0.12"}, {"12.5 %", "0.125"}, {"6% discount", "0.06"}, {"12 percent", "0.12"}, {"91.4%", "0.914"}, {"80 % capacity", "0.8"}, {"0.5%", "0.005"}, {"100%", "1"}, {"41% margin", "0.41"}, {"3 procent", "0.03"}},
		neg: []string{"12 units", "percentage", "0.12", "1.2.3", "ratio 3:1"}},
	"email": {pos: []sample{{"kim.aalto@halvorsen.example", "kim.aalto@halvorsen.example"}, {"Mail ELIN.Dahl@MarrowCreek.example now", "elin.dahl@marrowcreek.example"}, {"a+b@x.io", "a+b@x.io"}, {"x_y@sub.domain.org", "x_y@sub.domain.org"}, {"<ops@lab.example>", "ops@lab.example"}},
		neg: []string{"user@", "@kim", "a@b", "kim at halvorsen", "x@y."}},
	"phone": {pos: []sample{{"+47 22 33 44 55", "+4722334455"}, {"(555) 010-2030", "+15550102030"}, {"+48 600 100 200", "+48600100200"}, {"+49 40 3070 5521", "+494030705521"}, {"call 0047 22 33 44 55", "+4722334455"}},
		neg: []string{"2026-12-15", "INV-2026-0042", "version 3.11.174", "order 1234", "10.0.0.1", "4111 1111 1111 1111", "12500"}},
	"iban": {pos: []sample{{"NO93 8601 1117 947", "NO9386011117947"}, {"DE89 3704 0044 0532 0130 00", "DE89370400440532013000"}, {"GB29NWBK60161331926819", "GB29NWBK60161331926819"}},
		neg: []string{"NO93 8601 1117 948", "DE00 1234 5678 9012 3456 78", "AB12 3456"}},
	"card": {pos: []sample{{"4111 1111 1111 1111", "**** 1111"}, {"5500-0000-0000-0004", "**** 0004"}, {"340000000000009", "**** 0009"}},
		neg: []string{"4111 1111 1111 1112", "1234 5678 9012 3456", "+47 22 33 44 55"}},
	"url": {pos: []sample{{"https://halvorsen.example/pricing", "https://halvorsen.example/pricing"}, {"see www.example.org.", "https://www.example.org"}, {"example.org/docs", "https://example.org/docs"}},
		neg: []string{"file.txt", "v1.2.0", "e.g. this"}},
	"version":     {pos: []sample{{"v1.2.0", "1.2.0"}, {"3.11.174", "3.11.174"}, {"protocol v3", "3"}}, neg: []string{"15.12.2026", "10.0.0.1"}},
	"id_code":     {pos: []sample{{"INV-2026-0042", "INV-2026-0042"}, {"SKU-10422", "SKU-10422"}, {"ticket #4821", "#4821"}, {"R-114", "R-114"}}, neg: []string{"COVID", "#1", "x-12"}},
	"org":         {pos: []sample{{"Halvorsen Freight AS signed", "Halvorsen Freight"}, {"Acme GmbH", "Acme"}, {"Brightline Supply Co. ordered", "Brightline Supply"}, {"Nordic Labs Ltd", "Nordic Labs"}}, neg: []string{"as soon as", "The AS"}},
	"person":      {pos: []sample{{"Call Kim Aalto today", "Kim Aalto"}, {"Dr. Ana Petrov", "Ana Petrov"}, {"met Jan Kowalski", "Jan Kowalski"}}, neg: []string{"Microsoft Entra", "The Plan", "Q4 Review"}},
	"vat":         {pos: []sample{{"PL5260250274", "PL5260250274"}, {"NO 974760673 MVA", "NO974760673MVA"}}, neg: []string{"PL1234567890", "NO 123456789 MVA"}},
	"ip":          {pos: []sample{{"host 10.0.0.1 down", "10.0.0.1"}, {"2001:db8::1", "2001:db8::1"}}, neg: []string{"999.1.1.1", "1.2.3"}},
	"duration":    {pos: []sample{{"18 months", "P18M"}, {"90 days", "P90D"}, {"2h 30m", "PT2H30M"}, {"P18M", "P18M"}}, neg: []string{"month", "2 apples"}},
	"number_unit": {pos: []sample{{"4.5 V", "4.5 V"}, {"25 °C", "25 °C"}, {"3.2 mS/cm", "3.2 mS/cm"}, {"500 rps", "500 rps"}, {"12 kg", "12 kg"}}, neg: []string{"12 kgb", "v2"}},
}

func TestCorpusPrecisionRecall(t *testing.T) {
	o := &Options{Locales: []string{"en", "pl", "de", "fr", "es", "nb"}, DateOrder: "", DefaultRegion: ""}
	types := []string{}
	for k := range corpus {
		types = append(types, k)
	}
	sort.Strings(types)
	report := []string{}
	for _, typ := range types {
		c := corpus[typ]
		tp, fn, fp := 0, 0, 0
		for _, s := range c.pos {
			es := o.ExtractTypes(s.text, []string{typ})
			hit := false
			for _, e := range es {
				if s.want == "" || e.Value == s.want {
					hit = true
				}
			}
			if hit {
				tp++
			} else {
				fn++
				t.Logf("%s: missed %q (want %q, got %v)", typ, s.text, s.want, values(es))
			}
			for _, e := range es {
				if s.want != "" && e.Value != s.want {
					fp++
					t.Logf("%s: wrong value in %q: %q", typ, s.text, e.Value)
				}
			}
		}
		for _, n := range c.neg {
			if es := o.ExtractTypes(n, []string{typ}); len(es) > 0 {
				fp += len(es)
				t.Logf("%s: false positive in %q: %v", typ, n, values(es))
			}
		}
		prec := 1.0
		if tp+fp > 0 {
			prec = float64(tp) / float64(tp+fp)
		}
		rec := float64(tp) / float64(tp+fn)
		report = append(report, fmt.Sprintf("%-12s precision %.2f recall %.2f (%d positives, %d negatives)", typ, prec, rec, len(c.pos), len(c.neg)))
		if prec < .95 || rec < .85 {
			t.Errorf("%s: precision %.2f recall %.2f below targets 0.95 / 0.85", typ, prec, rec)
		}
	}
	t.Log("\n" + strings.Join(report, "\n"))
}

func values(es []Entity) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.Value)
	}
	return out
}

func TestValidators(t *testing.T) {
	if !Luhn("4111111111111111") || Luhn("4111111111111112") {
		t.Error("Luhn")
	}
	if !ValidIBAN("NO9386011117947") || ValidIBAN("NO9386011117948") {
		t.Error("mod-97")
	}
	if validDate(2026, 2, 30) || !validDate(2028, 2, 29) {
		t.Error("calendar")
	}
	if v, c, ok := ParseMoney("€48,000"); !ok || v != 48000 || c != "EUR" {
		t.Error("ParseMoney")
	}
	if d, ok := NormalizeDate("15 grudnia 2026"); !ok || d != "2026-12-15" {
		t.Error("NormalizeDate pl")
	}
	o := &Options{FxBase: "EUR", FxRates: map[string]float64{"USD": .92}}
	if c, ok := o.CompareValues("money", "USD 1000.00", "EUR 900.00"); !ok || c != 1 {
		t.Error("fx comparison")
	}
	if _, ok := (&Options{}).CompareValues("money", "USD 1000.00", "EUR 900.00"); ok {
		t.Error("cross-currency without fx must be incomparable")
	}
}

func TestMaskAndCustom(t *testing.T) {
	o := &Options{}
	if got := o.MaskText("pay 4111 1111 1111 1111 or call +47 22 33 44 55", []string{"phone"}); strings.Contains(got, "4111 1111 1111 1111") || strings.Contains(got, "22 33 44 55") {
		t.Errorf("mask: %s", got)
	}
	c := &Custom{Name: "sku", Type: "id_code", Regex: `\bSKU-\d{5}\b`, Normalize: "upper"}
	if err := c.Compile(); err != nil {
		t.Fatal(err)
	}
	o.Custom = []*Custom{c}
	es := o.ExtractTypes("Order SKU-10422 and SKU-1", []string{"id_code"})
	if len(es) != 1 || es[0].Pattern != "sku" {
		t.Fatalf("custom pattern replaces the built-in match: %+v", es)
	}
	if err := (&Custom{Name: "bad", Regex: "(?<=x)"}).Compile(); err == nil {
		t.Error("invalid regex must fail")
	}
}
