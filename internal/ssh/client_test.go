package ssh

import "testing"

func TestParseHashLine(t *testing.T) {
	const hash = "98ea6e4f216f2fb4b69fff9b3a44842c38686ca685f3f55dc48c5d3fb1107be4"

	cases := []struct {
		name     string
		line     string
		wantName string
		wantOK   bool
	}{
		{"plain", hash + "  /srv/app/a.txt", "/srv/app/a.txt", true},
		{"path with spaces", hash + "  /srv/app/b c.txt", "/srv/app/b c.txt", true},
		{"binary mode star", hash + " */srv/app/a.bin", "/srv/app/a.bin", true},
		{"escaped path", `\` + hash + "  /srv/app/a b.txt", "/srv/app/a b.txt", true},
		{"blank", "", "", false},
		{"no path", hash, "", false},
		{"truncated hash", "98ea6e4f  /srv/app/a.txt", "", false},
		{"error message", "sha256sum: /srv/app/x: No such file or directory", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, name, ok := parseHashLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("parseHashLine(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if name != tc.wantName {
				t.Fatalf("parseHashLine(%q) name = %q, want %q", tc.line, name, tc.wantName)
			}
		})
	}
}

func TestParseStatLine(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantName  string
		wantSize  int64
		wantMtime int64
		wantOK    bool
	}{
		{"gnu and bsd share the layout", "275140608 1789077892 /srv/app/odoo.deb", "/srv/app/odoo.deb", 275140608, 1789077892, true},
		{"path with spaces", "3 1789077892 /srv/app/b c.txt", "/srv/app/b c.txt", 3, 1789077892, true},
		{"empty line", "", "", 0, 0, false},
		{"missing path", "3 1789077892", "", 0, 0, false},
		{"error message", "stat: /srv/app/x: No such file or directory", "", 0, 0, false},
		{"non-numeric size", "x 1789077892 /srv/app/a", "", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, st, ok := parseStatLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("parseStatLine(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if name != tc.wantName || st.Size != tc.wantSize || st.ModTime != tc.wantMtime {
				t.Fatalf("parseStatLine(%q) = (%q, %+v), want (%q, size %d, mtime %d)",
					tc.line, name, st, tc.wantName, tc.wantSize, tc.wantMtime)
			}
		})
	}
}
