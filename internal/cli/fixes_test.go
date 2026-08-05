package cli_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/cli"
)

func TestPreparseFixes(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "no fixes flag is untouched",
			in:   []string{"commit", "--dry-run"},
			want: []string{"commit", "--dry-run"},
		},
		{
			name: "the space-separated form is folded",
			in:   []string{"commit", "--fixes", "123", "456"},
			want: []string{"commit", "--fixes", "123,456"},
		},
		{
			name: "the short form is folded too",
			in:   []string{"commit", "-f", "123", "456", "789"},
			want: []string{"commit", "-f", "123,456,789"},
		},
		{
			name: "the comma form is already valid and left alone",
			in:   []string{"commit", "--fixes", "123,456"},
			want: []string{"commit", "--fixes", "123,456"},
		},
		{
			name: "a comma form followed by bare numbers absorbs them",
			in:   []string{"commit", "--fixes", "123,456", "789"},
			want: []string{"commit", "--fixes", "123,456,789"},
		},
		{
			name: "the repeated form is left alone",
			in:   []string{"commit", "--fixes", "123", "--fixes", "456"},
			want: []string{"commit", "--fixes", "123", "--fixes", "456"},
		},
		{
			name: "a non-numeric argument terminates absorption",
			in:   []string{"commit", "--fixes", "123", "456", "--dry-run"},
			want: []string{"commit", "--fixes", "123,456", "--dry-run"},
		},
		{
			name: "a leading hash is stripped",
			in:   []string{"commit", "--fixes", "#123", "#456"},
			want: []string{"commit", "--fixes", "123,456"},
		},
		{
			name: "the equals form is left alone",
			in:   []string{"commit", "--fixes=123,456"},
			want: []string{"commit", "--fixes=123,456"},
		},
		{
			name: "the equals form still absorbs following bare numbers",
			in:   []string{"commit", "--fixes=123", "456"},
			want: []string{"commit", "--fixes=123,456"},
		},
		{
			name: "numbers after an unrelated flag are untouched",
			in:   []string{"commit", "--model", "qwen3:4b", "123"},
			want: []string{"commit", "--model", "qwen3:4b", "123"},
		},
		{
			name: "arguments after a bare double dash are untouched",
			in:   []string{"commit", "--", "--fixes", "123", "456"},
			want: []string{"commit", "--", "--fixes", "123", "456"},
		},
		{
			name: "a short flag cluster ending in f is recognized",
			in:   []string{"commit", "-nf", "123", "456"},
			want: []string{"commit", "-nf", "123,456"},
		},
		{
			name: "a short flag cluster not ending in f is not",
			in:   []string{"commit", "-fn", "123", "456"},
			want: []string{"commit", "-fn", "123", "456"},
		},
		{
			name: "a trailing fixes flag with no value is left alone",
			in:   []string{"commit", "--fixes"},
			want: []string{"commit", "--fixes"},
		},
		{
			name: "an empty argument list",
			in:   []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cli.PreparseFixes(tt.in)

			if len(got) != len(tt.want) {
				t.Fatalf("PreparseFixes(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("PreparseFixes(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPreparseFixesDoesNotMutateItsInput(t *testing.T) {
	in := []string{"commit", "--fixes", "123", "456"}
	original := strings.Join(in, " ")

	cli.PreparseFixes(in)

	if strings.Join(in, " ") != original {
		t.Errorf("PreparseFixes mutated its argument: %v", in)
	}
}
