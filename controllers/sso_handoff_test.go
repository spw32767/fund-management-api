package controllers

import (
	"net/url"
	"strings"
	"testing"
)

func TestValidateReturnTo(t *testing.T) {
	const allowed = "https://academic.computing.kku.ac.th"

	t.Run("disabled when no allowlist", func(t *testing.T) {
		// SSO_HANDOFF_ALLOWED_ORIGINS unset -> feature off, everything rejected.
		if got := validateReturnTo(allowed + "/auth/callback"); got != "" {
			t.Fatalf("expected empty when feature disabled, got %q", got)
		}
	})

	t.Run("allowed origin passes and is returned verbatim", func(t *testing.T) {
		t.Setenv("SSO_HANDOFF_ALLOWED_ORIGINS", allowed)
		in := allowed + "/auth/callback?next=/dashboard"
		if got := validateReturnTo(in); got != in {
			t.Fatalf("expected %q, got %q", in, got)
		}
	})

	t.Run("host is matched case-insensitively", func(t *testing.T) {
		t.Setenv("SSO_HANDOFF_ALLOWED_ORIGINS", allowed)
		in := "https://ACADEMIC.Computing.KKU.ac.th/auth/callback"
		if got := validateReturnTo(in); got != in {
			t.Fatalf("expected mixed-case host to be allowed, got %q", got)
		}
	})

	t.Run("multiple origins in allowlist", func(t *testing.T) {
		t.Setenv("SSO_HANDOFF_ALLOWED_ORIGINS", "https://a.example.com , "+allowed+"/")
		in := allowed + "/cb"
		if got := validateReturnTo(in); got != in {
			t.Fatalf("expected allowed among many, got %q", got)
		}
	})

	t.Run("rejects disallowed origin", func(t *testing.T) {
		t.Setenv("SSO_HANDOFF_ALLOWED_ORIGINS", allowed)
		if got := validateReturnTo("https://evil.example.com/steal"); got != "" {
			t.Fatalf("expected empty for disallowed origin, got %q", got)
		}
	})

	t.Run("rejects non-https", func(t *testing.T) {
		t.Setenv("SSO_HANDOFF_ALLOWED_ORIGINS", allowed)
		if got := validateReturnTo("http://academic.computing.kku.ac.th/cb"); got != "" {
			t.Fatalf("expected empty for http scheme, got %q", got)
		}
	})

	t.Run("rejects empty and malformed", func(t *testing.T) {
		t.Setenv("SSO_HANDOFF_ALLOWED_ORIGINS", allowed)
		for _, in := range []string{"", "   ", "://nope", "not a url", "/relative/only"} {
			if got := validateReturnTo(in); got != "" {
				t.Fatalf("expected empty for %q, got %q", in, got)
			}
		}
	})
}

func TestAppendTicketToURL(t *testing.T) {
	t.Run("adds ticket when no existing query", func(t *testing.T) {
		out := appendTicketToURL("https://academic.computing.kku.ac.th/auth/callback", "abc123")
		u, err := url.Parse(out)
		if err != nil {
			t.Fatalf("result not parseable: %v", err)
		}
		if u.Query().Get("ticket") != "abc123" {
			t.Fatalf("ticket not set correctly: %s", out)
		}
	})

	t.Run("preserves existing query params", func(t *testing.T) {
		out := appendTicketToURL("https://academic.computing.kku.ac.th/cb?next=%2Fhome", "t-9")
		u, err := url.Parse(out)
		if err != nil {
			t.Fatalf("result not parseable: %v", err)
		}
		if u.Query().Get("ticket") != "t-9" {
			t.Fatalf("ticket missing: %s", out)
		}
		if u.Query().Get("next") != "/home" {
			t.Fatalf("existing query dropped: %s", out)
		}
		if !strings.HasPrefix(out, "https://academic.computing.kku.ac.th/cb?") {
			t.Fatalf("unexpected base: %s", out)
		}
	})
}
