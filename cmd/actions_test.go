package cmd

import "testing"

func TestValidateActionName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"plain name", "doctor", false},
		{"dashes and digits", "deploy-v2", false},
		{"surrounding space is trimmed", "  doctor  ", false},
		{"empty", "", true},
		{"only whitespace", "   ", true},
		{"inner space", "run tests", true},
		{"inner tab", "run\ttests", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateActionName(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("validateActionName(%q) = nil, want error", tc.in)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateActionName(%q) = %v, want nil", tc.in, err)
			}
		})
	}
}

// The headless add path resolves the action name from --name; without the flag
// registered on `actions add` there is no way to name an action without a TTY.
func TestActionsAddHasNameFlag(t *testing.T) {
	if actionsAddCmd.Flags().Lookup("name") == nil {
		t.Fatal("actions add is missing the --name flag")
	}
}
