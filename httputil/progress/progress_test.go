package progress

import (
	"testing"
)

func TestFormatMb(t *testing.T) {
	type test struct {
		input int64
		want  string
	}
	tests := []test{
		{input: 48*1024*1024 + 512, want: "48 MB"},
		{input: 58538527, want: "55 MB"},
		{input: 0, want: "0 MB"},
		{input: 48*1024*1024 - 1, want: "47 MB"},
		{input: 48 * 1024 * 1024, want: "48 MB"},
		{input: 48 * 1024 * 1024 * 1024, want: "49152 MB"},
	}

	for _, tc := range tests {
		name := tc.want
		t.Run(name, func(t *testing.T) {
			got := formatMb(tc.input)
			if got != tc.want {
				t.Errorf("formatMb() = %q, want %q", got ,tc.want)
			}
		})
	}
}

func TestFormatPercentage(t *testing.T) {
	type test struct {
		curr, total int64
		want        string
	}
	tests := []test{
		{curr: 0, total: 1000, want: "0%"},
		{curr: 1000, total: 1000, want: "100%"},
		{curr: 500, total: 1000, want: "50%"},
	}

	for _, tc := range tests {
		name := tc.want
		t.Run(name, func(t *testing.T) {
			got := formatPercentage(tc.curr, tc.total)
			if got != tc.want {
				t.Fatalf("formatPercentage() = %q, want %q", got, tc.want)
			}
		})
	}
}
