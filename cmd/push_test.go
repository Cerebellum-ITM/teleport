package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestResolvePushDest(t *testing.T) {
	const base = "/srv/app"

	tests := []struct {
		name    string
		local   string
		to      string
		single  bool
		want    string
		wantErr string
	}{
		{name: "mirror local path", local: "web/dist", want: "/srv/app/web/dist"},
		{name: "mirror file at root", local: "app.env", want: "/srv/app/app.env"},
		{name: "single --to is the exact destination", local: "web/dist", to: "web/dist.new", single: true, want: "/srv/app/web/dist.new"},
		{name: "single --to dot is the profile path", local: "web/dist", to: ".", single: true, want: "/srv/app"},
		{name: "single --to trailing slash cleaned", local: "web/dist", to: "sub/", single: true, want: "/srv/app/sub"},
		{name: "multi --to joins the basename", local: "web/dist", to: "releases", single: false, want: "/srv/app/releases/dist"},
		{name: "multi --to joins nested basename", local: "a/b/c.txt", to: "releases/x", single: false, want: "/srv/app/releases/x/c.txt"},
		{name: "reject parent escape", local: "web/dist", to: "../etc", single: true, wantErr: "escapes the profile path"},
		{name: "reject nested escape", local: "web/dist", to: "a/../../b", single: true, wantErr: "escapes the profile path"},
		{name: "reject absolute", local: "web/dist", to: "/etc", single: true, wantErr: "must be relative"},
		{name: "reject absolute nested", local: "web/dist", to: "/srv/app/other", single: true, wantErr: "must be relative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePushDest(base, tt.local, tt.to, tt.single)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got dest %q", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("dest = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePushDestOutsideCwd(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := resolvePushDest("/srv/app", "../elsewhere/file.txt", "", true); err == nil {
		t.Fatal("expected an error for a path outside the current directory")
	} else if !strings.Contains(err.Error(), "--to") {
		t.Fatalf("error %q should name --to as the fix", err)
	}
}

func TestCollectPushItems(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	mustWrite(t, "dist/index.html", "<html>", 0o644)
	mustWrite(t, "dist/assets/app.js", "console.log(1)", 0o644)
	mustWrite(t, "dist/bin/run.sh", "#!/bin/sh\n", 0o755)
	mustWrite(t, "dist/.gitignore-shaped", "ignored by git, pushed anyway", 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "dist/empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "dist/index.html"), filepath.Join(dir, "dist/link.html")); err != nil {
		t.Fatal(err)
	}

	items, dirs, err := collectPushItems([]string{"dist"}, "/srv/app", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotItems := make(map[string]pushItem, len(items))
	for _, it := range items {
		gotItems[it.Remote] = it
	}
	wantRemotes := []string{
		"/srv/app/dist/index.html",
		"/srv/app/dist/assets/app.js",
		"/srv/app/dist/bin/run.sh",
		"/srv/app/dist/.gitignore-shaped",
		"/srv/app/dist/link.html",
	}
	if len(items) != len(wantRemotes) {
		t.Fatalf("collected %d items, want %d: %v", len(items), len(wantRemotes), remotesOf(items))
	}
	for _, want := range wantRemotes {
		if _, ok := gotItems[want]; !ok {
			t.Errorf("missing %s in %v", want, remotesOf(items))
		}
	}

	if m := gotItems["/srv/app/dist/bin/run.sh"].Mode; m != 0o755 {
		t.Errorf("exec bit not preserved: mode = %o, want 755", m)
	}
	if m := gotItems["/srv/app/dist/index.html"].Mode; m != 0o644 {
		t.Errorf("plain file mode = %o, want 644", m)
	}
	// A symlink to a file uploads the target's content, so its size matches.
	if got, want := gotItems["/srv/app/dist/link.html"].Size, gotItems["/srv/app/dist/index.html"].Size; got != want {
		t.Errorf("symlink size = %d, want %d (followed)", got, want)
	}

	sort.Strings(dirs)
	wantDirs := []string{
		"/srv/app/dist",
		"/srv/app/dist/assets",
		"/srv/app/dist/bin",
		"/srv/app/dist/empty",
	}
	if len(dirs) != len(wantDirs) {
		t.Fatalf("dirs = %v, want %v", dirs, wantDirs)
	}
	for i := range wantDirs {
		if dirs[i] != wantDirs[i] {
			t.Fatalf("dirs = %v, want %v", dirs, wantDirs)
		}
	}
}

func TestCollectPushItemsDirsParentsFirst(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	mustWrite(t, "a/b/c/deep.txt", "x", 0o644)

	_, dirs, err := collectPushItems([]string{"a"}, "/srv/app", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 1; i < len(dirs); i++ {
		if len(dirs[i-1]) > len(dirs[i]) {
			t.Fatalf("dirs not ordered parents-first: %v", dirs)
		}
	}
}

func TestCollectPushItemsErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	mustWrite(t, "ok.txt", "fine", 0o644)
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "loop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "loop"), filepath.Join(dir, "loop/self")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		locals []string
		want   string
	}{
		{name: "missing path", locals: []string{"nope.txt"}, want: "nope.txt"},
		{name: "missing path is not a no-op", locals: []string{"ok.txt", "nope.txt"}, want: "nope.txt"},
		{name: "broken symlink", locals: []string{"broken"}, want: "broken"},
		{name: "broken symlink inside a tree", locals: []string{"."}, want: "broken"},
		{name: "symlink cycle", locals: []string{"loop"}, want: "cycle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, dirs, err := collectPushItems(tt.locals, "/srv/app", "")
			if err == nil {
				t.Fatalf("expected an error naming %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
			if items != nil || dirs != nil {
				t.Fatalf("collection must yield nothing on error, got %d items / %d dirs", len(items), len(dirs))
			}
		})
	}
}

func TestPushMode(t *testing.T) {
	tests := []struct {
		in   os.FileMode
		want os.FileMode
	}{
		{0o644, 0o644},
		{0o600, 0o644},
		{0o755, 0o755},
		{0o700, 0o755},
		{0o666, 0o644},
	}
	for _, tt := range tests {
		if got := pushMode(tt.in); got != tt.want {
			t.Errorf("pushMode(%o) = %o, want %o", tt.in, got, tt.want)
		}
	}
}

func remotesOf(items []pushItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Remote)
	}
	sort.Strings(out)
	return out
}

func mustWrite(t *testing.T, rel, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rel, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(rel, mode); err != nil {
		t.Fatal(err)
	}
}
