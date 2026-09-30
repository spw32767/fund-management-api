// Import existing workbook classifications into benchmark documents only.
// Default is a dry run. No Scopus calls, document insertion or metric writes.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fund-management-api/config"
	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type classification struct {
	EID        string `json:"eid"`
	Year       int    `json:"year"`
	Category   string `json:"category"`
	Confidence string `json:"confidence"`
}
type document struct {
	ID                       uint
	EID                      string `gorm:"column:eid"`
	PubYear                  int
	Category                 *uint64
	ClassificationConfidence *string
}
type change struct {
	classification
	ID         uint   `json:"document_id"`
	CategoryID uint64 `json:"category_id"`
}
type report struct {
	Database           string                   `json:"database"`
	File               string                   `json:"source_file"`
	SHA256             string                   `json:"source_sha256"`
	GeneratedAt        time.Time                `json:"generated_at"`
	Apply              bool                     `json:"apply"`
	SourceRows         int                      `json:"source_rows"`
	Unclassified       int                      `json:"unclassified_needs_review"`
	Eligible           int                      `json:"eligible_classified_rows"`
	Matched            int                      `json:"matched_documents"`
	AlreadySame        int                      `json:"already_same"`
	Conflicts          []string                 `json:"preserved_existing_conflicts"`
	Missing            []string                 `json:"unmatched_eids"`
	YearMismatch       []string                 `json:"publication_year_mismatches"`
	OutsideYears       []string                 `json:"current_year_outside_2025_2026"`
	Changes            []change                 `json:"changes"`
	Updated            int64                    `json:"updated"`
	CoreRolesUnchanged bool                     `json:"core_roles_unchanged"`
	Runs               []map[string]interface{} `json:"harvest_runs"`
}

