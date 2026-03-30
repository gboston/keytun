// ABOUTME: Tests for formatting helpers in the host command.
// ABOUTME: Covers duration and byte formatting functions.
package cmd

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},           // rounds to 0
		{1 * time.Second, "1s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m0s"},
		{61 * time.Second, "1m1s"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h0m0s"},
		{3661 * time.Second, "1h1m1s"},
		{7200*time.Second + 30*time.Minute + 45*time.Second, "2h30m45s"},
	}

	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 bytes"},
		{1, "1 bytes"},
		{512, "512 bytes"},
		{1023, "1023 bytes"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{10240, "10.0 KB"},
		{1048576, "1024.0 KB"},
	}

	for _, tc := range tests {
		got := formatBytes(tc.n)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
