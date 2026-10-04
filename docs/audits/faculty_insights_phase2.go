// Read-only parity check using the production normalizer and unchanged dashboard gate.
// Run from backend root: go run docs/audits/faculty_insights_phase2.go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"fund-management-api/services"
	driver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"os"
	"time"
)

func main() {
	if err := audit(); err != nil {
		fmt.Fprintln(os.Stderr, "read-only parity audit failed (connection/DB identity withheld)")
		os.Exit(1)
	}
}
func audit() error {
	_ = godotenv.Load()
	c := driver.NewConfig()
	c.User = os.Getenv("DB_USERNAME")
	c.Passwd = os.Getenv("DB_PASSWORD")
	c.Net = "tcp"
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "3306"
	}
	c.Addr = os.Getenv("DB_HOST") + ":" + port
	c.DBName = os.Getenv("DB_DATABASE")
	c.ParseTime = true
	c.Timeout = 10 * time.Second
	c.ReadTimeout = 45 * time.Second
	db, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cat := map[string]string{}
	rows, err := tx.Query(`SELECT afid,COALESCE(country,'') FROM scopus_affiliations ORDER BY afid LIMIT 100000`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, country string
		if err = rows.Scan(&id, &country); err != nil {
			return err
		}
		cat[id] = country
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(cat) == 100000 {
		return fmt.Errorf("catalogue cap reached")
	}
	rows, err = tx.Query(`SELECT sd.raw_json FROM scopus_documents sd WHERE COALESCE(YEAR(sd.cover_date),CAST(RIGHT(sd.cover_display_date,4) AS UNSIGNED)) BETWEEN 2024 AND 2026 AND EXISTS (SELECT 1 FROM scopus_document_authors sda JOIN scopus_authors sa ON sa.id=sda.author_id JOIN users u ON TRIM(u.scopus_id)=sa.scopus_author_id JOIN scopus_affiliations aff ON aff.id=sda.affiliation_id WHERE sda.document_id=sd.id AND u.delete_at IS NULL AND u.is_test=0 AND u.scopus_id IS NOT NULL AND TRIM(u.scopus_id)<>'' AND LOWER(TRIM(COALESCE(aff.name,''))) IN ('khon kaen university','faculty of science, khon kaen university')) ORDER BY sd.id LIMIT 100000`)
	if err != nil {
		return err
	}
	counts := map[string]int{"total": 0, "yes": 0, "no": 0, "unknown": 0}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		n := services.NormalizeScopusInsight(raw, cat)
		counts["total"]++
		if n.International == nil {
			counts["unknown"]++
		} else if *n.International {
			counts["yes"]++
		} else {
			counts["no"]++
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if counts["total"] == 100000 {
		return fmt.Errorf("document cap reached")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"mode": "read_only_parity_audit", "environment": os.Getenv("ENVIRONMENT"), "normalizer_version": services.ScopusInsightNormalizerVersion, "faculty_2567_2569": counts, "audited_at_utc": time.Now().UTC()})
}