func main() {
	file := flag.String("file", "", "source FINAL workbook")
	apply := flag.Bool("apply", false, "fill empty classifications; default dry run")
	expect := flag.String("expect-database", "", "required exact database name")
	output := flag.String("report", "", "optional JSON audit file")
	flag.Parse()
	if *file == "" || *expect == "" {
		log.Fatal("-file and -expect-database are required")
	}
	rows, sourceRows, unclassified, err := readWorkbook(*file)
	if err != nil {
		log.Fatal(err)
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		log.Fatal(err)
	}
	hash := sha256.Sum256(b)
	_ = godotenv.Load()
	if *expect != os.Getenv("DB_DATABASE") {
		log.Fatal("database guard: .env target differs from expected database")
	}
	config.InitDB()
	db := config.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	r := report{File: filepath.Base(*file), SHA256: fmt.Sprintf("%x", hash), GeneratedAt: time.Now().UTC(), Apply: *apply, SourceRows: sourceRows, Unclassified: unclassified, Eligible: len(rows), Changes: []change{}, Missing: []string{}, Conflicts: []string{}, YearMismatch: []string{}}
	if err := db.Raw("SELECT DATABASE()").Scan(&r.Database).Error; err != nil {
		log.Fatal(err)
	}
	if r.Database != *expect {
		log.Fatal("database guard: actual database differs from expected database")
	}
	var categories []struct {
		CategoryID uint64
		Name       string
	}
	if err := db.Table("paper_categories").Select("category_id, name").Scan(&categories).Error; err != nil {
		log.Fatal(err)
	}
	catIDs := map[string]uint64{}
	for _, c := range categories {
		name := strings.TrimSpace(c.Name)
		if old, ok := catIDs[name]; ok && old != c.CategoryID {
			log.Fatalf("ambiguous category name %q", name)
		}
		catIDs[name] = c.CategoryID
	}
	var docs []document
	sourceEIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		sourceEIDs = append(sourceEIDs, row.EID)
	}
	if err := db.Table("scopus_benchmark_documents").Select("id,eid,pub_year,category,classification_confidence").Where("eid IN ?", sourceEIDs).Scan(&docs).Error; err != nil {
		log.Fatal(err)
	}
	byEID := map[string]document{}
	for _, d := range docs {
		byEID[d.EID] = d
	}
	for _, row := range rows {
		catID, ok := catIDs[row.Category]
		if !ok {
			log.Fatalf("unmapped category %q", row.Category)
		}
		d, ok := byEID[row.EID]
		if !ok {
			r.Missing = append(r.Missing, row.EID)
			continue
		}
		if d.PubYear != row.Year {
			r.YearMismatch = append(r.YearMismatch, row.EID)
			// Category is attached to the paper, not its publication year.
			// Both source and current DB years are in the requested 2025-2026
			// window. Keep the live publication year and record the difference.
		}
		if d.PubYear < 2025 || d.PubYear > 2026 {
			r.OutsideYears = append(r.OutsideYears, row.EID)
			continue
		}
		r.Matched++
		if d.Category != nil || d.ClassificationConfidence != nil {
			if d.Category != nil && *d.Category == catID && d.ClassificationConfidence != nil && *d.ClassificationConfidence == row.Confidence {
				r.AlreadySame++
			} else {
				r.Conflicts = append(r.Conflicts, row.EID)
			}
			continue
		}
		r.Changes = append(r.Changes, change{classification: row, ID: d.ID, CategoryID: catID})
	}
	before, err := roleFingerprint(db)
	if err != nil {
		log.Fatal(err)
	}
	if *apply {
		// Small atomic batches keep the harvest unblocked. Recheck emptiness in
		// SQL so an intervening classifier cannot be overwritten.
		for start := 0; start < len(r.Changes); start += 250 {
			end := start + 250
			if end > len(r.Changes) {
				end = len(r.Changes)
			}
			parts := []string{}
			args := []interface{}{}
			for _, c := range r.Changes[start:end] {
				parts = append(parts, "SELECT ? AS id, ? AS category, ? AS confidence")
				args = append(args, c.ID, c.CategoryID, c.Confidence)
			}
			args = append(args, "excel_import", "excel_final_2025_2026", r.GeneratedAt)
			result := db.Exec("UPDATE scopus_benchmark_documents d JOIN ("+strings.Join(parts, " UNION ALL ")+") x ON x.id=d.id SET d.category=x.category, d.classification_confidence=x.confidence, d.classification_model=?, d.classification_taxonomy_version=?, d.classified_at=? WHERE d.category IS NULL AND d.classification_confidence IS NULL AND d.pub_year IN (2025,2026)", args...)
			if result.Error != nil {
				log.Fatal(result.Error)
			}
			r.Updated += result.RowsAffected
		}
	}
	after, err := roleFingerprint(db)
	if err != nil {
		log.Fatal(err)
	}
	r.CoreRolesUnchanged = before == after
	if !r.CoreRolesUnchanged {
		log.Fatal("core role fingerprint changed during import; inspect concurrent core ingestion")
	}
	if err := db.Table("scopus_benchmark_harvest_runs").Select("id, status, year_from, year_to, pages_fetched, documents_upserted, requests_made, started_at, finished_at").Order("id DESC").Limit(3).Find(&r.Runs).Error; err != nil {
		log.Fatal(err)
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if *output != "" {
		if err := os.WriteFile(*output, encoded, 0600); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("database=%s source_rows=%d unclassified=%d eligible=%d matched=%d already_same=%d conflicts=%d unmatched=%d year_mismatch=%d outside_years=%d pending=%d updated=%d core_roles_unchanged=%t\n", r.Database, r.SourceRows, r.Unclassified, r.Eligible, r.Matched, r.AlreadySame, len(r.Conflicts), len(r.Missing), len(r.YearMismatch), len(r.OutsideYears), len(r.Changes), r.Updated, r.CoreRolesUnchanged)
	for _, run := range r.Runs {
		v, _ := json.Marshal(run)
		fmt.Println(string(v))
	}
}

func readWorkbook(path string) ([]classification, int, int, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	raw, err := f.GetRows("raw_data")
	if err != nil {
		return nil, 0, 0, err
	}
	if len(raw) == 0 {
		return nil, 0, 0, fmt.Errorf("raw_data is empty")
	}
	cols := headers(raw[0])
	idCol, ok := cols["scopus_id"]
	if !ok {
		return nil, 0, 0, fmt.Errorf("raw_data missing scopus_id")
	}
	eidCol, ok := cols["eid"]
	if !ok {
		return nil, 0, 0, fmt.Errorf("raw_data missing eid")
	}
	eids := map[string]string{}
	for _, r := range raw[1:] {
		id, eid := cell(r, idCol), cell(r, eidCol)
		if id == "" {
			continue
		}
		if eid == "" {
			return nil, 0, 0, fmt.Errorf("missing EID for %s", id)
		}
		if old, ok := eids[id]; ok && old != eid {
			return nil, 0, 0, fmt.Errorf("ambiguous EID for %s", id)
		}
		eids[id] = eid
	}
	data, err := f.GetRows("classified_scopus-benchmark-doc")
	if err != nil {
		return nil, 0, 0, err
	}
	if len(data) == 0 {
		return nil, 0, 0, fmt.Errorf("classification sheet empty")
	}
	cols = headers(data[0])
	for _, h := range []string{"scopus_id", "publication_year", "Category", "classification_confidence"} {
		if _, ok := cols[h]; !ok {
			return nil, 0, 0, fmt.Errorf("missing header %s", h)
		}
	}
	out := []classification{}
	seen := map[string]bool{}
	total, unclassified := 0, 0
	for _, r := range data[1:] {
		id := cell(r, cols["scopus_id"])
		if id == "" {
			continue
		}
		total++
		year, err := strconv.Atoi(cell(r, cols["publication_year"]))
		if err != nil || year < 2025 || year > 2026 {
			return nil, 0, 0, fmt.Errorf("invalid publication year for %s", id)
		}
		eid, ok := eids[id]
		if !ok {
			return nil, 0, 0, fmt.Errorf("missing raw_data match for %s", id)
		}
		if seen[eid] {
			return nil, 0, 0, fmt.Errorf("duplicate EID %s", eid)
		}
		seen[eid] = true
		cat, confidence := cell(r, cols["Category"]), cell(r, cols["classification_confidence"])
		if cat == "" && confidence == "Needs Review" {
			unclassified++
			continue
		}
		if cat == "" || !validConfidence(confidence) {
			return nil, 0, 0, fmt.Errorf("unsupported classification for %s: %q / %q", id, cat, confidence)
		}
		out = append(out, classification{EID: eid, Year: year, Category: cat, Confidence: confidence})
	}
	return out, total, unclassified, nil
}
func headers(row []string) map[string]int {
	m := map[string]int{}
	for i, v := range row {
		m[strings.TrimSpace(v)] = i
	}
	return m
}
func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
func validConfidence(v string) bool {
	return v == "High" || v == "Medium" || v == "Low" || v == "Preface"
}
func roleFingerprint(db *gorm.DB) (string, error) {
	var out string
	err := db.Raw(`SELECT CONCAT(COUNT(*),':',COALESCE(SUM(CRC32(CONCAT_WS('|',r.id,r.document_id,r.author_id,COALESCE(r.is_first_author,'NULL'),COALESCE(r.is_corresponding_author,'NULL'),COALESCE(d.author_role_status,'NULL'),COALESCE(d.author_role_checked_at,'NULL')))),0)) FROM scopus_document_authors r JOIN scopus_documents d ON d.id=r.document_id`).Scan(&out).Error
	return out, err
}
