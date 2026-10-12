package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFacultyInsightSortValidation(t *testing.T) {
	for _, query := range []string{"sort_field=title", "sort_direction=asc", "sort_field=&sort_direction=", "sort_field=&sort_direction=asc", "sort_field=details&sort_direction=asc", "sort_field=title%3BDROP&sort_direction=asc", "sort_field=title&sort_direction=DESC", "sort_field=year&sort_direction=asc%3BDELETE"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		if _, err := parseFacultyInsightDimensions(c); err == nil {
			t.Fatalf("accepted %s", query)
		}
	}
	for _, field := range []string{"title", "year", "collaboration", "faculty_role"} {
		for _, direction := range []string{"asc", "desc"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?sort_field="+field+"&sort_direction="+direction, nil)
			d, err := parseFacultyInsightDimensions(c)
			if err != nil || d.SortField != field || d.SortDirection != direction {
				t.Fatalf("valid sort rejected: %+v %v", d, err)
			}
		}
	}
}

func TestFacultyInsightSortLabelsTiesAndMissing(t *testing.T) {
	year := func(v int) *int { return &v }
	countries := func(names ...string) []facultyInsightCountry {
		result := []facultyInsightCountry{}
		for _, name := range names {
			result = append(result, facultyInsightCountry{Name: name})
		}
		return result
	}
	tests := []struct {
		field     string
		docs      []facultyInsightDocument
		asc, desc []uint
	}{
		{"title", []facultyInsightDocument{{ID: 8}, {ID: 7, Title: insightString(" ")}, {ID: 4, Title: insightString("alpha")}, {ID: 2, Title: insightString(" Alpha ")}, {ID: 3, Title: insightString("Zulu")}, {ID: 5, Title: insightString("ก")}, {ID: 6, Title: insightString("😀")}}, []uint{2, 4, 3, 5, 6, 7, 8}, []uint{6, 5, 3, 2, 4, 7, 8}},
		{"year", []facultyInsightDocument{{ID: 9}, {ID: 4, YearCE: year(2026)}, {ID: 3, YearCE: year(2024)}, {ID: 2, YearCE: year(2026)}, {ID: 8}}, []uint{3, 2, 4, 8, 9}, []uint{2, 4, 3, 8, 9}},
		{"faculty_role", []facultyInsightDocument{{ID: 6, FacultyRole: "unknown"}, {ID: 4, FacultyRole: "first"}, {ID: 3, FacultyRole: "corresponding"}, {ID: 2, FacultyRole: "coauthor"}, {ID: 5, FacultyRole: "coauthor"}, {ID: 7}}, []uint{2, 5, 3, 4, 6, 7}, []uint{4, 3, 2, 5, 6, 7}},
		{"collaboration", []facultyInsightDocument{{ID: 9, InternationalStatus: "unknown", CountryEvidenceCurrent: true, Countries: countries("Japan")}, {ID: 8, InternationalStatus: "unknown"}, {ID: 4, InternationalStatus: "yes", CountryEvidenceCurrent: true, Countries: countries("Japan", "Thailand")}, {ID: 3, InternationalStatus: "yes", CountryEvidenceCurrent: true, Countries: countries("China", "Thailand")}, {ID: 2, InternationalStatus: "no", CountryEvidenceCurrent: true, Countries: countries("Thailand")}, {ID: 5, InternationalStatus: "yes", CountryEvidenceCurrent: false, Countries: countries("China")}, {ID: 6, InternationalStatus: "yes", CountryEvidenceCurrent: true}}, []uint{2, 3, 4, 5, 6, 8, 9}, []uint{4, 3, 2, 5, 6, 8, 9}},
	}
	for _, tt := range tests {
		for _, direction := range []string{"asc", "desc"} {
			docs := append([]facultyInsightDocument(nil), tt.docs...)
			sortFacultyInsightDocuments(docs, tt.field, direction)
			ids := []uint{}
			for _, d := range docs {
				ids = append(ids, d.ID)
			}
			want := tt.asc
			if direction == "desc" {
				want = tt.desc
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("%s %s got %v want %v", tt.field, direction, ids, want)
			}
		}
	}
	docs := []facultyInsightDocument{{ID: 1}, {ID: 2}}
	sortFacultyInsightDocuments(docs, "", "")
	if docs[0].ID != 1 {
		t.Fatal("legacy default changed")
	}
}
