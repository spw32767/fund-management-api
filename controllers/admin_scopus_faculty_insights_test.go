package controllers

import (
	"net/http/httptest"
	"testing"

	"fund-management-api/models"
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
)

func insightBool(v bool) *bool       { return &v }
func insightString(v string) *string { return &v }

func TestFacultyInsightRoleTrustAndPrecedence(t *testing.T) {
	first := facultyInsightAuthor{AuthorID: 1, First: insightBool(true), Corresponding: insightBool(true)}
	corr := facultyInsightAuthor{AuthorID: 2, First: insightBool(false), Corresponding: insightBool(true)}
	co := facultyInsightAuthor{AuthorID: 3, First: insightBool(false), Corresponding: insightBool(false)}
	unknown := facultyInsightAuthor{AuthorID: 4}
	for _, tt := range []struct {
		name, status string
		authors      []facultyInsightAuthor
		want         string
	}{
		{"overlap once", "complete", []facultyInsightAuthor{first, corr, co}, "first"},
		{"first overrides unknown", "complete", []facultyInsightAuthor{first, unknown}, "first"},
		{"unknown defeats corr", "complete", []facultyInsightAuthor{corr, unknown}, "unknown"},
		{"unknown defeats co", "complete", []facultyInsightAuthor{co, unknown}, "unknown"},
		{"missing first blocks corr", "complete", []facultyInsightAuthor{{AuthorID: 1, Corresponding: insightBool(true)}}, "unknown"},
		{"pending first", "pending", []facultyInsightAuthor{first}, "unknown"},
		{"review first", "needs_review", []facultyInsightAuthor{first}, "unknown"},
		{"fetch error first", "fetch_error", []facultyInsightAuthor{first}, "unknown"},
		{"absent status", "", []facultyInsightAuthor{first}, "unknown"},
		{"successful no correspondence first", "no_correspondence", []facultyInsightAuthor{{AuthorID: 1, First: insightBool(true), Corresponding: insightBool(false)}}, "first"},
		{"contradictory no correspondence", "no_correspondence", []facultyInsightAuthor{corr}, "unknown"},
		{"co only", "no_correspondence", []facultyInsightAuthor{co}, "coauthor"},
		{"corr beats co", "complete", []facultyInsightAuthor{corr, co}, "corresponding"},
		{"duplicate identical", "complete", []facultyInsightAuthor{first, first}, "first"},
		{"conflicting first duplicate", "complete", []facultyInsightAuthor{first, {AuthorID: 1, First: insightBool(false), Corresponding: insightBool(true)}}, "unknown"},
		{"empty authors", "complete", nil, "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := facultyInsightRole(insightString(tt.status), tt.authors); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}

func TestFacultyInsightCountryTrust(t *testing.T) {
	for _, tt := range []struct {
		name, status, version string
		international         *bool
		complete              bool
		want                  string
	}{
		{"complete domestic", "complete", services.ScopusInsightNormalizerVersion, insightBool(false), true, "no"},
		{"partial foreign proves yes", "incomplete", services.ScopusInsightNormalizerVersion, insightBool(true), false, "yes"},
		{"partial domestic uncertain", "incomplete", services.ScopusInsightNormalizerVersion, insightBool(false), false, "unknown"},
		{"null uncertain", "incomplete", services.ScopusInsightNormalizerVersion, nil, false, "unknown"},
		{"dirty catalogue", "dirty_catalogue", services.ScopusInsightNormalizerVersion, insightBool(true), true, "unknown"},
		{"dirty payload", "dirty_payload", services.ScopusInsightNormalizerVersion, insightBool(false), true, "unknown"},
		{"obsolete", "complete", "old", insightBool(true), true, "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &models.ScopusDocumentInsight{Status: tt.status, NormalizerVersion: tt.version, International: tt.international, CountriesComplete: tt.complete}
			if got := facultyInsightInternational(m); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
	if facultyInsightInternational(nil) != "unknown" {
		t.Fatal("absent evidence must be unknown")
	}
}

func TestFacultyInsightAggregateDenominatorsAndMemberships(t *testing.T) {
	empty := aggregateFacultyInsights(nil)
	for _, p := range empty.InternationalPercent {
		if p != nil {
			t.Fatal("zero denominator is not null")
		}
	}
	for _, p := range empty.RolePercent {
		if p != nil {
			t.Fatal("zero role denominator is not null")
		}
	}
	docs := []facultyInsightDocument{
		{InternationalStatus: "yes", FacultyRole: "first", Countries: []facultyInsightCountry{{Key: "thailand"}, {Key: "japan", Name: "Japan"}, {Key: "japan", Name: "Japan"}, {Key: "china", Name: "China"}}},
		{InternationalStatus: "no", FacultyRole: "coauthor"},
		{InternationalStatus: "unknown", FacultyRole: "unknown", Countries: []facultyInsightCountry{{Key: "japan"}}},
	}
	a := aggregateFacultyInsights(docs)
	if a.Total != 3 || a.Roles["first"] != 1 || a.CountryRole["unknown"]["unknown"] != 1 || len(a.Partners) != 2 || a.Partners[0].Documents != 1 || *a.Partners[0].PercentInternational != 100 {
		t.Fatalf("bad aggregate %+v", a)
	}
	if *a.InternationalPercent["yes"] != 100.0/3 {
		t.Fatal("denominator must include unknown")
	}
	for _, s := range insightInternationalStates {
		for _, r := range insightRoleStates {
			if _, ok := a.CountryRole[s][r]; !ok {
				t.Fatal("missing cross-tab state")
			}
		}
	}
}

func TestFacultyInsightDimensionValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"year_be=2026x", "year_be=-1", "faculty_role=First", "international_status=bad", "country_key=x%3BDELETE", "revision=abc", "page=0", "page=1000001", "page_size=201", "page_size=no", "page="} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		if _, err := parseFacultyInsightDimensions(c); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
	for _, query := range []string{"year_be=2569&faculty_role=first&international_status=yes&country_key=japan&page=2&page_size=200", "year_be=undated"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		if _, err := parseFacultyInsightDimensions(c); err != nil {
			t.Fatalf("valid query %s: %v", query, err)
		}
	}
}
