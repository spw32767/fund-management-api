package services

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

// releaseNamedLock releases a MySQL advisory lock (GET_LOCK/RELEASE_LOCK) as a best-effort
// cleanup. RELEASE_LOCK returns 1 when this session released the lock, 0 when another session
// holds it, and NULL when the lock does not exist at all — which happens on long-running jobs
// when the pooled connection that acquired the lock is recycled/closed, auto-releasing it.
//
// The lock is only a guard against concurrent runs, so none of those outcomes is worth failing
// on; we scan into a nullable value (a plain int cannot hold NULL) and never surface it as an
// error. Only an actual query/connection error is returned.
func releaseNamedLock(ctx context.Context, db *gorm.DB, lockName string) error {
	var released sql.NullInt64
	return db.WithContext(ctx).Raw("SELECT RELEASE_LOCK(?)", lockName).Scan(&released).Error
}
