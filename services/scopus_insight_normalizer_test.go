package services

import (
	"encoding/json"
	"reflect"
	"testing"
)

const insightDomestic = `{"author-count":"1","affiliation":[{"afid":"A","affiliation-country":"Thailand"}],"author":[{"authid":"one","afid":"A"}]}`
const insightDual = `{"author-count":"1","affiliation":[{"afid":"A","affiliation-country":"Thailand"},{"afid":"B","affiliation-country":"Viet Nam"}],"author":[{"authid":"one","afid":["A","B"]}]}`

func insightStatus(n ScopusInsightNormalization) string {
	if n.International == nil {
		return "unknown"
	}
	if *n.International {
		return "yes"
	}
	return "no"
}
func hasInsightReason(n ScopusInsightNormalization, r string) bool {
	for _, x := range n.Reasons {
		if x == r {
			return true
		}
	}
	return false
}
func mutateInsight(raw string, edit func(map[string]interface{})) string {
	var p map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &p)
	edit(p)
	b, _ := json.Marshal(p)
	return string(b)
}

func TestCoreInsightNormalization(t *testing.T) {
	tests := []struct {
		name, raw, status, reason string
		cat                       map[string]string
		complete                  bool
	}{
		{name: "complete domestic", raw: insightDomestic, status: "no", complete: true},
		{name: "secondary foreign dual affiliation", raw: insightDual, status: "yes", complete: true},
		{name: "catalogue supplies country", raw: `{"author-count":1,"affiliation":{"afid":"A"},"author":{"authid":"one","afid":"A"}}`, cat: map[string]string{"A": "Thailand"}, status: "no", complete: true},
		{name: "unmapped AFID retained with payload evidence", raw: insightDual, cat: map[string]string{"A": "Thailand"}, status: "yes", complete: true},
		{name: "unknown value is not foreign", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) {
			p["affiliation"] = map[string]interface{}{"afid": "A", "affiliation-country": "Atlantis"}
		}), status: "unknown", reason: "unrecognized_payload_country"},
		{name: "placeholder not foreign", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) {
			p["affiliation"] = map[string]interface{}{"afid": "A", "affiliation-country": "N/A"}
		}), status: "unknown", reason: "unresolved_affiliation_country"},
		{name: "missing author count", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { delete(p, "author-count") }), status: "unknown", reason: "missing_or_invalid_author_count"},
		{name: "zero author count", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { p["author-count"] = 0 }), status: "unknown", reason: "missing_or_invalid_author_count"},
		{name: "truncated authors", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { p["author-count"] = 2 }), status: "unknown", reason: "author_count_mismatch"},
		{name: "explicit truncation", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { p["affiliations-truncated"] = true }), status: "unknown", reason: "explicit_truncation"},
		{name: "missing AFIDs", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { p["author"] = map[string]interface{}{"authid": "one"} }), status: "unknown", reason: "missing_or_malformed_author_afids"},
		{name: "empty AFID array", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) {
			p["author"] = map[string]interface{}{"authid": "one", "afid": []string{}}
		}), status: "unknown", reason: "missing_or_malformed_author_afids"},
		{name: "author AFID absent from doc list", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) {
			p["author"] = map[string]interface{}{"authid": "one", "afid": []string{"A", "B"}}
		}), cat: map[string]string{"A": "Thailand", "B": "India"}, status: "yes", reason: "author_afid_missing_document_affiliation"},
		{name: "known foreign beats incomplete roster", raw: mutateInsight(insightDual, func(p map[string]interface{}) { delete(p, "author-count") }), status: "yes", reason: "missing_or_invalid_author_count"},
		{name: "invalid payload", raw: `{`, status: "unknown", reason: "invalid_payload"},
		{name: "non object", raw: `[]`, status: "unknown", reason: "invalid_payload"},
		{name: "trailing payload", raw: insightDomestic + ` {}`, status: "unknown", reason: "invalid_payload"},
		{name: "null doc affiliations", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { p["affiliation"] = nil }), cat: map[string]string{"A": "Thailand"}, status: "unknown", reason: "missing_or_malformed_document_affiliations"},
		{name: "empty doc affiliations", raw: mutateInsight(insightDomestic, func(p map[string]interface{}) { p["affiliation"] = []interface{}{} }), cat: map[string]string{"A": "Thailand"}, status: "unknown", reason: "missing_or_malformed_document_affiliations"},
		{name: "malformed list retains known foreign evidence", raw: `{"author-count":1,"author":{"authid":"one","afid":"B"},"affiliation":[42,{"afid":"B","affiliation-country":"India"}]}`, status: "yes", reason: "missing_or_malformed_document_affiliations"},
		{name: "foreign without afid still positive", raw: `{"author-count":1,"author":{"authid":"one","afid":"A"},"affiliation":{"affiliation-country":"India"}}`, status: "yes", reason: "missing_or_invalid_document_afid"},
		{name: "payload catalogue conflict cannot assert domestic", raw: insightDomestic, cat: map[string]string{"A": "India"}, status: "yes", reason: "payload_catalogue_country_conflict"},
		{name: "cleared catalogue does not reuse payload", raw: insightDomestic, cat: map[string]string{"A": ""}, status: "unknown", reason: "unresolved_catalogue_country"},
		{name: "unknown catalogue cannot prove foreign", raw: insightDomestic, cat: map[string]string{"A": "Not a country"}, status: "unknown", reason: "unresolved_catalogue_country"},
		{name: "duplicate author id defeats complete", raw: `{"author-count":2,"author":[{"authid":"one","afid":"A"},{"authid":"one","afid":"A"}],"affiliation":{"afid":"A","affiliation-country":"Thailand"}}`, status: "unknown", reason: "missing_or_duplicate_author_id"},
		{name: "conflicting duplicate countries", raw: `{"author-count":1,"author":{"authid":"one","afid":"A"},"affiliation":[{"afid":"A","affiliation-country":"Thailand"},{"afid":"A","affiliation-country":"India"}]}`, status: "yes", reason: "conflicting_payload_countries"},
		{name: "wrapped numeric and scalar shapes with Search attributes", raw: `{"author-count":{"@limit":"100","$":1},"author":{"authid":{"$":"one"},"afid":[{"@_fa":"true","$":"A"}]},"affiliation":{"afid":{"$":"A"},"affiliation-country":{"$":"  THAILAND  "}}}`, status: "no", complete: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := NormalizeScopusInsight([]byte(tc.raw), tc.cat)
			if got := insightStatus(n); got != tc.status {
				t.Fatalf("status=%s want=%s; reasons=%v", got, tc.status, n.Reasons)
			}
			if n.CountriesComplete != tc.complete {
				t.Fatalf("complete=%v want=%v reasons=%v", n.CountriesComplete, tc.complete, n.Reasons)
			}
			if tc.reason != "" && !hasInsightReason(n, tc.reason) {
				t.Fatalf("missing reason %s: %v", tc.reason, n.Reasons)
			}
		})
	}
}

