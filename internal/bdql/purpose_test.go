// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"strings"
	"testing"

	"github.com/Gabski/brainDB/internal/brain"
)

const purposeYAML = `purpose:
  name: Renewals brain
  description: Renewals, pricing, discounts, contracts and customer commitments for the Q4 renewal pipeline.
  inScope:
    topics: ["renewal pricing and discounts", "contracts and terms", "customer commitments"]
    kinds: [fact, decision, deal]
  outOfScope:
    topics: ["lunch menus and canteen food", "personal errands and dentist appointments"]
    entities: [card]
  examples:
    accept: ["Offer a 6% multi-year discount for 3-year renewals"]
    reject: ["Lunch menu for Friday: fish soup and bread"]
  admission: {mode: %MODE%, accept: 0.35, review: 0.20}
`

func purposeStore(t *testing.T, mode string) *brain.Store {
	st := fixture(t)
	cfg, errs := brain.ParseConfig(strings.Replace(purposeYAML, "%MODE%", mode, 1))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := st.B.ApplyConfig(cfg, false); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPurposeEnforce(t *testing.T) {
	st := purposeStore(t, "enforce")
	r := &Runner{Store: st}
	res, err := r.Run(`CHECK "Lunch menu for Friday: fish soup and bread";
CHECK "Offer a 5% multi-year discount for 3-year renewals of the contract" AS decision;`)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Results[0].Rows[0].Vals["decision"]; d != "rejected" {
		t.Fatalf("lunch menu: %v (%s)", d, res.Render("md"))
	}
	if d := res.Results[1].Rows[0].Vals["decision"]; d != "admitted" {
		t.Fatalf("renewal discount: %v (%s)", d, res.Render("md"))
	}
	// deterministic
	res2, _ := r.Run(`CHECK "Lunch menu for Friday: fish soup and bread";`)
	if res2.Results[0].Rows[0].Vals["score"] != res.Results[0].Rows[0].Vals["score"] {
		t.Fatal("scores must be deterministic")
	}
	// rejected writes do not abort; $X is unset with a clear error
	res, err = r.Run(`REMEMBER note "Lunch menu for Friday: fish soup and bread" AS L; REMEMBER decision "Offer a 5% discount for 3-year renewals" AS D;`)
	if err != nil || !strings.Contains(res.Render("md"), "rejected") {
		t.Fatalf("rejection is reported, script continues: %v %s", err, res.Render("md"))
	}
	if _, err := r.Run(`REMEMBER note "Lunch menu for Friday: fish soup and bread" AS L; BOOST $L;`); err == nil || !strings.Contains(err.Error(), "rejected by the purpose filter") {
		t.Fatalf("$L of a rejected REMEMBER: %v", err)
	}
	if _, err := r.Run(`STRICT PURPOSE; REMEMBER decision "Offer 4% renewal discount"; REMEMBER note "Lunch menu for Friday: fish soup and bread";`); err == nil || !strings.Contains(err.Error(), "STRICT PURPOSE") {
		t.Fatalf("STRICT PURPOSE aborts: %v", err)
	}
	// forbidden entity: always rejected, even with override
	res, _ = r.Run(`REMEMBER fact "Renewal contract paid with card 4111 1111 1111 1111" OVERRIDE PURPOSE REASON 'audit';`)
	if !strings.Contains(res.Render("md"), "forbidden entity card") {
		t.Fatalf("card must be rejected: %s", res.Render("md"))
	}
	// override admits with a flag
	res, err = r.Run(`REMEMBER note "Lunch menu for Friday: fish soup and bread" OVERRIDE PURPOSE REASON 'team event logistics' AS O; SELECT purpose.flag FROM memory WHERE id = $O;`)
	if err != nil || res.Results[1].Rows[0].Vals["purpose.flag"] != "override" {
		t.Fatalf("override: %v %s", err, res.Render("md"))
	}
}

func TestPurposeQuarantine(t *testing.T) {
	st := purposeStore(t, "enforce")
	b := st.B
	// find a text in the review band
	text := ""
	for _, c := range []string{"Customer asked about the canteen lunch menu during the renewal call", "Team dinner after the renewal review", "Office plants need water before the contract meeting",
		"Kim mentioned the dentist and the renewal", "Renewal call moved; lunch menu on Friday"} {
		if b.CheckPurpose(c, "note", "notes", nil).Decision == "quarantine" {
			text = c
			break
		}
	}
	if text == "" {
		t.Skip("no review-band sample for this embedding")
	}
	r := &Runner{Store: st}
	res, err := r.Run(`REMEMBER note "` + text + `" AS Q; SELECT id, purpose.score, purpose.section FROM quarantine;`)
	if err != nil || len(res.Results[1].Rows) != 1 {
		t.Fatalf("quarantine: %v %s", err, res.Render("md"))
	}
	q := res.Results[1].Rows[0].Vals["id"].(string)
	res, _ = r.Run(`SELECT id FROM memory WHERE text ~ '` + strings.Fields(text)[0] + `'; SEARCH "` + text + `" LIMIT 20;`)
	for _, row := range append(res.Results[0].Rows, res.Results[1].Rows...) {
		if row.Vals["id"] == q {
			t.Fatal("quarantined items never appear in memory queries or search")
		}
	}
	if _, err := r.Run(`ADMIT ` + q + ` IN facts;`); err != nil {
		t.Fatal(err)
	}
	res, _ = r.Run(`SELECT section, labels FROM memory WHERE id = ` + q + `;`)
	if len(res.Results[0].Rows) != 1 || res.Results[0].Rows[0].Vals["section"] != "facts" {
		t.Fatalf("admit: %s", res.Render("md"))
	}
	res, _ = r.Run(`REMEMBER note "` + text + `" AS Q2;`)
	q2 := ""
	for _, id := range b.Quarantined() {
		q2 = id
	}
	if _, err := r.Run(`REJECT ` + q2 + ` REASON 'off-topic';`); err != nil || b.Has(q2) {
		t.Fatalf("reject: %v", err)
	}
}

func TestPurposeAdviseAndHeader(t *testing.T) {
	st := purposeStore(t, "advise")
	r := &Runner{Store: st}
	res, err := r.Run(`REMEMBER note "Lunch menu for Friday: fish soup and bread" AS L; SELECT purpose.flag FROM memory WHERE id = $L;`)
	if err != nil || res.Results[1].Rows[0].Vals["purpose.flag"] != "low" {
		t.Fatalf("advise stores with flag low: %v %s", err, res.Render("md"))
	}
	if x := st.B.ContextXML(); !strings.Contains(x, `<purpose name="Renewals brain">`) {
		t.Fatal("context XML carries the purpose")
	}
	if md := st.B.ContextMD(); !strings.Contains(md, "> purpose: Renewals brain") {
		t.Fatal("context MD carries the purpose")
	}
	st2 := purposeStore(t, "off")
	res, _ = (&Runner{Store: st2}).Run(`REMEMBER note "Lunch menu for Friday" AS L; SELECT purpose.flag FROM memory WHERE id = $L;`)
	if len(res.Results[1].Rows) != 1 {
		t.Fatal("mode off stores everything")
	}
}
