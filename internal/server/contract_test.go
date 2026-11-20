// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Gabski/brainDB/internal/brain"
)

// The web UI reads these payloads by field name and TypeScript cannot check them: the client casts the
// JSON to its interface, so a renamed or dropped field passes the type check and only breaks at runtime.
// These tests pin the field names the UI depends on.

func keysOf(t *testing.T, body string) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, body)
	}
	return m
}

func TestWhoamiContract(t *testing.T) {
	_, ts, _ := newServer(t)
	code, body, _ := do(t, "GET", ts.URL+"/api/whoami", "")
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	m := keysOf(t, body)
	// "name" is what the UI renders in the user menu and compares against the "view as" list.
	for _, k := range []string{"name", "admin", "roles", "labels", "access", "db", "can"} {
		if _, ok := m[k]; !ok {
			t.Errorf("whoami has no %q field: %s", k, body)
		}
	}
	if name, _ := m["name"].(string); name == "" {
		t.Errorf("whoami.name is empty: %s", body)
	}
	can, ok := m["can"].(map[string]any)
	if !ok {
		t.Fatalf("whoami.can is not an object: %s", body)
	}
	// The review queue is gated on the grant, not on the role's name.
	if _, ok := can["review"]; !ok {
		t.Errorf("whoami.can has no review flag: %s", body)
	}
}

func TestPatternsContract(t *testing.T) {
	_, ts, _ := newServer(t)
	code, body, _ := do(t, "GET", ts.URL+"/api/patterns", "")
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("not a JSON array: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no patterns")
	}
	for i, p := range list {
		// The UI keys its rows on the name and binds the checkbox to enabled. A missing name collapses
		// every built-in row onto the same React key; a missing flag renders them all as switched off.
		name, _ := p["name"].(string)
		if name == "" {
			t.Errorf("pattern %d has no name: %v", i, p)
		}
		if _, ok := p["enabled"].(bool); !ok {
			t.Errorf("pattern %s has no enabled flag: %v", name, p)
		}
		if _, ok := p["type"].(string); !ok {
			t.Errorf("pattern %s has no type: %v", name, p)
		}
	}
}

func TestQuarantineContract(t *testing.T) {
	s, ts, _ := newServer(t)
	st := s.Store("main")
	// Stage a quarantined item through the real admission filter: a purpose that nothing matches,
	// in enforce mode, with the whole scale as the review band.
	p := &brain.Purpose{Name: "unrelated", Description: "submarine hull welding tolerances and sonar calibration logs"}
	accept, review := 0.99, 0.0
	p.Admission.Mode, p.Admission.Accept, p.Admission.Review = "enforce", &accept, &review
	p.Admission.Apply = []string{"remember"}
	st.B.SetPurpose(p)

	code, body, _ := do(t, "POST", ts.URL+"/api/run", `{"script":"REMEMBER note \"quarterly cake rota for the office kitchen\" IN notes;"}`)
	if code != 200 {
		t.Fatalf("run status %d: %s", code, body)
	}

	code, body, _ = do(t, "GET", ts.URL+"/api/quarantine", "")
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("not a JSON array: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("nothing was quarantined: %s", body)
	}
	for _, it := range list {
		// The UI calls it.score.toFixed() and it.reasons.map(): a number and an array, under these
		// names. The raw dotted column names ("purpose.score") crash the panel.
		if _, ok := it["score"].(float64); !ok {
			t.Errorf("quarantine item has no numeric score: %v", it)
		}
		if _, ok := it["reasons"].([]any); !ok {
			t.Errorf("quarantine item has no reasons array: %v", it)
		}
		if _, ok := it["id"].(string); !ok {
			t.Errorf("quarantine item has no id: %v", it)
		}
		if _, ok := it["intendedSection"]; !ok {
			t.Errorf("quarantine item has no intendedSection: %v", it)
		}
		for _, bad := range []string{"purpose.score", "purpose.reasons", "purpose.section"} {
			if _, ok := it[bad]; ok {
				t.Errorf("quarantine item still carries the raw column name %q: %v", bad, it)
			}
		}
	}
}

// A page the operator happens to visit must not be able to drive the local API. The guards run
// before authentication, because by default no token is configured at all.
func TestCrossSiteGuards(t *testing.T) {
	s, ts, _ := newServer(t)
	s.LoopbackOnly = true
	body := `{"script":"REMEMBER note \"csrf\" IN notes;"}`

	for _, c := range []struct {
		name string
		hdr  []string
		want int
	}{
		// text/plain and a form encoding are simple requests: no CORS preflight, so the write lands
		// unless the server refuses it.
		{"text/plain from another origin", []string{"Content-Type", "text/plain;charset=UTF-8", "Origin", "https://evil.example"}, 403},
		// A Blob with an empty type sends no Content-Type at all, which defeats a type check alone.
		{"no Content-Type from another origin", []string{"Origin", "https://evil.example"}, 403},
		{"form encoding, same origin", []string{"Content-Type", "application/x-www-form-urlencoded"}, 415},
	} {
		code, _, _ := do(t, "POST", ts.URL+"/api/run", body, c.hdr...)
		if code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, code, c.want)
		}
	}

	// A body with no Content-Type at all: what a Blob of empty type produces, and what the test
	// helper above cannot express because it always sets the header.
	req, _ := http.NewRequest("POST", ts.URL+"/api/run", strings.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("no Content-Type: status %d, want 415", res.StatusCode)
	}

	// DNS rebinding: the request reaches the loopback socket but names the attacker's host. Go sends
	// req.Host on the wire, not a "Host" entry in the header map.
	req, _ = http.NewRequest("GET", ts.URL+"/api/state", nil)
	req.Host = "evil.example"
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("rebinding Host: status %d, want 403", res.StatusCode)
	}

	// Everything a real client does still works.
	for _, c := range []struct {
		name string
		hdr  []string
	}{
		{"json without an Origin (curl, MCP)", []string{"Content-Type", "application/json"}},
		{"json from the page the server serves", []string{"Content-Type", "application/json", "Origin", ts.URL}},
	} {
		if code, b, _ := do(t, "POST", ts.URL+"/api/run", `{"script":"SELECT COUNT(*) FROM memory;"}`, c.hdr...); code != 200 {
			t.Errorf("%s: status %d: %s", c.name, code, b)
		}
	}

	// An operator can still allow a browser client on another origin.
	s.AllowedOrigins = []string{"https://studio.example"}
	if code, b, _ := do(t, "POST", ts.URL+"/api/run", `{"script":"SELECT COUNT(*) FROM memory;"}`,
		"Content-Type", "application/json", "Origin", "https://studio.example"); code != 200 {
		t.Errorf("allow-listed origin: status %d: %s", code, b)
	}
}
