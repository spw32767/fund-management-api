package main

import (
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"strings"
	"testing"
)

func TestTargetGuard(t *testing.T) {
	for _, apply := range []bool{false, true} {
		for _, target := range []string{"", "other", " dev "} {
			if validateTarget(apply, target, "dev") == nil {
				t.Fatalf("accepted apply=%v target=%q", apply, target)
			}
		}
		if validateTarget(apply, "dev", "dev") != nil {
			t.Fatal("exact target rejected")
		}
	}
}

func TestLockRetryHintsAreSanitized(t *testing.T) {
	for _, code := range []uint16{1213, 1205} {
		err := fmt.Errorf("document failed: %w", &driver.MySQLError{Number: code, Message: "sensitive source error"})
		got := lockRetryHint(err)
		if !strings.Contains(got, "retryable lock conflict") || strings.Contains(got, "sensitive") {
			t.Fatal(got)
		}
	}
	if strings.Contains(lockRetryHint(fmt.Errorf("other")), "retryable lock conflict") {
		t.Fatal("non-lock error classified as retryable")
	}
}
