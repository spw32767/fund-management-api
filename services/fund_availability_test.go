package services

import "testing"

func TestIsFundStatusOpen(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "active", status: "active", want: true},
		{name: "active ignores casing and whitespace", status: " Active ", want: true},
		{name: "disabled", status: "disable", want: false},
		{name: "empty", status: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFundStatusOpen(tt.status); got != tt.want {
				t.Fatalf("IsFundStatusOpen(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
