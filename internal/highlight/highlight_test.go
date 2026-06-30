package highlight

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestCodeKnownExtension(t *testing.T) {
	body, lang, lines, err := Code([]byte("package main\n\nfunc main() {}\n"), "main.go", colorprofile.ANSI256)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if lang != "Go" {
		t.Errorf("lang = %q, want Go", lang)
	}
	if lines != 3 {
		t.Errorf("lines = %d, want 3", lines)
	}
	if !strings.Contains(body, "main") {
		t.Errorf("body missing source text: %q", body)
	}
}

func TestCodeUnknownExtensionFallsBack(t *testing.T) {
	body, lang, lines, err := Code([]byte("just some text\nsecond line\n"), "notes.zzz", colorprofile.NoTTY)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if lang == "" {
		t.Errorf("lang is empty, want a fallback name")
	}
	if lines != 2 {
		t.Errorf("lines = %d, want 2", lines)
	}
	if !strings.Contains(body, "just some text") {
		t.Errorf("body missing source text: %q", body)
	}
}

func TestDiffCounts(t *testing.T) {
	raw := "diff --git a/x b/x\n" +
		"--- a/x\n" +
		"+++ b/x\n" +
		"@@ -1,2 +1,3 @@\n" +
		" context\n" +
		"-removed one\n" +
		"+added one\n" +
		"+added two\n"
	// adds=2 / dels=1 proves the +++/--- file headers were not miscounted as
	// add/remove lines (they start with + and -). Use a plaintext filename so
	// the asserted words are not split by syntax-highlight escape codes.
	body, adds, dels := Diff([]byte(raw), "x.txt", 80, colorprofile.NoTTY)
	if adds != 2 {
		t.Errorf("adds = %d, want 2", adds)
	}
	if dels != 1 {
		t.Errorf("dels = %d, want 1", dels)
	}
	if !strings.Contains(body, "added one") || !strings.Contains(body, "removed one") {
		t.Errorf("body missing diff lines:\n%s", body)
	}
}

func TestIsBinary(t *testing.T) {
	if IsBinary([]byte("plain text\n")) {
		t.Error("text classified as binary")
	}
	if !IsBinary([]byte("abc\x00def")) {
		t.Error("NUL-containing blob not classified as binary")
	}
}
