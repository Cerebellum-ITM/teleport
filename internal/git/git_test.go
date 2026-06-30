package git

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// initRepo builds a throwaway git repo in a temp dir and returns the SHAs of
// three commits: add a.txt, modify a.txt, delete a.txt.
func initRepo(t *testing.T) (addSHA, modSHA, delSHA string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	head := func() string {
		t.Helper()
		out, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatalf("rev-parse: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile("a.txt", []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init")
	write("v1\n")
	run("add", "a.txt")
	run("commit", "-m", "add")
	addSHA = head()

	write("v2\n")
	run("commit", "-am", "modify")
	modSHA = head()

	if err := os.Remove("a.txt"); err != nil {
		t.Fatal(err)
	}
	run("commit", "-am", "delete")
	delSHA = head()
	return addSHA, modSHA, delSHA
}

func TestFileAtCommit(t *testing.T) {
	addSHA, modSHA, _ := initRepo(t)

	if got, err := FileAtCommit(addSHA, "a.txt"); err != nil || string(got) != "v1\n" {
		t.Errorf("FileAtCommit(add) = %q, %v; want \"v1\\n\"", got, err)
	}
	if got, err := FileAtCommit(modSHA, "a.txt"); err != nil || string(got) != "v2\n" {
		t.Errorf("FileAtCommit(mod) = %q, %v; want \"v2\\n\"", got, err)
	}
}

func TestFileBeforeCommit(t *testing.T) {
	_, _, delSHA := initRepo(t)

	got, err := FileBeforeCommit(delSHA, "a.txt")
	if err != nil {
		t.Fatalf("FileBeforeCommit: %v", err)
	}
	if string(got) != "v2\n" {
		t.Errorf("FileBeforeCommit(del) = %q; want pre-delete \"v2\\n\"", got)
	}
}

func TestFileDiffAtCommit(t *testing.T) {
	addSHA, modSHA, delSHA := initRepo(t)

	add, err := FileDiffAtCommit(addSHA, "a.txt")
	if err != nil {
		t.Fatalf("diff add: %v", err)
	}
	if !strings.Contains(string(add), "+v1") || !strings.Contains(string(add), "new file") {
		t.Errorf("add diff missing markers:\n%s", add)
	}

	mod, err := FileDiffAtCommit(modSHA, "a.txt")
	if err != nil {
		t.Fatalf("diff mod: %v", err)
	}
	if !strings.Contains(string(mod), "-v1") || !strings.Contains(string(mod), "+v2") {
		t.Errorf("mod diff missing -v1/+v2:\n%s", mod)
	}

	del, err := FileDiffAtCommit(delSHA, "a.txt")
	if err != nil {
		t.Fatalf("diff del: %v", err)
	}
	if !strings.Contains(string(del), "-v2") || !strings.Contains(string(del), "deleted file") {
		t.Errorf("del diff missing deletion markers:\n%s", del)
	}
}
