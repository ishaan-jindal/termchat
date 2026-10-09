package shared

import "testing"

func TestIsValidReaction(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"+1", true},
		{"-1", true},
		{"laugh", true},
		{"heart", true},
		{"wow", true},
		{"eyes", true},
		{"fire", true},
		{"clap", true},
		{"", false},
		{"unknown", false},
		{"Laugh", false},
		{"HEART", false},
		{"Clap", false},
		{"heart ", false},
		{" heart", false},
		{"+ 1", false},
		{"fire\n", false},
	}

	for _, tc := range cases {
		if got := IsValidReaction(tc.name); got != tc.valid {
			t.Errorf("IsValidReaction(%q) = %v, want %v", tc.name, got, tc.valid)
		}
	}
}
