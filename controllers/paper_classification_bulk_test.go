package controllers

import "testing"

func TestClassificationSourcesAreWhitelisted(t *testing.T) {
	for source, table := range map[string]string{"benchmark": "scopus_benchmark_documents", "faculty": "scopus_documents"} {
		got, ok := classificationTable(source)
		if !ok || got != table {
			t.Fatalf("%s resolved to %q", source, got)
		}
	}
	if _, ok := classificationTable("scopus_documents; DROP TABLE users"); ok {
		t.Fatal("untrusted table name was accepted")
	}
}

func TestClassificationSnapshotDetectsContentChanges(t *testing.T) {
	title, abstract := "A paper", "First abstract"
	d := classificationInput{Title: &title, Abstract: &abstract, AuthKeywords: []byte(`["graph"]`)}
	initial := classificationHash(d)
	abstract = "Revised abstract"
	if classificationHash(d) == initial {
		t.Fatal("abstract change did not invalidate snapshot")
	}
	abstract = "First abstract"
	d.AuthKeywords = []byte(`["security"]`)
	if classificationHash(d) == initial {
		t.Fatal("keyword change did not invalidate snapshot")
	}
}
