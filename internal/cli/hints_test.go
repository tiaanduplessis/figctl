package cli

import "testing"

func TestJoinSentences(t *testing.T) {
	tests := []struct {
		name        string
		first, hint string
		want        string
	}{
		{"adds the missing full stop", "file ABC not found", "Check the key.", "file ABC not found. Check the key."},
		{"keeps existing punctuation", "Not allowed.", "Try --profile.", "Not allowed. Try --profile."},
		{"keeps a question mark", "Missing token?", "Log in.", "Missing token? Log in."},
		{"keeps a colon", "Reason:", "no access", "Reason: no access"},
		{"no hint returns the message", "file ABC not found", "", "file ABC not found"},
		{"no message returns the hint", "", "Check the key.", "Check the key."},
		{"trims surrounding space", "  broken  ", "  fix it.  ", "broken. fix it."},
		{"both empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinSentences(tt.first, tt.hint); got != tt.want {
				t.Errorf("joinSentences(%q, %q) = %q, want %q", tt.first, tt.hint, got, tt.want)
			}
		})
	}
}
