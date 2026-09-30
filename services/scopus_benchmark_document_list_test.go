package services

import (
	"fmt"
	"testing"
)

func TestSummaryDocumentSearchFindsMatchesBeyondFirstPage(t *testing.T) {
	docs := make([]SummaryDocument, 105)
	for i := range docs {
		docs[i] = SummaryDocument{ID: uint(i + 1), EID: fmt.Sprintf("2-s2.0-%d", i), Title: "Ordinary paper", CategoryID: 1, Quartile: "Q1"}
	}
	docs[100].Title = "Contextual Mobile Games"
	docs[100].Authors = []SummaryAuthor{{Name: "ผู้เขียน ทดสอบ", ScopusAuthorID: "12345"}}
	for _, search := range []string{"  MOBILE games  ", "ผู้เขียน", "12345", "2-s2.0-100"} {
		matches := FilterSummaryDocumentList(docs, search, nil, "")
		if len(matches) != 1 || matches[0].ID != 101 {
			t.Fatalf("search %q failed to find result outside the first page: %+v", search, matches)
		}
	}
	if matches := FilterSummaryDocumentList(docs, "not in corpus", nil, ""); len(matches) != 0 {
		t.Fatal("unmatched query returned documents")
	}
}

func TestSummaryDocumentSearchCombinesCategoryAndQuartile(t *testing.T) {
	docs := []SummaryDocument{
		{ID: 1, Title: "AI research", CategoryID: 1, Quartile: "Q1", DOI: "10.123/first"},
		{ID: 2, Title: "AI research", CategoryID: 1, Quartile: "Q2"},
		{ID: 3, Title: "AI research", CategoryID: 2, Quartile: "Q1"},
		{ID: 4, Title: "AI research", CategoryID: 0, Quartile: "missing", PublicationName: "Example Journal"},
	}
	category := uint64(1)
	if matches := FilterSummaryDocumentList(docs, "AI", &category, "Q1"); len(matches) != 1 || matches[0].ID != 1 {
		t.Fatalf("combined filters: %+v", matches)
	}
	category = 0
	if matches := FilterSummaryDocumentList(docs, "example journal", &category, "missing"); len(matches) != 1 || matches[0].ID != 4 {
		t.Fatalf("unclassified documents: %+v", matches)
	}
	if matches := FilterSummaryDocumentList(docs[:1], "", nil, "Q2"); len(matches) != 0 {
		t.Fatal("additional filter broadened an already selected cohort")
	}
	if matches := FilterSummaryDocumentList(docs, "10.123/first", nil, ""); len(matches) != 1 || matches[0].ID != 1 {
		t.Fatal("DOI search did not find the paper")
	}
}
