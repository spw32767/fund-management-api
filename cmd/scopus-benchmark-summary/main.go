// Offline report preparation. No Scopus API requests are made.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"fund-management-api/config"
	"fund-management-api/services"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	migration := flag.String("migrate", "", "SQL migration file to apply")
	verify := flag.String("verify-migration", "", "test migration on fresh isolated fixture tables, rerun and clean up")
	backfill := flag.Bool("backfill", false, "recover affiliation metadata from stored JSON only")
	verifyBackfill := flag.Bool("verify-backfill", false, "with -backfill: verify rerun stability and untouched classification/core roles")
	expect := flag.String("expect-database", "", "required for writes: exact database name to protect the target")
	audit := flag.Bool("audit", false, "print default report and an all-category Journal coverage audit")
	yearFrom := flag.Int("year-from", time.Now().Year()-1, "first report year")
	yearTo := flag.Int("year-to", time.Now().Year(), "last report year")
	flag.Parse()
	_ = godotenv.Load()
	if (*migration != "" || *verify != "" || *backfill) && (*expect == "" || *expect != os.Getenv("DB_DATABASE")) {
		log.Fatal("writes require -expect-database matching DB_DATABASE")
	}
	config.InitDB()
	db := config.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if *verify != "" {
		if e := verifyMigration(db, *verify); e != nil {
			log.Fatal(e)
		}
		fmt.Println("fresh schema and rerun preservation verified; isolated fixture tables removed")
	}
	if *migration != "" {
		b, e := os.ReadFile(*migration)
		if e != nil {
			log.Fatal(e)
		}
		e = applySQL(db, string(b))
		if e != nil {
			log.Fatal(e)
		}
		fmt.Println("migration applied")
	}
	svc := services.NewScopusBenchmarkService(db, nil)
	if *backfill {
		var before *services.BenchmarkSummaryReport
		var beforeClass, beforeRoles string
		if *verifyBackfill {
			f, _ := services.ParseBenchmarkSummaryFilter(url.Values{"category": {"all"}}, time.Now())
			var e error
			before, e = svc.BenchmarkSummary(context.Background(), f)
			if e != nil {
				log.Fatal(e)
			}
			beforeClass, beforeRoles, e = protectedFingerprint(db)
			if e != nil {
				log.Fatal(e)
			}
		}
		out, e := svc.BackfillSummaryAffiliations(context.Background())
		if e != nil {
			log.Fatal(e)
		}
		printJSON(out)
		if *verifyBackfill {
			after, e := svc.BenchmarkSummary(context.Background(), before.Filters)
			if e != nil {
				log.Fatal(e)
			}
			afterClass, afterRoles, e := protectedFingerprint(db)
			if e != nil {
				log.Fatal(e)
			}
			if before.Revision != after.Revision || beforeClass != afterClass || beforeRoles != afterRoles {
				log.Fatal("rerun verification changed report/classification/core-role fingerprint")
			}
			fmt.Println("backfill rerun stable; benchmark classifications and core XML roles unchanged")
		}
	}
	if *audit {
		q := url.Values{"year_from": {fmt.Sprint(*yearFrom)}, "year_to": {fmt.Sprint(*yearTo)}}
		for _, category := range []string{"classified", "all"} {
			q.Set("category", category)
			f, e := services.ParseBenchmarkSummaryFilter(q, time.Now())
			if e != nil {
				log.Fatal(e)
			}
			r, e := svc.BenchmarkSummary(context.Background(), f)
			if e != nil {
				log.Fatal(e)
			}
			printJSON(map[string]interface{}{"filters": r.Filters, "years": r.Years, "yearly": r.Yearly, "total": r.Total, "coverage": r.Coverage, "faculty_roles": r.FacultyRoles, "revision": r.Revision})
		}
	}
	if *migration == "" && *verify == "" && !*backfill && !*audit {
		flag.Usage()
	}
}
func protectedFingerprint(db *gorm.DB) (string, string, error) {
	var classification, roles string
	e := db.Raw(`SELECT CONCAT(COUNT(*),':',COALESCE(SUM(CRC32(CONCAT_WS('|',id,COALESCE(category,'NULL'),COALESCE(classification_confidence,'NULL'),COALESCE(classification_model,'NULL'),COALESCE(classification_taxonomy_version,'NULL'),COALESCE(classified_at,'NULL')))),0)) FROM scopus_benchmark_documents`).Scan(&classification).Error
	if e != nil {
		return "", "", e
	}
	e = db.Raw(`SELECT CONCAT(COUNT(*),':',COALESCE(SUM(CRC32(CONCAT_WS('|',r.id,r.document_id,r.author_id,COALESCE(r.is_first_author,'NULL'),COALESCE(r.is_corresponding_author,'NULL'),COALESCE(d.author_role_status,'NULL'),COALESCE(d.author_role_checked_at,'NULL')))),0)) FROM scopus_document_authors r JOIN scopus_documents d ON d.id=r.document_id`).Scan(&roles).Error
	return classification, roles, e
}

func applySQL(db *gorm.DB, script string) error {
	lines := []string{}
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	// Prepared statements are connection-scoped, not pooled across executions.
	return db.Connection(func(tx *gorm.DB) error {
		for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if e := tx.Exec(stmt).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
func verifyMigration(db *gorm.DB, path string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	prefix := fmt.Sprintf("summary_test_%d_", time.Now().UnixNano())
	tables := []string{"scopus_benchmark_document_affiliations", "scopus_benchmark_author_affiliations", "scopus_benchmark_document_authors", "scopus_benchmark_documents", "paper_categories"}
	script := string(b)
	names := map[string]string{}
	for i, table := range tables {
		name := fmt.Sprintf("%s%d", prefix, i)
		names[table] = name
		script = strings.ReplaceAll(script, table, name)
	}
	defer func() {
		for _, table := range tables {
			name := names[table]
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if e := db.Exec("DROP TABLE IF EXISTS " + name).Error; e != nil {
				log.Printf("fixture cleanup %s: %v", name, e)
			}
		}
	}()
	for _, table := range []string{"scopus_benchmark_documents", "scopus_benchmark_document_authors"} {
		if e := db.Exec("CREATE TABLE " + names[table] + " (id BIGINT UNSIGNED PRIMARY KEY)").Error; e != nil {
			return e
		}
	}
	if e = applySQL(db, script); e != nil {
		return e
	}
	table := names["scopus_benchmark_documents"]
	if e = db.Exec("INSERT INTO " + table + " (id,category,classification_confidence,classification_model,classification_taxonomy_version,classified_at) VALUES (1,123,'Preface','model','v1','2026-01-01')").Error; e != nil {
		return e
	}
	if e = db.Exec("UPDATE " + names["paper_categories"] + " SET name='custom name',is_active=0 WHERE code='AI_ALGORITHMS'").Error; e != nil {
		return e
	}
	if e = applySQL(db, script); e != nil {
		return e
	}
	var count int64
	if e = db.Table(table).Where("id=1 AND category=123 AND classification_confidence='Preface' AND classification_model='model' AND classification_taxonomy_version='v1' AND classified_at='2026-01-01'").Count(&count).Error; e != nil {
		return e
	}
	if count != 1 {
		return fmt.Errorf("migration erased classification fixture")
	}
	if e = db.Table(names["paper_categories"]).Where("code='AI_ALGORITHMS' AND name='custom name' AND is_active=0").Count(&count).Error; e != nil {
		return e
	}
	if count != 1 {
		return fmt.Errorf("migration changed existing taxonomy")
	}
	return nil
}
func printJSON(v interface{}) {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		log.Fatal(e)
	}
	fmt.Println(string(b))
}
