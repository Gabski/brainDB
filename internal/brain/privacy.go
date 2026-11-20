// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import "github.com/Gabski/brainDB/internal/patterns"

func (b *Brain) patOpts() *patterns.Options {
	if b.Pat == nil {
		b.Pat = &patterns.Options{}
	}
	return b.Pat
}

// maskText masks PII types listed in Privacy (cards are always masked).
func (b *Brain) maskText(s string) string {
	if s == "" {
		return s
	}
	if len(b.Privacy) == 0 && !mayHoldCard(s) {
		// With no masked types configured, MaskText only masks card numbers. Running the whole pattern
		// library over every rendered result to find none was the single biggest cost in a query.
		return s
	}
	return b.patOpts().MaskText(s, b.Privacy)
}

// SetPatterns configures the pattern library and privacy masking.
func (b *Brain) SetPatterns(o *patterns.Options, mask, redact []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Pat, b.Privacy, b.RedactOnIngest = o, mask, redact
}

// Patterns lists built-in and custom patterns.
func (b *Brain) Patterns() []patterns.Info {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.patOpts().List()
}

// ExtractText runs the pattern library on any text.
func (b *Brain) ExtractText(text string, types []string) []patterns.Entity {
	b.mu.Lock()
	defer b.mu.Unlock()
	es := b.patOpts().ExtractTypes(text, types)
	for i := range es {
		if es[i].Type == "card" || contains(b.Privacy, es[i].Type) {
			es[i].Raw = patterns.Mask(es[i].Type, es[i].Raw)
		}
	}
	return es
}

// PatternOptions returns the pattern configuration.
func (b *Brain) PatternOptions() *patterns.Options {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.patOpts()
}

// SetDisabledPatterns sets the disabled pattern types.
func (b *Brain) SetDisabledPatterns(d []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.patOpts().Disabled = d
	b.entCache = nil
}

// mayHoldCard reports whether s could contain a card number: at least 13 digits in one run, counting
// spaces and dashes as part of the run. It over-approximates on purpose — a false yes only costs one
// full scan, a false no would leak a card.
func mayHoldCard(s string) bool {
	digits := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
			if digits >= 13 {
				return true
			}
		case c == ' ' || c == '-':
			// keeps the run alive: "4111 1111 1111 1111"
		default:
			digits = 0
		}
	}
	return false
}
