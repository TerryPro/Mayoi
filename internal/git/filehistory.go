package git

import (
	"strconv"
	"strings"
	"time"
)

// FileCommit is a commit in a file's history: the commit, the path the
// file had then, the path it was renamed from, and what the commit did to
// it.
type FileCommit struct {
	Commit
	Status byte
	Path   string
	Orig   string
}

// fileHistoryFormat is how FileHistory has git print a commit: its fields,
// separated by unit separators, after a record separator.
const fileHistoryFormat = "--format=%x1e%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%s"

// FileHistory returns the last n commits that changed a file, newest first,
// those of the names it had before included, and what each did to it.
func (r *Repo) FileHistory(path string, n int) ([]FileCommit, error) {
	if _, err := r.run(nil, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return nil, nil
	}
	if n <= 0 {
		n = 50
	}
	rel := r.Rel(path)
	out, err := r.run(nil, "log", "--follow", "--name-status", "-n", strconv.Itoa(n), fileHistoryFormat, "--", rel)
	if err != nil {
		return nil, err
	}
	var list []FileCommit
	for _, rec := range strings.Split(string(out), "\x1e") {
		rec = strings.TrimLeft(rec, "\r\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		lines := strings.Split(rec, "\n")
		f := strings.SplitN(strings.TrimSpace(lines[0]), "\x1f", 6)
		if len(f) < 6 {
			continue
		}
		c := Commit{Hash: f[0], Parents: strings.Fields(f[1]), Author: f[2], Email: f[3], Subject: f[5]}
		if s, err := strconv.ParseInt(f[4], 10, 64); err == nil {
			c.Time = time.Unix(s, 0)
		}
		fc := FileCommit{Commit: c, Status: 'M', Path: r.Abs(rel)}
		for _, line := range lines[1:] {
			line = strings.TrimSuffix(line, "\r")
			if line == "" || line[0] == '\t' {
				continue
			}
			parts := strings.Split(line, "\t")
			st := parts[0]
			if st == "" || len(parts) < 2 {
				continue
			}
			switch st[0] {
			case 'R':
				if len(parts) >= 3 {
					fc.Status, fc.Orig, fc.Path = 'R', r.Abs(parts[1]), r.Abs(parts[2])
				}
			case 'C':
				if len(parts) >= 3 {
					fc.Status, fc.Orig, fc.Path = 'A', r.Abs(parts[1]), r.Abs(parts[2])
				}
			case 'A', 'M', 'D', 'T':
				fc.Status, fc.Path = st[0], r.Abs(parts[1])
			}
		}
		list = append(list, fc)
	}
	return list, nil
}
