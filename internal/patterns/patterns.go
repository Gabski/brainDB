// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Package patterns is brainDB's built-in entity library: RE2 regexps plus validators and normalisers for dates,
// money, percentages, units, e-mails, phones, URLs, people, organisations, IBANs, VAT ids, cards, IPs, versions,
// id codes, hashtags, mentions, coordinates and addresses. Entities are derived data, recomputed from text.
package patterns

import (
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Entity is one recognised value.
type Entity struct {
	Type       string  `json:"type"`
	Value      string  `json:"value"`
	Raw        string  `json:"raw"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Confidence float64 `json:"confidence"`
	PII        bool    `json:"pii"`
	Amount     float64 `json:"amount,omitempty"`
	Currency   string  `json:"currency,omitempty"`
	Unit       string  `json:"unit,omitempty"`
	Pattern    string  `json:"pattern,omitempty"`
}

// Custom is a user-defined pattern.
type Custom struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Regex     string `json:"regex"`
	Normalize string `json:"normalize,omitempty"` // upper | lower | none
	PII       bool   `json:"pii,omitempty"`
	re        *regexp.Regexp
}

// Options configure extraction.
type Options struct {
	Locales       []string           `json:"locales"`
	DefaultRegion string             `json:"defaultRegion"`
	DateOrder     string             `json:"dateOrder"` // dmy | mdy | ymd
	Disabled      []string           `json:"disabled"`
	Custom        []*Custom          `json:"custom"`
	FxBase        string             `json:"-"`
	FxRates       map[string]float64 `json:"-"`
}

// MaxMatches limits matches per pattern per text.
const MaxMatches = 1000

// Info describes a pattern type. Name and Enabled are always serialised: the UI keys its rows on the
// name and binds its checkbox to Enabled, so an omitted name collapses every built-in row onto the
// same key and an omitted flag renders every active pattern as switched off.
type Info struct {
	Type    string `json:"type"`
	PII     bool   `json:"pii"`
	Builtin bool   `json:"builtin"`
	Enabled bool   `json:"enabled"`
	Regex   string `json:"regex,omitempty"`
	Example string `json:"example"`
	Name    string `json:"name"`
}

var piiTypes = map[string]bool{"email": true, "phone": true, "person": true, "address": true, "iban": true, "vat": true, "card": true, "ip": true}

// IsPII reports whether a type is personal data.
func IsPII(t string) bool { return piiTypes[t] }

// Builtins lists the built-in types in extraction order.
var Builtins = []Info{
	{Type: "datetime", Example: "2026-11-20 09:30"}, {Type: "date", Example: "15 Dec 2026"}, {Type: "duration", Example: "18 months"},
	{Type: "money", Example: "€48,000"}, {Type: "percent", Example: "12.5 %"}, {Type: "number_unit", Example: "4.5 V"},
	{Type: "email", PII: true, Example: "kim.aalto@halvorsen.example"}, {Type: "phone", PII: true, Example: "+47 22 33 44 55"},
	{Type: "url", Example: "https://halvorsen.example/pricing"}, {Type: "domain", Example: "halvorsen.example"},
	{Type: "person", PII: true, Example: "Kim Aalto"}, {Type: "org", Example: "Halvorsen Freight AS"},
	{Type: "iban", PII: true, Example: "NO93 8601 1117 947"}, {Type: "vat", PII: true, Example: "PL5260250274"},
	{Type: "card", PII: true, Example: "4111 1111 1111 1111"}, {Type: "ip", PII: true, Example: "10.0.0.1"},
	{Type: "version", Example: "v1.2.0"}, {Type: "id_code", Example: "INV-2026-0042"}, {Type: "hashtag", Example: "#renewal"},
	{Type: "mention", Example: "@kim"}, {Type: "geo", Example: "59.91, 10.75"}, {Type: "address", PII: true, Example: "Karl Johans gate 22, 0159 Oslo"},
}

func init() {
	for i := range Builtins {
		Builtins[i].Builtin = true
		Builtins[i].Name = Builtins[i].Type // built-ins are addressed by their type name
	}
}

// List returns built-in and custom pattern descriptions.
func (o *Options) List() []Info {
	out := append([]Info(nil), Builtins...)
	for i := range out {
		out[i].Enabled = !o.disabled(out[i].Name)
	}
	for _, c := range o.customs() {
		out = append(out, Info{Type: c.Type, Name: c.Name, PII: c.PII, Regex: c.Regex, Enabled: !o.disabled(c.Name)})
	}
	return out
}

func (o *Options) customs() []*Custom {
	if o == nil {
		return nil
	}
	return o.Custom
}

// Compile validates a custom pattern.
func (c *Custom) Compile() error {
	if c.Name == "" {
		return fmt.Errorf("pattern needs a name")
	}
	if c.Type == "" {
		c.Type = c.Name
	}
	re, err := regexp.Compile(c.Regex)
	if err != nil {
		return fmt.Errorf("pattern %s: %v", c.Name, err)
	}
	c.re = re
	return nil
}

func (o *Options) disabled(t string) bool {
	if o == nil {
		return false
	}
	for _, d := range o.Disabled {
		if d == t {
			return true
		}
	}
	return false
}

func (o *Options) hasLocale(l string) bool {
	if o == nil || len(o.Locales) == 0 {
		return l == "en"
	}
	for _, x := range o.Locales {
		if x == l {
			return true
		}
	}
	return false
}

func (o *Options) nonEnglish() bool {
	if o == nil {
		return false
	}
	for _, x := range o.Locales {
		if x != "en" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- numbers

const numRe = `\d{1,3}(?:[ \x{a0}\x{202f},.']\d{3})+(?:[.,]\d{1,2})?|\d+(?:[.,]\d+)?`

// ParseAmount parses a localised number ("48 000", "1.200,50", "12,5", "$12.5k").
func ParseAmount(s string, preferDecimalComma bool) (float64, bool) {
	t := strings.TrimSpace(s)
	mult := 1.0
	low := strings.ToLower(t)
	for suf, m := range map[string]float64{"bn": 1e9, "k": 1e3, "m": 1e6} {
		if strings.HasSuffix(low, suf) {
			mult = m
			t = strings.TrimSpace(t[:len(t)-len(suf)])
			break
		}
	}
	t = strings.NewReplacer(" ", "", " ", "", " ", "", "'", "").Replace(t)
	if t == "" {
		return 0, false
	}
	lc, ld := strings.LastIndex(t, ","), strings.LastIndex(t, ".")
	switch {
	case lc >= 0 && ld >= 0:
		if lc > ld {
			t = strings.ReplaceAll(t, ".", "")
			t = strings.Replace(t, ",", ".", 1)
		} else {
			t = strings.ReplaceAll(t, ",", "")
		}
	case lc >= 0:
		if strings.Count(t, ",") > 1 || (len(t)-lc-1 == 3 && !preferDecimalComma) {
			t = strings.ReplaceAll(t, ",", "")
		} else {
			t = strings.Replace(t, ",", ".", 1)
		}
	case ld >= 0:
		if strings.Count(t, ".") > 1 || (len(t)-ld-1 == 3 && preferDecimalComma) {
			t = strings.ReplaceAll(t, ".", "")
		}
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return v * mult, true
}

// ---------------------------------------------------------------- money

// Currencies is the ISO 4217 subset recognised.
var Currencies = map[string]bool{"EUR": true, "USD": true, "GBP": true, "PLN": true, "NOK": true, "SEK": true, "DKK": true, "CHF": true,
	"JPY": true, "CNY": true, "CAD": true, "AUD": true, "NZD": true, "CZK": true, "HUF": true, "INR": true, "BRL": true, "MXN": true,
	"ZAR": true, "SGD": true, "HKD": true, "KRW": true, "TRY": true, "ISK": true, "RON": true, "BGN": true, "UAH": true}

var curSym = map[string]string{"€": "EUR", "$": "USD", "£": "GBP", "¥": "JPY", "zł": "PLN", "zl": "PLN", "kr": "NOK", "₹": "INR", "US$": "USD",
	"euro": "EUR", "euros": "EUR", "dollars": "USD", "dollar": "USD", "złotych": "PLN", "zloty": "PLN", "kroner": "NOK", "kronor": "SEK"}

var (
	curCodes  = `EUR|USD|GBP|PLN|NOK|SEK|DKK|CHF|JPY|CNY|CAD|AUD|NZD|CZK|HUF|INR|BRL|MXN|ZAR|SGD|HKD|KRW|TRY|ISK|RON|BGN|UAH`
	moneyPre  = regexp.MustCompile(`(?:US\$|[$€£¥₹]|\b(?:` + curCodes + `))\s?(` + numRe + `)(?:\s?(k|m|bn|M|K)\b)?`)
	moneyPost = regexp.MustCompile(`(` + numRe + `)(?:\s?(k|m|bn|M|K))?\s?(€|zł|\b(?:` + curCodes + `|kr|euros?|dollars?|złotych|zloty|kroner|kronor)\b)`)
)

func normCurrency(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	if Currencies[u] {
		return u
	}
	if c, ok := curSym[strings.ToLower(strings.TrimSpace(s))]; ok {
		return c
	}
	if c, ok := curSym[strings.TrimSpace(s)]; ok {
		return c
	}
	return ""
}

// FormatMoney renders a normalised money value.
func FormatMoney(amount float64, cur string) string {
	return cur + " " + strconv.FormatFloat(amount, 'f', 2, 64)
}

// ParseMoney parses "EUR 48000.00" (or any money text) into amount and currency.
func ParseMoney(s string) (float64, string, bool) {
	es := (&Options{}).ExtractTypes(s, []string{"money"})
	if len(es) == 0 {
		return 0, "", false
	}
	return es[0].Amount, es[0].Currency, true
}

// ---------------------------------------------------------------- dates

var monthNames = map[string]map[string]int{
	"en": {"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6, "july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "sept": 9, "oct": 10, "nov": 11, "dec": 12},
	"pl": {"stycznia": 1, "lutego": 2, "marca": 3, "kwietnia": 4, "maja": 5, "czerwca": 6, "lipca": 7, "sierpnia": 8, "września": 9, "października": 10, "listopada": 11, "grudnia": 12,
		"styczeń": 1, "luty": 2, "marzec": 3, "kwiecień": 4, "maj": 5, "czerwiec": 6, "lipiec": 7, "sierpień": 8, "wrzesień": 9, "październik": 10, "listopad": 11, "grudzień": 12},
	"de": {"januar": 1, "februar": 2, "märz": 3, "april": 4, "mai": 5, "juni": 6, "juli": 7, "august": 8, "september": 9, "oktober": 10, "november": 11, "dezember": 12},
	"fr": {"janvier": 1, "février": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6, "juillet": 7, "août": 8, "septembre": 9, "octobre": 10, "novembre": 11, "décembre": 12},
	"es": {"enero": 1, "febrero": 2, "marzo": 3, "abril": 4, "mayo": 5, "junio": 6, "julio": 7, "agosto": 8, "septiembre": 9, "octubre": 10, "noviembre": 11, "diciembre": 12},
	"nb": {"januar": 1, "februar": 2, "mars": 3, "april": 4, "mai": 5, "juni": 6, "juli": 7, "august": 8, "september": 9, "oktober": 10, "november": 11, "desember": 12},
}

func (o *Options) months() map[string]int {
	m := map[string]int{}
	for l, names := range monthNames {
		if o.hasLocale(l) || l == "en" {
			for k, v := range names {
				m[k] = v
			}
		}
	}
	return m
}

var (
	reISODate  = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	reNumDate  = regexp.MustCompile(`\b(\d{1,2})([./])(\d{1,2})([./])(\d{4})\b`)
	reQuarter  = regexp.MustCompile(`\bQ([1-4])\s?(\d{4})\b`)
	reWeek     = regexp.MustCompile(`\b(\d{4})-W(\d{2})\b`)
	reDateTime = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})[T ](\d{2}):(\d{2})(?::(\d{2}))?(?:\.\d+)?(Z|[+-]\d{2}:?\d{2})?`)
	reWord     = `[\p{L}]+`
	reMonDMY   = regexp.MustCompile(`(?i)\b(\d{1,2})(?:st|nd|rd|th|\.)?\s+(?:de\s+)?(` + reWord + `)\.?,?\s+(?:de\s+)?(\d{4})\b`)
	reMonMDY   = regexp.MustCompile(`(?i)\b(` + reWord + `)\.?\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})\b`)
)

func validDate(y, m, d int) bool {
	if y < 1000 || y > 2999 || m < 1 || m > 12 || d < 1 {
		return false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Day() == d && int(t.Month()) == m
}

func isoDate(y, m, d int) string { return fmt.Sprintf("%04d-%02d-%02d", y, m, d) }

// NormalizeDate returns ISO form of a date written in any supported way.
func NormalizeDate(s string) (string, bool) {
	es := (&Options{Locales: []string{"en", "pl", "de", "fr", "es", "nb"}}).ExtractTypes(s, []string{"datetime", "date"})
	for _, e := range es {
		if e.Type == "date" {
			return e.Value, true
		}
		if e.Type == "datetime" {
			return e.Value[:10], true
		}
	}
	return "", false
}

// ---------------------------------------------------------------- other regexps

var (
	reEmail   = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9._%+-]*@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*\.[a-z]{2,24}\b`)
	reURL     = regexp.MustCompile(`(?i)\b(?:https?://[^\s<>"'\x60]+|www\.[a-z0-9-]+(?:\.[a-z0-9-]+)+[^\s<>"'\x60]*)`)
	tlds      = `com|org|net|io|ai|dev|app|edu|gov|info|biz|co|eu|example|test|no|pl|de|fr|es|se|dk|fi|uk|nl|be|ch|at|it|cz|us|ca|au|jp|cn|in|br`
	reDomain  = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:` + tlds + `)\b(?:/[^\s<>"']*)?`)
	reDur     = regexp.MustCompile(`(?i)\b(\d+(?:[.,]\d+)?)\s?(years?|yrs?|months?|mos?|weeks?|wks?|days?|hours?|hrs?|minutes?|mins?|seconds?|secs?)\b|\b(\d+)(h|m|s|d|w)\b`)
	reDurISO  = regexp.MustCompile(`\bP(?:\d+Y)?(?:\d+M)?(?:\d+W)?(?:\d+D)?(?:T(?:\d+H)?(?:\d+M)?(?:\d+(?:\.\d+)?S)?)?\b`)
	rePercent = regexp.MustCompile(`(?i)(-?(?:` + numRe + `))\s?(%|percent\b|per cent\b|procent\b|prozent\b|pourcent\b|por ciento\b|prosent\b)`)
	units     = `mS/cm|µS/cm|uS/cm|S/m|kWh|MWh|Wh|mAh|Ah|kV|mV|V|mA|A|MW|kW|W|°C|°F|K|rps|rpm|GHz|MHz|kHz|Hz|kg|mg|g|t|km|cm|mm|µm|nm|m|ml|mL|l|L|MPa|kPa|Pa|bar|psi|TB|GB|MB|KB|ms|µs|ns|dB`
	reUnit    = regexp.MustCompile(`(-?\d+(?:[.,]\d+)?)\s?(` + units + `)(?:\b|$|[^\p{L}\d])`)
	rePhone   = regexp.MustCompile(`(?:(?:\+|\b00)\d{1,3}[\s.-]?)?(?:\(\d{1,4}\)[\s.-]?)?\d{2,4}(?:[\s.-]\d{2,4}){1,4}`)
	reIBAN    = regexp.MustCompile(`\b[A-Z]{2}\d{2}(?:\s?[A-Z0-9]{4}){2,7}(?:\s?[A-Z0-9]{1,4})?\b`)
	reCard    = regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`)
	reIPv4    = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reIPv6    = regexp.MustCompile(`(?i)(?:^|[^0-9a-z:])([0-9a-f]{0,4}(?::[0-9a-f]{0,4}){2,7})`)
	reVersion = regexp.MustCompile(`\b[vV](\d+(?:\.\d+){0,3}(?:-[0-9A-Za-z.]+)?)\b|\b(\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?)\b`)
	reIDCode  = regexp.MustCompile(`\b[A-Z][A-Z0-9]{0,5}-\d{2,}(?:-\d+)*\b|(?:^|[\s(])(#\d{3,})\b`)
	reHashtag = regexp.MustCompile(`(?:^|[\s(])#([\p{L}][\p{L}\d_-]+)`)
	reMention = regexp.MustCompile(`(?:^|[\s(])@([\p{L}][\p{L}\d_.-]*[\p{L}\d])`)
	reGeo     = regexp.MustCompile(`(?:^|[^\d.])(-?\d{1,2}\.\d{2,7}),\s?(-?\d{1,3}\.\d{2,7})\b`)
	reAddrEU  = regexp.MustCompile(`\b((?:\p{Lu}[\p{L}.'-]*\s)(?:[\p{L}.'-]+\s){0,3}\d{1,4}[A-Za-z]?),\s?((?:[A-Z]{1,2}-)?\d{2}-\d{3}|\d{4,5})\s+(\p{Lu}[\p{L}-]+(?:\s\p{Lu}[\p{L}-]+)?)`)
	reAddrUS  = regexp.MustCompile(`\b(\d{1,5}\s(?:[A-Z][a-z]+\s){1,3}(?:St|Street|Ave|Avenue|Rd|Road|Blvd|Boulevard|Lane|Ln|Dr|Drive|Way|Ct|Court|Pl|Place)\.?),\s?([A-Z][a-z]+(?:\s[A-Z][a-z]+)?),\s?([A-Z]{2})\s(\d{5}(?:-\d{4})?)\b`)
	orgSuffix = `AS|ASA|AB|Ltd\.?|Limited|LLC|L\.L\.C\.|Inc\.?|Corp\.?|Corporation|Co\.|GmbH|AG|SE|S\.A\.|SA|sp\. z o\.o\.|Sp\. z o\.o\.|S\.p\.A\.|S\.r\.l\.|BV|B\.V\.|NV|N\.V\.|Oy|Oyj|ApS|A/S|PLC|plc|SAS|SARL|KG|GbR|LLP|Pty Ltd`
	reOrg     = regexp.MustCompile(`\b((?:\p{Lu}[\p{L}\d&'.-]*\s+){0,4}\p{Lu}[\p{L}\d&'-]*),?\s+(` + orgSuffix + `)(?:[\s,;:)]|$)`)
	rePerson  = regexp.MustCompile(`\b(?:(Dr|Mr|Mrs|Ms|Mx|Prof|Sir|Dame|Pan|Pani|Herr|Frau)\.?\s+)?(\p{Lu}[\p{Ll}'-]+(?:\s+\p{Lu}[\p{Ll}'-]+){0,2})\b`)
	reVATNO   = regexp.MustCompile(`\b(NO)\s?(\d{3}\s?\d{3}\s?\d{3})\s?MVA\b`)
	reVATEU   = regexp.MustCompile(`\b(PL|DE|GB|FR|AT|BE|DK|ES|FI|IT|NL|SE|CZ|IE|PT|LU|SK|SI|HU|RO|BG|HR|LT|LV|EE|GR|EL|CY|MT|CH)\s?([0-9A-Z]{2}\d{6,10}|\d{8,12})\b`)
)

var ibanLen = map[string]int{"NO": 15, "PL": 28, "DE": 22, "GB": 22, "FR": 27, "ES": 24, "SE": 24, "DK": 18, "NL": 18, "FI": 18, "BE": 16,
	"AT": 20, "CH": 21, "IT": 27, "IE": 22, "PT": 25, "CZ": 24, "SK": 24, "HU": 28, "LU": 20, "EE": 20, "LT": 20, "LV": 21, "IS": 26}

// ValidIBAN checks length and mod-97.
func ValidIBAN(s string) bool {
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	if n, ok := ibanLen[s[:2]]; ok && n != len(s) {
		return false
	}
	r := s[4:] + s[:4]
	rem := 0
	for _, c := range r {
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
			rem = (rem*10 + v) % 97
			continue
		case c >= 'A' && c <= 'Z':
			v = int(c-'A') + 10
		default:
			return false
		}
		rem = (rem*100 + v) % 97
	}
	return rem == 1
}

// Luhn validates a card number.
func Luhn(digits string) bool {
	sum, alt := 0, false
	n := 0
	for i := len(digits) - 1; i >= 0; i-- {
		c := digits[i]
		if c < '0' || c > '9' {
			continue
		}
		n++
		d := int(c - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return n >= 13 && n <= 19 && sum%10 == 0
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validVAT(country, num string) bool {
	num = strings.ReplaceAll(num, " ", "")
	w := func(ws []int, mod int) (int, bool) {
		if len(num) < len(ws) {
			return 0, false
		}
		s := 0
		for i, x := range ws {
			s += int(num[i]-'0') * x
		}
		return s % mod, true
	}
	switch country {
	case "PL":
		if len(num) != 10 {
			return false
		}
		r, _ := w([]int{6, 5, 7, 2, 3, 4, 5, 6, 7}, 11)
		return r == int(num[9]-'0')
	case "NO":
		if len(num) != 9 {
			return false
		}
		r, _ := w([]int{3, 2, 7, 6, 5, 4, 3, 2}, 11)
		c := (11 - r) % 11
		return c != 10 && c == int(num[8]-'0')
	case "DE":
		return len(num) == 9
	case "GB":
		return len(num) == 9 || len(num) == 12
	}
	return len(num) >= 8 && len(num) <= 12
}

var phoneCC = map[string]string{"NO": "47", "PL": "48", "DE": "49", "US": "1", "CA": "1", "GB": "44", "FR": "33", "ES": "34", "SE": "46", "DK": "45", "FI": "358", "NL": "31", "IT": "39", "CH": "41", "AT": "43"}

// ---------------------------------------------------------------- person heuristics

var nonNames = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`The This That These Those There Then When Where What Which Who Why How We You They He She It I Our Your Their
		A An And Or But If For From To In On At By With Without Of As Is Are Was Were Be Been Has Have Had Do Does Did Not No Yes All Any Some
		Monday Tuesday Wednesday Thursday Friday Saturday Sunday January February March April May June July August September October November December
		Feb Mar Apr Jun Jul Aug Sep Sept Oct Nov Dec Q1 Q2 Q3 Q4 API SSO MFA OIDC SAML Entra Microsoft Google Graph Redis Kubernetes AI UI UX
		New Old Next Last First Second Third Final Open Closed Done Todo Task Note Fact Decision Question Idea Summary File Chunk Record
		Client Customer Account Owner Team Project Sales Lab Report Status Plan Order Invoice Contract Renewal Deal Meeting Call Email Phone
		North South East West Northern Southern Eastern Western Street Road Avenue Lane City Town Mr Mrs Ms Dr Prof Sir Dame
		Please Thanks Hello Hi Dear Regards Best Kind Note Notes See Also Total Price Pricing Discount Quote Risk Risks Product Products
		Cell Cells Test Tests Run Runs Sample Samples Batch Experiment Result Results Data Version Release Sprint Demo Budget Cost Costs`) {
		nonNames[w] = true
	}
}

var firstNames = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`Kim Ana Anna Jan Piotr Paweł Pawel Krzysztof Tomasz Marek Michał Michal Katarzyna Agnieszka Magdalena Maria Ewa
		Lars Ola Kari Ingrid Nils Erik Per Knut Sigrid Astrid Bjørn Bjorn Hans Peter Klaus Jürgen Jurgen Sabine Ursula Thomas Stefan Andreas
		Pierre Jean Marie Sophie Camille Luc Juan Carlos José Jose Lucia Sofia Pablo Elena Ivan Olga Dmitri Nina
		John James Robert Michael William David Richard Joseph Charles Mary Patricia Jennifer Linda Elizabeth Barbara Susan Jessica Sarah Karen
		Emma Olivia Noah Liam Lee Alex Sam Chris Jordan Taylor Morgan Casey Jamie Robin Maya Mia Lena Leo Max Ben Tom Tim Dan Eva Ida Emil Oskar
		Henrik Jonas Mikael Johan Magnus Anders Sven Lisa Julia Laura Clara Hannah Lea Paul Felix Lukas Marco Luca Giulia Francesca Priya Raj Arjun
		Wei Li Chen Yuki Hiro Aiko Omar Fatima Ali Sara Noor Amir Leila Tobias Victor Viktor Oliver Harry George Jack Charlie Grace Chloe Ella`) {
		firstNames[w] = true
	}
}

// ---------------------------------------------------------------- extraction

type span struct{ s, e int }

func overlaps(spans []span, s, e int) bool {
	for _, x := range spans {
		if s < x.e && e > x.s {
			return true
		}
	}
	return false
}

// Extract finds all entities in text.
func (o *Options) Extract(text string) []Entity { return o.ExtractTypes(text, nil) }

// ExtractTypes finds entities of the given types (nil = all).
func (o *Options) ExtractTypes(text string, types []string) []Entity {
	if o == nil {
		o = &Options{}
	}
	want := func(t string) bool {
		if o.disabled(t) {
			return false
		}
		if len(types) == 0 {
			return true
		}
		for _, x := range types {
			if x == t {
				return true
			}
		}
		return false
	}
	out := []Entity{}
	numeric := []span{} // accepted numeric spans: later numeric types must not overlap them
	add := func(e Entity, claim bool) {
		e.PII = piiTypes[e.Type]
		if e.Confidence == 0 {
			e.Confidence = .9
		}
		out = append(out, e)
		if claim {
			numeric = append(numeric, span{e.Start, e.End})
		}
	}
	find := func(re *regexp.Regexp) [][]int { return re.FindAllStringSubmatchIndex(text, MaxMatches) }
	sub := func(m []int, i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return text[m[2*i]:m[2*i+1]]
	}
	needType := func(t string) bool { return want(t) }
	// structured identifiers first: they claim their digits
	if needType("iban") || needType("card") || needType("phone") || needType("date") {
		for _, m := range find(reIBAN) {
			raw := text[m[0]:m[1]]
			if ValidIBAN(raw) {
				if want("iban") {
					add(Entity{Type: "iban", Value: strings.ToUpper(strings.ReplaceAll(raw, " ", "")), Raw: raw, Start: m[0], End: m[1], Confidence: .99}, true)
				} else {
					numeric = append(numeric, span{m[0], m[1]})
				}
			}
		}
	}
	for _, m := range find(reCard) {
		raw := text[m[0]:m[1]]
		d := digitsOf(raw)
		if overlaps(numeric, m[0], m[1]) || !Luhn(d) || len(d) < 13 {
			continue
		}
		if want("card") {
			add(Entity{Type: "card", Value: "**** " + d[len(d)-4:], Raw: raw, Start: m[0], End: m[1], Confidence: .95}, true)
		} else {
			numeric = append(numeric, span{m[0], m[1]})
		}
	}
	for _, m := range find(reDateTime) {
		if overlaps(numeric, m[0], m[1]) {
			continue
		}
		d := sub(m, 1)
		y, _ := strconv.Atoi(d[:4])
		mo, _ := strconv.Atoi(d[5:7])
		dd, _ := strconv.Atoi(d[8:10])
		h, _ := strconv.Atoi(sub(m, 2))
		mi, _ := strconv.Atoi(sub(m, 3))
		sec := 0
		if s := sub(m, 4); s != "" {
			sec, _ = strconv.Atoi(s)
		}
		if !validDate(y, mo, dd) || h > 23 || mi > 59 || sec > 59 {
			continue
		}
		tz := sub(m, 5)
		if tz == "" || tz == "Z" {
			tz = "Z"
		} else if !strings.Contains(tz, ":") {
			tz = tz[:3] + ":" + tz[3:]
		}
		if want("datetime") {
			add(Entity{Type: "datetime", Value: fmt.Sprintf("%sT%02d:%02d:%02d%s", d, h, mi, sec, tz), Raw: text[m[0]:m[1]], Start: m[0], End: m[1]}, false)
		}
		if want("date") {
			add(Entity{Type: "date", Value: d, Raw: d, Start: m[0], End: m[0] + 10}, true)
		} else {
			numeric = append(numeric, span{m[0], m[1]})
		}
	}
	months := o.months()
	dateFound := func(e Entity) {
		if want("date") {
			add(e, true)
		} else {
			numeric = append(numeric, span{e.Start, e.End})
		}
	}
	for _, m := range find(reISODate) {
		if overlaps(numeric, m[0], m[1]) {
			continue
		}
		y, _ := strconv.Atoi(sub(m, 1))
		mo, _ := strconv.Atoi(sub(m, 2))
		d, _ := strconv.Atoi(sub(m, 3))
		if validDate(y, mo, d) {
			dateFound(Entity{Type: "date", Value: isoDate(y, mo, d), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .99})
		}
	}
	for _, m := range find(reNumDate) {
		if overlaps(numeric, m[0], m[1]) || sub(m, 2) != sub(m, 4) {
			continue
		}
		a, _ := strconv.Atoi(sub(m, 1))
		c, _ := strconv.Atoi(sub(m, 3))
		y, _ := strconv.Atoi(sub(m, 5))
		order := o.DateOrder
		if order == "" {
			order = "dmy"
			if sub(m, 2) == "/" && !o.nonEnglish() {
				order = "mdy"
			}
		}
		if sub(m, 2) == "." {
			order = "dmy"
		}
		d, mo := a, c
		if order == "mdy" {
			d, mo = c, a
		}
		conf := .9
		if a <= 12 && c <= 12 && a != c {
			conf = .7
		}
		if !validDate(y, mo, d) {
			d, mo = mo, d
			if !validDate(y, mo, d) {
				continue
			}
		}
		dateFound(Entity{Type: "date", Value: isoDate(y, mo, d), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: conf})
	}
	for _, m := range find(reMonDMY) {
		if overlaps(numeric, m[0], m[1]) {
			continue
		}
		mo, ok := months[strings.ToLower(sub(m, 2))]
		if !ok {
			continue
		}
		d, _ := strconv.Atoi(sub(m, 1))
		y, _ := strconv.Atoi(sub(m, 3))
		if validDate(y, mo, d) {
			dateFound(Entity{Type: "date", Value: isoDate(y, mo, d), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .97})
		}
	}
	for _, m := range find(reMonMDY) {
		if overlaps(numeric, m[0], m[1]) {
			continue
		}
		mo, ok := months[strings.ToLower(sub(m, 1))]
		if !ok {
			continue
		}
		d, _ := strconv.Atoi(sub(m, 2))
		y, _ := strconv.Atoi(sub(m, 3))
		if validDate(y, mo, d) {
			dateFound(Entity{Type: "date", Value: isoDate(y, mo, d), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .97})
		}
	}
	for _, m := range find(reQuarter) {
		if !overlaps(numeric, m[0], m[1]) {
			dateFound(Entity{Type: "date", Value: sub(m, 2) + "-Q" + sub(m, 1), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .95})
		}
	}
	for _, m := range find(reWeek) {
		w, _ := strconv.Atoi(sub(m, 2))
		if w >= 1 && w <= 53 && !overlaps(numeric, m[0], m[1]) {
			dateFound(Entity{Type: "date", Value: sub(m, 1) + "-W" + sub(m, 2), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .95})
		}
	}
	// ip before versions/phones
	for _, m := range find(reIPv4) {
		raw := text[m[0]:m[1]]
		if a, err := netip.ParseAddr(raw); err == nil && !overlaps(numeric, m[0], m[1]) {
			if want("ip") {
				add(Entity{Type: "ip", Value: a.String(), Raw: raw, Start: m[0], End: m[1], Confidence: .95}, true)
			} else {
				numeric = append(numeric, span{m[0], m[1]})
			}
		}
	}
	if want("ip") {
		for _, m := range find(reIPv6) {
			raw := sub(m, 1)
			if a, err := netip.ParseAddr(raw); err == nil && a.Is6() && strings.Count(raw, ":") >= 2 {
				add(Entity{Type: "ip", Value: a.String(), Raw: raw, Start: m[2], End: m[3], Confidence: .9}, true)
			}
		}
	}
	// money (needs a currency token)
	preferComma := o.nonEnglish()
	for _, re := range []*regexp.Regexp{moneyPre, moneyPost} {
		for _, m := range find(re) {
			if overlaps(numeric, m[0], m[1]) {
				continue
			}
			raw := text[m[0]:m[1]]
			var num, suf, cur string
			if re == moneyPre {
				num, suf = sub(m, 1), sub(m, 2)
				cur = normCurrency(strings.TrimSpace(strings.TrimSuffix(raw[:m[2]-m[0]], " ")))
			} else {
				num, suf, cur = sub(m, 1), sub(m, 2), normCurrency(sub(m, 3))
			}
			if cur == "" {
				continue
			}
			if cur == "NOK" && preferComma && o.hasLocale("pl") && !o.hasLocale("nb") {
				cur = "SEK"
			}
			v, ok := moneyAmount(num+suf, preferComma || strings.Contains(raw, "zł") || cur == "PLN")
			if !ok {
				continue
			}
			if want("money") {
				add(Entity{Type: "money", Value: FormatMoney(v, cur), Raw: raw, Start: m[0], End: m[1], Amount: v, Currency: cur, Confidence: .95}, true)
			} else {
				numeric = append(numeric, span{m[0], m[1]})
			}
		}
	}
	for _, m := range find(rePercent) {
		if overlaps(numeric, m[0], m[1]) {
			continue
		}
		v, ok := ParseAmount(sub(m, 1), preferComma)
		if !ok {
			continue
		}
		if want("percent") {
			add(Entity{Type: "percent", Value: strconv.FormatFloat(round(v/100, 6), 'f', -1, 64), Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Amount: v / 100, Confidence: .97}, true)
		} else {
			numeric = append(numeric, span{m[0], m[1]})
		}
	}
	for _, m := range find(reGeo) {
		lat, _ := strconv.ParseFloat(sub(m, 1), 64)
		lon, _ := strconv.ParseFloat(sub(m, 2), 64)
		if math.Abs(lat) <= 90 && math.Abs(lon) <= 180 && !overlaps(numeric, m[2], m[5]) {
			if want("geo") {
				add(Entity{Type: "geo", Value: sub(m, 1) + "," + sub(m, 2), Raw: text[m[2]:m[5]], Start: m[2], End: m[5], Confidence: .8}, true)
			} else {
				numeric = append(numeric, span{m[2], m[5]})
			}
		}
	}
	// id codes and versions claim their digits before phones
	for _, m := range find(reIDCode) {
		s, e := m[0], m[1]
		raw := text[s:e]
		if m[2] >= 0 {
			s, e, raw = m[2], m[3], sub(m, 1)
		}
		if overlaps(numeric, s, e) {
			continue
		}
		if want("id_code") {
			add(Entity{Type: "id_code", Value: strings.ToUpper(raw), Raw: raw, Start: s, End: e, Confidence: .85}, true)
		} else {
			numeric = append(numeric, span{s, e})
		}
	}
	for _, m := range find(reVersion) {
		if overlaps(numeric, m[0], m[1]) {
			continue
		}
		v := sub(m, 1)
		if v == "" {
			v = sub(m, 2)
		}
		if want("version") {
			add(Entity{Type: "version", Value: v, Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .85}, true)
		} else {
			numeric = append(numeric, span{m[0], m[1]})
		}
	}
	for _, m := range find(reVATNO) {
		n := strings.ReplaceAll(sub(m, 2), " ", "")
		if validVAT("NO", n) && want("vat") && !overlaps(numeric, m[0], m[1]) {
			add(Entity{Type: "vat", Value: "NO" + n + "MVA", Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .95}, true)
		}
	}
	for _, m := range find(reVATEU) {
		c, n := sub(m, 1), sub(m, 2)
		if validVAT(c, n) && want("vat") && !overlaps(numeric, m[0], m[1]) {
			add(Entity{Type: "vat", Value: c + n, Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .9}, true)
		}
	}
	// phone
	for _, m := range find(rePhone) {
		raw := strings.TrimSpace(text[m[0]:m[1]])
		s, e := m[0], m[0]+len(raw)
		if overlaps(numeric, s, e) {
			continue
		}
		if s > 0 {
			if p := rune(text[s-1]); unicode.IsLetter(p) || p == '-' || p == '/' || p == '#' || p == '.' || unicode.IsDigit(p) {
				continue
			}
		}
		if e < len(text) {
			if p := rune(text[e]); unicode.IsLetter(p) || unicode.IsDigit(p) || p == '%' || (p == '.' && e+1 < len(text) && text[e+1] >= '0' && text[e+1] <= '9') {
				continue
			}
		}
		d := digitsOf(raw)
		plus, paren := strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "00"), strings.Contains(raw, "(")
		if len(d) < 7 || len(d) > 15 {
			continue
		}
		if !plus && !paren {
			if len(d) < 8 || len(d) > 11 {
				continue
			}
			if strings.Count(raw, ".") > 0 && len(d) != 10 {
				continue
			}
		}
		val := ""
		switch {
		case strings.HasPrefix(raw, "00"):
			val = "+" + d[2:]
		case plus:
			val = "+" + d
		case strings.HasPrefix(d, "00"):
			val = "+" + d[2:]
		default:
			cc := phoneCC[strings.ToUpper(o.DefaultRegion)]
			if cc == "" && len(d) == 10 && paren {
				cc = "1"
			}
			if cc == "" {
				val = d
			} else {
				val = "+" + cc + strings.TrimPrefix(d, "0")
			}
		}
		if want("phone") {
			conf := .9
			if !plus && !paren {
				conf = .75
			}
			add(Entity{Type: "phone", Value: val, Raw: raw, Start: s, End: e, Confidence: conf}, true)
		}
	}
	{
		for _, m := range find(reDurISO) {
			if !want("duration") {
				break
			}
			if m[1]-m[0] > 1 {
				add(Entity{Type: "duration", Value: text[m[0]:m[1]], Raw: text[m[0]:m[1]], Start: m[0], End: m[1], Confidence: .95}, false)
			}
		}
		ms := find(reDur)
		for i := 0; i < len(ms); i++ {
			m := ms[i]
			if overlaps(numeric, m[0], m[1]) {
				continue
			}
			parts := map[string]float64{}
			s, e := m[0], m[1]
			j := i
			for ; j < len(ms); j++ {
				mm := ms[j]
				if j > i && strings.TrimSpace(text[e:mm[0]]) != "" && strings.TrimSpace(text[e:mm[0]]) != "and" {
					break
				}
				n, u := sub(mm, 1), sub(mm, 2)
				if n == "" {
					n, u = sub(mm, 3), sub(mm, 4)
				}
				v, _ := strconv.ParseFloat(strings.ReplaceAll(n, ",", "."), 64)
				k := durUnit(u)
				if k == "" {
					break
				}
				parts[k] += v
				e = mm[1]
			}
			if len(parts) == 0 {
				continue
			}
			i = j - 1
			if want("duration") {
				add(Entity{Type: "duration", Value: isoDuration(parts), Raw: text[s:e], Start: s, End: e, Confidence: .9}, true)
			} else {
				numeric = append(numeric, span{s, e})
			}
		}
	}
	for _, m := range find(reUnit) {
		if overlaps(numeric, m[2], m[5]) {
			continue
		}
		v, ok := ParseAmount(sub(m, 1), preferComma)
		if !ok {
			continue
		}
		unit := sub(m, 2)
		if want("number_unit") {
			add(Entity{Type: "number_unit", Value: strconv.FormatFloat(v, 'f', -1, 64) + " " + unit, Raw: text[m[2]:m[5]], Start: m[2], End: m[5], Amount: v, Unit: unit, Confidence: .85}, true)
		}
	}
	// text entities
	if want("email") || want("domain") {
		for _, m := range find(reEmail) {
			raw := text[m[0]:m[1]]
			if want("email") {
				add(Entity{Type: "email", Value: strings.ToLower(raw), Raw: raw, Start: m[0], End: m[1], Confidence: .99}, false)
			}
			if want("domain") {
				at := strings.Index(raw, "@")
				add(Entity{Type: "domain", Value: strings.ToLower(raw[at+1:]), Raw: raw[at+1:], Start: m[0] + at + 1, End: m[1], Confidence: .95}, false)
			}
		}
	}
	urlSpans := []span{}
	for _, m := range find(reURL) {
		raw := strings.TrimRight(text[m[0]:m[1]], ".,;:!?)]}")
		e := m[0] + len(raw)
		u := raw
		if !strings.Contains(strings.ToLower(u), "://") {
			u = "https://" + u
		}
		host := u[strings.Index(u, "://")+3:]
		if i := strings.IndexAny(host, "/?#"); i >= 0 {
			host = host[:i]
		}
		host = strings.ToLower(host)
		urlSpans = append(urlSpans, span{m[0], e})
		if want("url") {
			add(Entity{Type: "url", Value: strings.ToLower(u[:strings.Index(u, "://")+3]) + host + u[strings.Index(u, "://")+3+len(host):], Raw: raw, Start: m[0], End: e, Confidence: .97}, false)
		}
		if want("domain") {
			add(Entity{Type: "domain", Value: strings.TrimPrefix(host, "www."), Raw: host, Start: m[0], End: e, Confidence: .9}, false)
		}
	}
	if want("domain") || want("url") {
		for _, m := range find(reDomain) {
			if overlaps(urlSpans, m[0], m[1]) || (m[0] > 0 && (text[m[0]-1] == '@' || text[m[0]-1] == '.')) {
				continue
			}
			raw := strings.TrimRight(text[m[0]:m[1]], ".,;:!?)")
			host := strings.ToLower(raw)
			path := ""
			if i := strings.Index(host, "/"); i >= 0 {
				host, path = host[:i], raw[i:]
			}
			if strings.Count(host, ".") < 1 || !strings.ContainsAny(host, "abcdefghijklmnopqrstuvwxyz") || isEmailPart(text, m[0]) {
				continue
			}
			if want("domain") {
				add(Entity{Type: "domain", Value: host, Raw: raw, Start: m[0], End: m[0] + len(raw), Confidence: .8}, false)
			}
			if path != "" && want("url") {
				add(Entity{Type: "url", Value: "https://" + host + path, Raw: raw, Start: m[0], End: m[0] + len(raw), Confidence: .8}, false)
			}
		}
	}
	for _, m := range find(reHashtag) {
		if want("hashtag") {
			add(Entity{Type: "hashtag", Value: "#" + strings.ToLower(sub(m, 1)), Raw: "#" + sub(m, 1), Start: m[2] - 1, End: m[3], Confidence: .9}, false)
		}
	}
	for _, m := range find(reMention) {
		if m[2] > 1 && text[m[2]-2] != ' ' && text[m[2]-2] != '(' && m[2]-1 != 0 {
			continue
		}
		if want("mention") {
			add(Entity{Type: "mention", Value: "@" + strings.ToLower(sub(m, 1)), Raw: "@" + sub(m, 1), Start: m[2] - 1, End: m[3], Confidence: .9}, false)
		}
	}
	orgSpans := []span{}
	for _, m := range find(reOrg) {
		name := strings.TrimSpace(sub(m, 1))
		words := strings.Fields(name)
		for len(words) > 1 && nonNames[words[0]] {
			words = words[1:]
		}
		name = strings.Join(words, " ")
		if name == "" || nonNames[name] {
			continue
		}
		s := m[2] + strings.Index(text[m[2]:m[3]], name)
		e := m[5]
		orgSpans = append(orgSpans, span{s, e})
		if want("org") {
			add(Entity{Type: "org", Value: name, Raw: text[s:e], Start: s, End: e, Unit: sub(m, 2), Confidence: .9}, false)
		}
	}
	addrSpans := []span{}
	if want("address") || want("person") {
		for _, m := range find(reAddrEU) {
			raw := text[m[0]:m[1]]
			addrSpans = append(addrSpans, span{m[0], m[1]})
			if want("address") {
				add(Entity{Type: "address", Value: strings.Join(strings.Fields(raw), " "), Raw: raw, Start: m[0], End: m[1], Confidence: .8}, false)
			}
		}
		for _, m := range find(reAddrUS) {
			raw := text[m[0]:m[1]]
			addrSpans = append(addrSpans, span{m[0], m[1]})
			if want("address") {
				add(Entity{Type: "address", Value: strings.Join(strings.Fields(raw), " "), Raw: raw, Start: m[0], End: m[1], Confidence: .85}, false)
			}
		}
	}
	if want("person") {
		for _, m := range find(rePerson) {
			title := sub(m, 1)
			name := sub(m, 2)
			s, e := m[4], m[5]
			if overlaps(orgSpans, s, e) || overlaps(addrSpans, s, e) || overlaps(urlSpans, s, e) {
				continue
			}
			toks := strings.Fields(name)
			for len(toks) > 0 && nonNames[toks[0]] {
				toks = toks[1:]
			}
			for len(toks) > 0 && nonNames[toks[len(toks)-1]] {
				toks = toks[:len(toks)-1]
			}
			bad := false
			for _, t := range toks {
				if nonNames[t] || len([]rune(t)) < 2 {
					bad = true
				}
			}
			if bad || len(toks) == 0 {
				continue
			}
			name = strings.Join(toks, " ")
			s = m[4] + strings.Index(text[m[4]:m[5]], name)
			e = s + len(name)
			conf := 0.0
			switch {
			case title != "" && len(toks) >= 1:
				conf = .9
			case len(toks) >= 2 && firstNames[toks[0]]:
				conf = .92
			case len(toks) >= 2 && len(toks) <= 3:
				conf = .6
			}
			if conf == 0 {
				continue
			}
			if title != "" {
				s = m[2]
			}
			add(Entity{Type: "person", Value: name, Raw: text[s:e], Start: s, End: e, Confidence: conf}, false)
		}
	}
	for _, c := range o.customs() {
		if c.re == nil {
			if c.Compile() != nil {
				continue
			}
		}
		// A custom pattern is addressed by its own name, so disabling it by name has to win even
		// when its entity type stays enabled for the built-ins.
		if o.disabled(c.Name) || o.disabled(c.Type) {
			continue
		}
		if len(types) > 0 && !want(c.Type) && !want(c.Name) {
			continue
		}
		for _, m := range c.re.FindAllStringIndex(text, MaxMatches) {
			raw := text[m[0]:m[1]]
			v := raw
			switch c.Normalize {
			case "upper":
				v = strings.ToUpper(v)
			case "lower":
				v = strings.ToLower(v)
			}
			e := Entity{Type: c.Type, Value: v, Raw: raw, Start: m[0], End: m[1], Confidence: .95, Pattern: c.Name}
			out = append(out, e)
			out[len(out)-1].PII = c.PII
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	// a custom pattern replaces a built-in match of the same type and span
	dedup := out[:0]
	seen := map[string]int{}
	for _, e := range out {
		k := fmt.Sprintf("%s|%d|%d", e.Type, e.Start, e.End)
		if i, ok := seen[k]; ok {
			if e.Pattern != "" {
				dedup[i] = e
			}
			continue
		}
		seen[k] = len(dedup)
		dedup = append(dedup, e)
	}
	return dedup
}

func isEmailPart(text string, start int) bool {
	for i := start - 1; i >= 0 && i >= start-64; i-- {
		c := text[i]
		if c == '@' {
			return true
		}
		if c == ' ' || c == '\n' || c == '\t' {
			return false
		}
	}
	return false
}

func round(x float64, n int) float64 {
	p := math.Pow10(n)
	return math.Round(x*p) / p
}

func durUnit(u string) string {
	u = strings.ToLower(u)
	switch {
	case strings.HasPrefix(u, "y"):
		return "Y"
	case u == "m" || strings.HasPrefix(u, "min"):
		return "TM"
	case strings.HasPrefix(u, "mo"):
		return "M"
	case strings.HasPrefix(u, "w"):
		return "W"
	case strings.HasPrefix(u, "d"):
		return "D"
	case strings.HasPrefix(u, "h"):
		return "TH"
	case strings.HasPrefix(u, "s"):
		return "TS"
	}
	return ""
}

func isoDuration(p map[string]float64) string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	s := "P"
	for _, k := range []string{"Y", "M", "W", "D"} {
		if v, ok := p[k]; ok {
			s += f(v) + k
		}
	}
	t := ""
	for _, k := range []string{"TH", "TM", "TS"} {
		if v, ok := p[k]; ok {
			t += f(v) + k[1:]
		}
	}
	if t != "" {
		s += "T" + t
	}
	return s
}

// ---------------------------------------------------------------- typed comparison & masking

// CompareValues compares two normalised values of a type: money (same currency, or via fx), dates (ISO), numbers.
// ok is false when the values are not comparable.
func (o *Options) CompareValues(typ, a, b string) (int, bool) {
	switch typ {
	case "money":
		x, cx, ok1 := ParseMoney(a)
		y, cy, ok2 := ParseMoney(b)
		if !ok1 || !ok2 {
			return 0, false
		}
		if cx != cy {
			if o == nil || o.FxRates == nil {
				return 0, false
			}
			rx, ry := o.fx(cx), o.fx(cy)
			if rx == 0 || ry == 0 {
				return 0, false
			}
			x, y = x*rx, y*ry
		}
		return cmpF(x, y), true
	case "date", "datetime":
		na, ok1 := NormalizeDate(a)
		nb, ok2 := NormalizeDate(b)
		if !ok1 || !ok2 {
			na, nb = a, b
		}
		return strings.Compare(na, nb), true
	case "percent", "number_unit", "number":
		x, ok1 := ParseAmount(strings.Fields(strings.TrimSuffix(a, "%") + " ")[0], false)
		y, ok2 := ParseAmount(strings.Fields(strings.TrimSuffix(b, "%") + " ")[0], false)
		if ok1 && ok2 {
			return cmpF(x, y), true
		}
	}
	return strings.Compare(strings.ToLower(a), strings.ToLower(b)), true
}

func (o *Options) fx(cur string) float64 {
	if cur == o.FxBase {
		return 1
	}
	return o.FxRates[cur]
}

func cmpF(x, y float64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// Mask hides most of a value: "+47 •• •• 44 55" style for phones, last 4 for iban/card.
func Mask(typ, raw string) string {
	switch typ {
	case "card":
		d := digitsOf(raw)
		if len(d) >= 4 {
			return "**** " + d[len(d)-4:]
		}
		return "****"
	}
	rs := []rune(raw)
	digits := 0
	for _, r := range rs {
		if unicode.IsDigit(r) || unicode.IsLetter(r) {
			digits++
		}
	}
	keepHead, keepTail := 3, 4
	if typ == "email" {
		at := strings.Index(raw, "@")
		if at > 1 {
			return raw[:1] + strings.Repeat("•", 3) + raw[at:]
		}
	}
	seen := 0
	out := make([]rune, len(rs))
	for i, r := range rs {
		if unicode.IsDigit(r) || unicode.IsLetter(r) {
			seen++
			if seen > keepHead && seen <= digits-keepTail {
				out[i] = '•'
				continue
			}
		}
		out[i] = r
	}
	return string(out)
}

// MaskText masks entities of the given types (cards are always masked).
func (o *Options) MaskText(text string, types []string) string {
	want := map[string]bool{"card": true}
	for _, t := range types {
		want[t] = true
	}
	ts := []string{}
	for t := range want {
		ts = append(ts, t)
	}
	es := o.ExtractTypes(text, ts)
	if len(es) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, e := range es {
		if !want[e.Type] || e.Start < last {
			continue
		}
		b.WriteString(text[last:e.Start])
		b.WriteString(Mask(e.Type, text[e.Start:e.End]))
		last = e.End
	}
	b.WriteString(text[last:])
	return b.String()
}

// Redact replaces entities of the given types with "[redacted type]".
func (o *Options) Redact(text string, types []string) string {
	es := o.ExtractTypes(text, types)
	var b strings.Builder
	last := 0
	for _, e := range es {
		if e.Start < last {
			continue
		}
		b.WriteString(text[last:e.Start])
		b.WriteString("[redacted " + e.Type + "]")
		last = e.End
	}
	b.WriteString(text[last:])
	return b.String()
}

// moneyAmount parses an amount: a single kind of separator followed by exactly three digits groups thousands in every
// locale (money has at most two decimals).
func moneyAmount(s string, preferComma bool) (float64, bool) {
	t := strings.TrimSpace(s)
	body := strings.TrimRight(strings.ToLower(t), "kmbn ")
	lc, ld := strings.LastIndex(body, ","), strings.LastIndex(body, ".")
	if (lc >= 0) != (ld >= 0) {
		last := lc
		if ld >= 0 {
			last = ld
		}
		if len(body)-last-1 == 3 {
			return ParseAmount(strings.NewReplacer(",", "", ".", "").Replace(body)+t[len(body):], false)
		}
	}
	return ParseAmount(t, preferComma)
}