func TestCoreInsightAliasesDeduplicationAndHash(t *testing.T) {
	raw := []byte(`{"author-count":1,"author":{"authid":"one","afid":["B","A","B",{"$":"A"}]},"affiliation":[{"afid":"B","affiliation-country":"Viet Nam"},{"afid":"A","affiliation-country":"  THAILAND "},{"afid":"B","affiliation-country":"Vietnam"}]}`)
	n := NormalizeScopusInsight(raw, map[string]string{"A": "Thailand", "B": "VIET NAM"})
	if insightStatus(n) != "yes" || !n.CountriesComplete || len(n.Affiliations) != 2 || len(n.Countries) != 2 {
		t.Fatalf("normalization=%+v", n)
	}
	if n.Countries[1].CountryKey != "vietnam" || n.Countries[1].CountryName != "Vietnam" {
		t.Fatal(n.Countries)
	}
	if !reflect.DeepEqual(n, NormalizeScopusInsight(raw, map[string]string{"B": "VIET NAM", "A": "Thailand"})) {
		t.Fatal("map order affects deterministic normalization")
	}
	changed := NormalizeScopusInsight(raw, map[string]string{"A": "Thailand", "B": ""})
	if n.CatalogueHash == changed.CatalogueHash || n.PayloadHash != changed.PayloadHash || insightStatus(changed) != "unknown" {
		t.Fatal("catalogue correction must invalidate foreign evidence")
	}
	if len(changed.Affiliations) != 2 {
		t.Fatal("unresolved AFID was discarded")
	}
}

func TestCoreInsightAFIDCaseWhitespaceAndConflicts(t *testing.T) {
	raw := []byte(`{"author-count":1,"author":{"authid":"one","afid":["a"," A "]},"affiliation":[{"afid":" A ","affiliation-country":" Thailand "},{"afid":"a","affiliation-country":"THAILAND"}]}`)
	n := NormalizeScopusInsight(raw, map[string]string{" A ": "thailand"})
	if insightStatus(n) != "no" || !n.CountriesComplete || len(n.Affiliations) != 1 || n.Affiliations[0].Afid != "a" {
		t.Fatalf("canonical AFIDs: %+v", n)
	}
	n = NormalizeScopusInsight(raw, map[string]string{"A": "Thailand", " a ": "India"})
	if insightStatus(n) != "unknown" || !hasInsightReason(n, "conflicting_catalogue_afids") {
		t.Fatalf("ambiguous catalogue: %+v", n)
	}
	accent := NormalizeScopusInsight(raw, map[string]string{"á": "India"})
	if insightStatus(accent) != "no" {
		t.Fatal("accent-insensitive collation must not invent AFID match", accent)
	}
}
