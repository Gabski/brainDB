// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// OMap is an insertion-ordered map used for templates, YAML and ordered JSON.
// Values are nil, bool, string, json.Number, float64, int, []any or *OMap.
type OMap struct {
	Keys  []string
	Vals  map[string]any
	Lines map[string]int // source line of each key (YAML only)
}

// NewOMap returns an empty ordered map.
func NewOMap() *OMap { return &OMap{Vals: map[string]any{}} }

// Set sets a key (appending it if new).
func (m *OMap) Set(k string, v any) *OMap {
	if _, ok := m.Vals[k]; !ok {
		m.Keys = append(m.Keys, k)
	}
	m.Vals[k] = v
	return m
}

// Get returns a value.
func (m *OMap) Get(k string) (any, bool) { v, ok := m.Vals[k]; return v, ok }

// Delete removes a key.
func (m *OMap) Delete(k string) {
	if _, ok := m.Vals[k]; !ok {
		return
	}
	delete(m.Vals, k)
	m.Keys = filterStr(m.Keys, func(x string) bool { return x != k })
}

func (m *OMap) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.Keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kk, _ := json.Marshal(k)
		b.Write(kk)
		b.WriteByte(':')
		v, err := json.Marshal(m.Vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (m *OMap) UnmarshalJSON(d []byte) error {
	v, err := DecodeOrderedJSON(d)
	if err != nil {
		return err
	}
	o, ok := v.(*OMap)
	if !ok {
		return errf("expected a JSON object")
	}
	*m = *o
	return nil
}

// DecodeOrderedJSON decodes JSON keeping object key order (objects → *OMap, numbers → json.Number).
func DecodeOrderedJSON(d []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(d))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errf("unexpected data after JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			m := NewOMap()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				m.Set(k, v)
			}
			_, err := dec.Token()
			return m, err
		case '[':
			l := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				l = append(l, v)
			}
			_, err := dec.Token()
			return l, err
		}
		return nil, errf("unexpected delimiter %v", x)
	default:
		return x, nil
	}
}

// toOrdered converts any JSON-marshalable value to the ordered generic form (struct field order is kept).
func toOrdered(v any) any {
	d, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	o, err := DecodeOrderedJSON(d)
	if err != nil {
		return nil
	}
	return o
}

func jsonIndent(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return strings.TrimRight(b.String(), "\n")
}
