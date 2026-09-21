package common

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"negative", -42, "0 B"},
		{"bytes", 512, "512 B"},
		{"kb", 1024, "1.00 KB"},
		{"mb", 1024 * 1024, "1.00 MB"},
		{"gb", 1024 * 1024 * 1024, "1.00 GB"},
		{"tb", 1024 * 1024 * 1024 * 1024, "1.00 TB"},
		{"gb_fraction", 1536 * 1024 * 1024, "1.50 GB"},
		{"mb_fraction", 3 * 1024 * 1024 / 2, "1.50 MB"},
		{"mid_kb", 2048 + 512, "2.50 KB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatBytes(tc.bytes); got != tc.want {
				t.Fatalf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}
