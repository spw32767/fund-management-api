//go:build insight_integration || insight_mariadb

package controllers

import (
	"fmt"
	"strings"
	"testing"
)

func TestFacultyInsightAPISortFullPopulationBeforePages(t *testing.T) {
	db := facultyAPIIntegrationDB(t)
	seedFacultyAPIFixture(t, db)
	for id := 1; id <= 246; id++ {
		if err := db.Exec(`UPDATE scopus_documents SET title=? WHERE id=?`, fmt.Sprintf("Sort %03d", 247-id), id).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{`UPDATE scopus_documents SET title='Alpha' WHERE id=245`, `UPDATE scopus_documents SET title='alpha' WHERE id=246`, `UPDATE scopus_documents SET title=NULL WHERE id=243`, `UPDATE scopus_documents SET title=' ' WHERE id=244`} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	total, rev := facultyAPITotal(t, "")
	if total != 246 {
		t.Fatal(total)
	}
	request := func(query string) map[string]interface{} {
		w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+rev+query)
		if w.Code != 200 || b["revision"] != rev {
			t.Fatalf("%d %v", w.Code, b)
		}
		return b
	}
	ids := func(b map[string]interface{}) []int {
		out := []int{}
		for _, doc := range b["documents"].([]interface{}) {
			out = append(out, int(doc.(map[string]interface{})["document_id"].(float64)))
		}
		return out
	}
	defaultPage := request("&page_size=200")
	if defaultPage["sort"] != "year_desc" || defaultPage["sort_field"] != "year" || defaultPage["sort_direction"] != "desc" || ids(defaultPage)[0] != 1 || ids(defaultPage)[1] != 5 {
		t.Fatal("default must be newest year first with stable ID ties")
	}
	defaultLast := request("&page=2&page_size=200")
	lastDefaultDocs := defaultLast["documents"].([]interface{})
	if lastDefaultDocs[len(lastDefaultDocs)-1].(map[string]interface{})["year_ce"] != nil || ids(defaultLast)[len(lastDefaultDocs)-1] != 246 {
		t.Fatal("default undated must remain last")
	}
	for _, direction := range []string{"asc", "desc"} {
		first := request("&page_size=200&sort_field=title&sort_direction=" + direction)
		last := request("&page=2&page_size=200&sort_field=title&sort_direction=" + direction)
		a, b := ids(first), ids(last)
		if len(a) != 200 || len(b) != 46 || b[44] != 243 || b[45] != 244 {
			t.Fatal("missing/page coverage", a, b)
		}
		if direction == "asc" && (a[0] != 245 || a[1] != 246 || a[2] != 242) {
			t.Fatal("did not sort beyond page 200", a[:3])
		}
		if direction == "desc" && (a[0] != 1 || b[42] != 245 || b[43] != 246) {
			t.Fatal("descending or stable ties", a[0], b)
		}
		seen := map[int]bool{}
		for _, id := range append(a, b...) {
			if seen[id] {
				t.Fatal("duplicate across pages", id)
			}
			seen[id] = true
		}
		if len(seen) != 246 {
			t.Fatal("population truncated")
		}
	}
	for _, field := range []string{"year", "collaboration", "faculty_role"} {
		for _, direction := range []string{"asc", "desc"} {
			b := request("&page_size=200&sort_field=" + field + "&sort_direction=" + direction)
			if b["sort"] != field+"_"+direction || b["total"] != float64(246) || b["sort_field"] != field {
				t.Fatal("sort response", b)
			}
			tail := request("&page=2&page_size=200&sort_field=" + field + "&sort_direction=" + direction)["documents"].([]interface{})
			last := tail[len(tail)-1].(map[string]interface{})
			if field == "year" && last["year_ce"] != nil || field == "collaboration" && last["international_status"] != "unknown" || field == "faculty_role" && last["faculty_role"] != "unknown" {
				t.Fatal("missing not last", field, direction, last)
			}
		}
	}
	b := request("&drilldown_search=Alice&international_status=no&country_key=thailand&faculty_role=coauthor&sort_field=title&sort_direction=asc&page_size=10")
	if b["total"].(float64) <= 200 || ids(b)[0] != 245 || ids(b)[1] != 242 || b["scope_total"] != b["total"] {
		t.Fatal("search/dimension/sort population", b)
	}
	b = request("&drilldown_search=sid-246&year_be=undated&sort_field=title&sort_direction=desc")
	if b["total"] != float64(1) || ids(b)[0] != 246 {
		t.Fatal("searched undated selection", b)
	}
	_, after := facultyAPITotal(t, "")
	if after != rev {
		t.Fatal("sorting affected summary revision")
	}
	w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, "revision="+strings.Repeat("0", 64)+"&sort_field=title&sort_direction=asc")
	if w.Code != 409 || b["documents"] != nil {
		t.Fatal("sort bypassed revision")
	}
	for _, query := range []string{"sort_field=details&sort_direction=asc", "sort_field=title&sort_direction=asc%3BDELETE", "sort_direction=desc"} {
		w, b := facultyAPIRequest(t, AdminGetScopusFacultyInsightsDrilldown, query)
		if w.Code != 400 || b["code"] != "invalid_insight_dimension" {
			t.Fatal("invalid sort accepted", query, b)
		}
	}
}
