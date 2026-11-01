package git

import (
	"os"
	"testing"
)

// FileHistory follows a file through a rename, and says what each commit
// did to it.
func TestFileHistoryFollowsRenames(t *testing.T) {
	r, git := repo(t)
	write(t, r.Root, "tracked.txt", "one\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	write(t, r.Root, "tracked.txt", "two\n")
	git("commit", "-q", "-am", "second")
	if err := os.Rename(r.Abs("tracked.txt"), r.Abs("renamed.txt")); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "rename")

	cs, err := r.FileHistory(r.Abs("renamed.txt"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("%d commits: %+v", len(cs), cs)
	}
	want := []struct {
		subject string
		status  byte
		path    string
		orig    string
	}{
		{"rename", 'R', "renamed.txt", "tracked.txt"},
		{"second", 'M', "tracked.txt", ""},
		{"first", 'A', "tracked.txt", ""},
	}
	for i, w := range want {
		c := cs[i]
		if c.Subject != w.subject || c.Status != w.status || c.Path != r.Abs(w.path) {
			t.Errorf("commit %d is %q %c %q", i, c.Subject, c.Status, r.Rel(c.Path))
		}
		if w.orig != "" && c.Orig != r.Abs(w.orig) {
			t.Errorf("commit %d was renamed from %q", i, r.Rel(c.Orig))
		}
		if !c.Time.IsZero() && c.Author != "t" {
			t.Errorf("commit %d is by %q", i, c.Author)
		}
	}
}
