package controllers

import "testing"

func TestNormalizePaperDOI(t *testing.T) {
	if got := normalizePaperDOI(" HTTPS://DOI.ORG/10.1000/ABC "); got != "10.1000/abc" {
		t.Fatalf("normalizePaperDOI() = %q", got)
	}
}

func TestPaperTitleSimilarity(t *testing.T) {
	left := normalizePaperTitle("Deep Learning: A Study of Networks")
	right := normalizePaperTitle("Deep Learning - A Study of Networks")
	if score := paperTitleSimilarity(left, right); score != 1 {
		t.Fatalf("expected punctuation-only difference to match exactly, got %v", score)
	}
	different := normalizePaperTitle("Unrelated database optimization research")
	if score := paperTitleSimilarity(left, different); score >= 0.82 {
		t.Fatalf("expected unrelated title below threshold, got %v", score)
	}
}

func TestIsValidClassificationConfidence(t *testing.T) {
	for _, value := range []string{"High", "Medium", "Low", "Preface"} {
		if !isValidClassificationConfidence(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "Needs Review", "0.95", "high"} {
		if isValidClassificationConfidence(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}
