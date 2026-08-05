package commit_test

import (
	"strings"
	"testing"

	"github.com/guerrero/gitia/internal/commit"
	"github.com/guerrero/gitia/internal/rules"
)

func TestRepairFixesMechanicalViolations(t *testing.T) {
	rs := rules.Conventional()

	tests := []struct {
		name string
		in   commit.Message
		want commit.Message
	}{
		{
			name: "lowercases the type",
			in:   commit.Message{Type: "Feat", Subject: "add a thing"},
			want: commit.Message{Type: "feat", Subject: "add a thing"},
		},
		{
			name: "strips a trailing period from the subject",
			in:   commit.Message{Type: "fix", Subject: "correct the path."},
			want: commit.Message{Type: "fix", Subject: "correct the path"},
		},
		{
			name: "trims surrounding whitespace",
			in:   commit.Message{Type: " fix ", Scope: " cli ", Subject: "  correct the path  "},
			want: commit.Message{Type: "fix", Scope: "cli", Subject: "correct the path"},
		},
		{
			name: "strips a type prefix the model duplicated into the subject",
			in:   commit.Message{Type: "feat", Subject: "feat: add a thing"},
			want: commit.Message{Type: "feat", Subject: "add a thing"},
		},
		{
			name: "drops empty body paragraphs",
			in:   commit.Message{Type: "fix", Subject: "x", Body: []string{"real", "  ", ""}},
			want: commit.Message{Type: "fix", Subject: "x", Body: []string{"real"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commit.Repair(tt.in, rs)
			if got.Type != tt.want.Type {
				t.Errorf("Type = %q, want %q", got.Type, tt.want.Type)
			}
			if got.Scope != tt.want.Scope {
				t.Errorf("Scope = %q, want %q", got.Scope, tt.want.Scope)
			}
			if got.Subject != tt.want.Subject {
				t.Errorf("Subject = %q, want %q", got.Subject, tt.want.Subject)
			}
			if len(got.Body) != len(tt.want.Body) {
				t.Fatalf("Body = %q, want %q", got.Body, tt.want.Body)
			}
			for i := range tt.want.Body {
				if got.Body[i] != tt.want.Body[i] {
					t.Errorf("Body[%d] = %q, want %q", i, got.Body[i], tt.want.Body[i])
				}
			}
		})
	}
}

func TestRepairLowercasesSubjectOnlyWhenRequired(t *testing.T) {
	rs := rules.Conventional()
	in := commit.Message{Type: "feat", Subject: "Add A Thing"}

	if got := commit.Repair(in, rs); got.Subject != "Add A Thing" {
		t.Errorf("Subject = %q; CaseAny must leave the subject alone", got.Subject)
	}

	rs.SubjectCase = rules.CaseLower
	if got := commit.Repair(in, rs); got.Subject != "add a thing" {
		t.Errorf("Subject = %q, want %q", got.Subject, "add a thing")
	}
}

func TestRepairIsIdempotent(t *testing.T) {
	rs := rules.Conventional()
	in := commit.Message{Type: " Feat ", Subject: "feat: Do The Thing.", Body: []string{" p ", ""}}

	once := commit.Repair(in, rs)
	twice := commit.Repair(once, rs)

	if commit.Render(once, rs) != commit.Render(twice, rs) {
		t.Errorf("Repair is not idempotent:\nonce:  %q\ntwice: %q",
			commit.Render(once, rs), commit.Render(twice, rs))
	}
}

func TestRepairDoesNotInventATypeItCannotFix(t *testing.T) {
	rs := rules.Conventional()
	got := commit.Repair(commit.Message{Type: "feature", Subject: "x"}, rs)

	if got.Type != "feature" {
		t.Errorf("Type = %q; repair must not guess a replacement for an unknown type", got.Type)
	}
	if len(commit.Validate(got, rs)) == 0 {
		t.Error("Validate found no violation; an unknown type must survive repair as a violation")
	}
}

func TestRepairTruncatesAnOverlongHeaderAtAWordBoundary(t *testing.T) {
	rs := rules.Conventional()
	subject := strings.TrimSpace(strings.Repeat("word ", 30))

	got := commit.Repair(commit.Message{Type: "feat", Subject: subject}, rs)

	if n := len(commit.Header(got)); n > rs.HeaderMaxLength {
		t.Errorf("header is %d characters after repair, limit is %d", n, rs.HeaderMaxLength)
	}
	if strings.HasSuffix(got.Subject, " ") || strings.HasSuffix(got.Subject, ".") {
		t.Errorf("Subject = %q; truncation must leave no trailing space or period", got.Subject)
	}
}
