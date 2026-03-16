package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"fund-management-api/config"

	"github.com/gin-gonic/gin"
)

func GetSupportFundMappings(c *gin.Context) {
	rows, columns, err := listSupportFundMappings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to fetch support fund mappings",
			"details": err.Error(),
		})
		return
	}

	labels, searchableColumns, visibleColumns := getSupportFundMappingMetadata(columns)

	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"count":              len(rows),
		"columns":            columns,
		"data":               rows,
		"column_labels":      labels,
		"searchable_columns": searchableColumns,
		"visible_columns":    visibleColumns,
	})
}

func listSupportFundMappings() ([]map[string]interface{}, []string, error) {
	queryRows, err := openSupportFundMappingRows()
	if err != nil {
		return nil, nil, err
	}
	defer queryRows.Close()

	columns, err := queryRows.Columns()
	if err != nil {
		return nil, nil, err
	}

	result := make([]map[string]interface{}, 0)
	values := make([]interface{}, len(columns))
	valuePointers := make([]interface{}, len(columns))

	for queryRows.Next() {
		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := queryRows.Scan(valuePointers...); err != nil {
			return nil, nil, err
		}

		row := make(map[string]interface{}, len(columns))
		for i, column := range columns {
			row[column] = normalizeSupportFundMappingValue(values[i])
		}

		result = append(result, row)
	}

	if err := queryRows.Err(); err != nil {
		return nil, nil, err
	}

	sortSupportFundMappings(result)

	return result, columns, nil
}

func openSupportFundMappingRows() (*sql.Rows, error) {
	queries := []string{
		"SELECT * FROM support_fundmapping",
		"SELECT * FROM fund_cpkku.support_fundmapping",
	}

	var lastErr error
	for _, query := range queries {
		rows, err := config.DB.Raw(query).Rows()
		if err == nil {
			return rows, nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("support_fundmapping query failed")
	}

	return nil, lastErr
}

func sortSupportFundMappings(rows []map[string]interface{}) {
	sort.Slice(rows, func(i, j int) bool {
		left := normalizeForSort(rows[i]["name"])
		right := normalizeForSort(rows[j]["name"])

		if left == right {
			return fmt.Sprint(rows[i]["id"]) < fmt.Sprint(rows[j]["id"])
		}

		if left == "" {
			return false
		}

		if right == "" {
			return true
		}

		return left < right
	})
}

func normalizeForSort(value interface{}) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(strings.ToLower(fmt.Sprint(value)))
}

func normalizeSupportFundMappingValue(raw interface{}) interface{} {
	switch value := raw.(type) {
	case []byte:
		return string(value)
	case time.Time:
		return value.Format(time.RFC3339)
	default:
		return value
	}
}

func getSupportFundMappingMetadata(columns []string) (map[string]string, []string, []string) {
	labels := make(map[string]string, len(columns))
	for _, column := range columns {
		labels[column] = column
	}

	commentMap, err := loadSupportFundMappingColumnComments()
	if err == nil {
		for column, comment := range commentMap {
			if _, exists := labels[column]; exists {
				labels[column] = comment
			}
		}
	}

	searchableColumns := make([]string, 0)
	visibleColumns := make([]string, 0)

	for _, column := range columns {
		columnLower := strings.ToLower(strings.TrimSpace(column))
		label := strings.TrimSpace(labels[column])

		if labelContainsThai(label) {
			searchableColumns = append(searchableColumns, column)
			if columnLower != "keyword" {
				visibleColumns = append(visibleColumns, column)
			}
		}
	}

	if len(searchableColumns) == 0 {
		for _, column := range columns {
			columnLower := strings.ToLower(strings.TrimSpace(column))
			if columnLower == "req_id" || columnLower == "req_code" || columnLower == "create_date" {
				continue
			}
			searchableColumns = append(searchableColumns, column)
			if columnLower != "keyword" {
				visibleColumns = append(visibleColumns, column)
			}
		}
	}

	return labels, searchableColumns, visibleColumns
}

func loadSupportFundMappingColumnComments() (map[string]string, error) {
	type columnCommentRow struct {
		ColumnName    string
		ColumnComment string
	}

	queries := []struct {
		query string
		args  []interface{}
	}{
		{
			query: `
				SELECT column_name, column_comment
				FROM information_schema.columns
				WHERE table_schema = DATABASE()
					AND table_name = 'support_fundmapping'
			`,
		},
		{
			query: `
				SELECT column_name, column_comment
				FROM information_schema.columns
				WHERE table_schema = ?
					AND table_name = 'support_fundmapping'
			`,
			args: []interface{}{"fund_cpkku"},
		},
	}

	var lastErr error
	for _, q := range queries {
		rows := make([]columnCommentRow, 0)
		err := config.DB.Raw(q.query, q.args...).Scan(&rows).Error
		if err != nil {
			lastErr = err
			continue
		}

		if len(rows) == 0 {
			continue
		}

		result := make(map[string]string, len(rows))
		for _, row := range rows {
			columnName := strings.TrimSpace(row.ColumnName)
			columnComment := strings.TrimSpace(row.ColumnComment)
			if columnName == "" || columnComment == "" {
				continue
			}
			result[columnName] = columnComment
		}

		if len(result) > 0 {
			return result, nil
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("column comments not found")
	}

	return map[string]string{}, lastErr
}

func labelContainsThai(value string) bool {
	for _, r := range value {
		if r >= 0x0E00 && r <= 0x0E7F {
			return true
		}
	}

	return false
}
