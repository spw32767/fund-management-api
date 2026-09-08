package services

import (
	"encoding/json"
	"testing"
)

func TestBenchmarkAffiliationParsingAndFirstAuthorLink(t *testing.T) {
	raw := json.RawMessage(`{
		"eid":"2-s2.0-123",
		"affiliation":[
			{"afid":"60017165","affilname":"Khon Kaen University","affiliation-city":"Khon Kaen","affiliation-country":"Thailand","affiliation-url":"https://api.elsevier.com/content/affiliation/affiliation_id/60017165"},
			{"afid":"60280609","affilname":"Faculty of Science, Khon Kaen University","affiliation-country":"Thailand"}
		],
		"author":[{"authid":"111","afid":["60280609","60017165"]}]
	}`)

	entry, err := parseScopusEntry(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Affiliation) != 2 {
		t.Fatalf("affiliations=%d, want 2", len(entry.Affiliation))
	}
	if entry.Affiliation[0].Afid != "60017165" || entry.Affiliation[0].Country != "Thailand" {
		t.Fatalf("unexpected first affiliation: %+v", entry.Affiliation[0])
	}

	affiliationMap := map[string]uint{"60017165": 10, "60280609": 20}
	got := benchmarkAuthorAffiliationID(entry.Author[0], affiliationMap)
	if got == nil || *got != 20 {
		t.Fatalf("affiliation id=%v, want first author AF-ID mapped to 20", got)
	}
}

func TestBenchmarkAuthorAffiliationIDHandlesMissingMapping(t *testing.T) {
	author := scopusAuthor{Affiliations: scopusStringSlice{"unknown", "60017165"}}
	if got := benchmarkAuthorAffiliationID(author, map[string]uint{"60017165": 10}); got != nil {
		t.Fatalf("affiliation id=%v, want nil when first AF-ID is absent", *got)
	}
}

func TestBuildBenchmarkDocumentDoesNotRetainRawJSON(t *testing.T) {
	entry, err := parseScopusEntry(json.RawMessage(`{"eid":"2-s2.0-123","dc:title":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if document := buildBenchmarkDocument(entry); len(document.RawJSON) != 0 {
		t.Fatal("benchmark harvest must not retain raw_json after BE-2 backfill")
	}
}
