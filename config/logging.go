package config

import "path/filepath"

// LogFilePath returns the path to the backend log file.
func LogFilePath() string {
	return filepath.Join("logs", "fund-api.log")
}
