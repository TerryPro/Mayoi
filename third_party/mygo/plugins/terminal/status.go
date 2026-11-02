package terminal

import (
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
)

// Programs say what they are doing with the program status protocol (OSC
// 7501): idle, working, done, blocked on the user or failed, and why. The
// terminal keeps one record per id, as the protocol asks; how to show them
// is the app's. See https://www.superlogical.com/rex/docs/build/program-status.

// StatusState is what a program says it is doing.
type StatusState int

const (
	// StatusIdle is a program at rest, waiting for the user's next
	// instruction, as an agent at its own prompt.
	StatusIdle StatusState = iota
	// StatusWorking is a program running on its own.
	StatusWorking
	// StatusDone is work finished, whose result the user has not seen.
	StatusDone
	// StatusBlocked is a program that can't go on until the user does
	// something, which Kind says.
	StatusBlocked
	// StatusError is work that failed and stopped.
	StatusError
)

// StatusKind is what a blocked program needs from the user.
type StatusKind int

const (
	KindNone StatusKind = iota
	// KindPermission is approval to do something.
	KindPermission
	// KindQuestion is an answer the user has to type.
	KindQuestion
	// KindAuth is a login, password, token or other credential.
	KindAuth
)

// StatusRecord is what a program last said about one thing it does.
type StatusRecord struct {
	// ID is the record's id, "" for the root record, the program itself;
	// a "/" makes a record the child of another.
	ID    string
	State StatusState
	// Kind is set for StatusBlocked alone.
	Kind StatusKind
	// Progress is 0 to 100, or -1 for none, as for work of unknown length.
	Progress int
	// App names the program for machines, as "cargo" or "claude-code",
	// its own or its nearest ancestor's.
	App string
	// Title and Message are for people: a label among several records, and
	// a line of what it does. They are the program's, without invisible
	// formatting characters; show them as plain text.
	Title, Message string

	app    string // its own App
	serial uint64 // when it was last written, for eviction and ties
}

// ProgramStatus is what the programs of a terminal say they are doing.
type ProgramStatus struct {
	// Records are the records in the order of their ids, the root first.
	Records []StatusRecord
}

// Top returns the record most in need of the user: blocked, then failed,
// done, working and idle, the one written last among those alike.
func (s ProgramStatus) Top() (StatusRecord, bool) {
	var top StatusRecord
	ok := false
	for _, r := range s.Records {
		if !ok || urgency(r.State) > urgency(top.State) || urgency(r.State) == urgency(top.State) && r.serial > top.serial {
			top, ok = r, true
		}
	}
	return top, ok
}

func urgency(s StatusState) int {
	switch s {
	case StatusBlocked:
		return 5
	case StatusError:
		return 4
	case StatusDone:
		return 3
	case StatusWorking:
		return 2
	}
	return 1
}

// maxRecords is how many records a terminal keeps; a new one past it
// drops the one written longest ago.
const maxRecords = 256

// statuses are a terminal's records, which t.mu guards.
type statuses struct {
	m      map[string]*StatusRecord
	serial uint64
}

// apply applies a report, and reports whether the records changed.
func (s *statuses) apply(r vt.StatusReport) bool {
	if r.State == vt.StatusClear {
		return s.remove(func(rec *StatusRecord) bool {
			return r.ID == "" || rec.ID == r.ID || strings.HasPrefix(rec.ID, r.ID+"/")
		})
	}
	if r.State < vt.StatusIdle || r.State > vt.StatusError {
		return false
	}
	if s.m == nil {
		s.m = map[string]*StatusRecord{}
	}
	if _, ok := s.m[r.ID]; !ok && len(s.m) >= maxRecords {
		var oldest *StatusRecord
		for _, rec := range s.m {
			if oldest == nil || rec.serial < oldest.serial {
				oldest = rec
			}
		}
		delete(s.m, oldest.ID)
	}
	s.serial++
	rec := &StatusRecord{
		ID: r.ID, State: StatusState(r.State), Progress: r.Progress,
		app: r.App, Title: plain(r.Title), Message: plain(r.Message), serial: s.serial,
	}
	if rec.State == StatusBlocked {
		rec.Kind = StatusKind(r.Kind)
	}
	if rec.State != StatusWorking && rec.State != StatusBlocked {
		rec.Progress = -1
	}
	s.m[r.ID] = rec
	return true
}

// remove removes the records that match, and reports whether any did.
func (s *statuses) remove(match func(*StatusRecord) bool) bool {
	n := len(s.m)
	for id, rec := range s.m {
		if match(rec) {
			delete(s.m, id)
		}
	}
	return len(s.m) != n
}

// ended removes what a program that ended no longer does: the records
// working, blocked or idle. Done and failed work stays until the user
// has seen it.
func (s *statuses) ended() bool {
	return s.remove(func(r *StatusRecord) bool { return r.State != StatusDone && r.State != StatusError })
}

// seen turns done and failed records idle, keeping what they say, and
// reports whether any was.
func (s *statuses) seen() bool {
	changed := false
	for _, r := range s.m {
		if r.State == StatusDone || r.State == StatusError {
			r.State, changed = StatusIdle, true
		}
	}
	return changed
}

// snapshot returns the records, each with the app of its nearest
// ancestor that has one when it has none.
func (s *statuses) snapshot() ProgramStatus {
	var st ProgramStatus
	for _, r := range s.m {
		rec := *r
		rec.App = rec.app
		for id := rec.ID; rec.App == "" && id != ""; {
			i := strings.LastIndexByte(id, '/')
			if i < 0 {
				id = ""
			} else {
				id = id[:i]
			}
			if p := s.m[id]; p != nil {
				rec.App = p.app
			}
		}
		st.Records = append(st.Records, rec)
	}
	slices.SortFunc(st.Records, func(a, b StatusRecord) int { return strings.Compare(a.ID, b.ID) })
	return st
}

// plain drops the invisible formatting characters of a program's text,
// as text direction overrides, which could make it read as another's.
func plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

// ProgramStatus returns what the programs of the terminal say they are
// doing (OSC 7501).
func (t *Terminal) ProgramStatus() ProgramStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status.snapshot()
}

// SeeProgramStatus tells that the user has seen the work that finished or
// failed, as when they type into the terminal: its records turn idle,
// keeping what they say. OnProgramStatus runs if any did.
func (t *Terminal) SeeProgramStatus() {
	t.mu.Lock()
	changed := t.status.seen()
	st := t.status.snapshot()
	t.mu.Unlock()
	if changed && t.opts.OnProgramStatus != nil {
		t.opts.OnProgramStatus(st)
	}
}
