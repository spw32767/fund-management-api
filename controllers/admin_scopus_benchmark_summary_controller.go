package controllers

import (
	"fund-management-api/services"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"strconv"
	"time"
)

func AdminBenchmarkSummaryOptions(c *gin.Context) {
	out, err := services.NewScopusBenchmarkService(nil, nil).BenchmarkSummaryOptions(c.Request.Context())
	if err != nil {
		summaryReadError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": out})
}
func summaryReadError(c *gin.Context, err error) {
	log.Printf("benchmark summary read: %v", err)
	c.JSON(500, gin.H{"success": false, "error": "ไม่สามารถอ่านข้อมูลรายงานได้ กรุณาตรวจ migration และข้อมูล benchmark"})
}
func readSummary(c *gin.Context) *services.BenchmarkSummaryReport {
	f, err := services.ParseBenchmarkSummaryFilter(c.Request.URL.Query(), time.Now())
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return nil
	}
	out, err := services.NewScopusBenchmarkService(nil, nil).BenchmarkSummary(c.Request.Context(), f)
	if err != nil {
		summaryReadError(c, err)
		return nil
	}
	return out
}
func AdminBenchmarkSummary(c *gin.Context) {
	r := readSummary(c)
	if r == nil {
		return
	}
	r.Faculty = nil
	c.JSON(200, gin.H{"success": true, "data": r})
}
func AdminBenchmarkSummaryFaculty(c *gin.Context) {
	r := readSummary(c)
	if r == nil {
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"applied_filters": r.Filters, "generated_at": r.GeneratedAt, "revision": r.Revision, "year_states": r.Years, "coverage": r.Coverage, "faculty": r.Faculty}})
}
func AdminBenchmarkSummaryDocuments(c *gin.Context) {
	q := c.Request.URL.Query()
	page := 1
	var err error
	if q.Has("page") {
		page, err = strconv.Atoi(q.Get("page"))
		if err != nil || page < 1 || page > 100000 {
			c.JSON(400, gin.H{"error": "invalid page"})
			return
		}
	}
	level := c.DefaultQuery("level", "thailand")
	if level != "thailand" && level != "kku" && level != "coc" {
		c.JSON(400, gin.H{"error": "invalid level"})
		return
	}
	userID := 0
	if q.Has("user_id") {
		userID, err = strconv.Atoi(q.Get("user_id"))
		if err != nil || userID < 1 {
			c.JSON(400, gin.H{"error": "invalid user_id"})
			return
		}
	}
	year := 0
	if q.Has("year") {
		year, err = strconv.Atoi(q.Get("year"))
		if err != nil || year < 1900 || year > time.Now().Year()+1 {
			c.JSON(400, gin.H{"error": "invalid year"})
			return
		}
	}
	var category *uint64
	if q.Has("document_category") {
		n, e := strconv.ParseUint(q.Get("document_category"), 10, 64)
		if e != nil {
			c.JSON(400, gin.H{"error": "invalid document_category"})
			return
		}
		category = &n
	}
	role := q.Get("role")
	allowed := map[string]bool{"": true, "first": true, "corresponding": true, "lead": true, "co": true, "unknown": true}
	if !allowed[role] {
		c.JSON(400, gin.H{"error": "invalid role"})
		return
	}
	quartile := q.Get("quartile")
	if !map[string]bool{"": true, "T1": true, "Q1": true, "Q2": true, "Q3": true, "Q4": true, "missing": true, "not_applicable": true}[quartile] {
		c.JSON(400, gin.H{"error": "invalid quartile"})
		return
	}
	r := readSummary(c)
	if r == nil {
		return
	}
	matches := []services.SummaryDocument{}
	for _, d := range r.Documents {
		if level == "kku" && !d.KKU || level == "coc" && !d.COC || year != 0 && d.Year != year || category != nil && d.CategoryID != *category || quartile != "" && d.Quartile != quartile {
			continue
		}
		counts := d.FacultyRoles
		if userID != 0 {
			counts = services.SummaryRoleCounts{}
			for _, a := range d.Authors {
				eligible := false
				for _, id := range a.EligibleUserIDs {
					if id == userID {
						eligible = true
					}
				}
				if !eligible {
					continue
				}
				counts.Total = 1
				known := (a.RoleStatus == "complete" || a.RoleStatus == "no_correspondence") && a.First != nil && a.Corresponding != nil
				if !known {
					counts.Unknown = 1
				} else {
					if *a.First {
						counts.First = 1
					}
					if *a.Corresponding {
						counts.Corresponding = 1
					}
					if *a.First || *a.Corresponding {
						counts.Lead = 1
					} else {
						counts.Co = 1
					}
				}
			}
			if counts.Total == 0 {
				continue
			}
		}
		if role == "first" && counts.First == 0 || role == "corresponding" && counts.Corresponding == 0 || role == "lead" && counts.Lead == 0 || role == "co" && counts.Co == 0 || role == "unknown" && counts.Unknown == 0 {
			continue
		}
		matches = append(matches, d)
	}
	start := (page - 1) * 50
	if start > len(matches) {
		start = len(matches)
	}
	end := start + 50
	if end > len(matches) {
		end = len(matches)
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"applied_filters": r.Filters, "revision": r.Revision, "generated_at": r.GeneratedAt, "year_states": r.Years, "coverage": r.Coverage, "documents": matches[start:end], "total": len(matches), "page": page, "page_size": 50}})
}
func AdminBenchmarkSummaryExport(c *gin.Context) {
	view := c.DefaultQuery("view", "overview")
	if view != "overview" && view != "faculty" {
		c.JSON(400, gin.H{"error": "invalid view"})
		return
	}
	if c.Query("revision") == "" {
		c.JSON(400, gin.H{"error": "report revision is required"})
		return
	}
	r := readSummary(c)
	if r == nil {
		return
	}
	if c.Query("revision") != r.Revision {
		c.JSON(http.StatusConflict, gin.H{"error": "ข้อมูลเปลี่ยนแล้ว กรุณาอัปเดตรายงานก่อนส่งออก Excel"})
		return
	}
	b, err := services.BuildBenchmarkSummaryExcel(r, view)
	if err != nil {
		summaryReadError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="scopus-benchmark-`+view+`-`+strconv.Itoa(r.Filters.YearFrom)+`-`+strconv.Itoa(r.Filters.YearTo)+`.xlsx"`)
	c.Header("Cache-Control", "no-store")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b)
}
