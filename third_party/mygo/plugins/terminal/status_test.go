package terminal

import (
	"encoding/base64"
	"testing"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
)

func osc7501(pairs string) string { return "\x1b]7501;" + pairs + "\x1b\\" }

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestProgramStatus(t *testing.T) {
	loadLib(t)
	var got []ProgramStatus
	pipe := newPipe()
	term, err := New(Options{Conn: pipe, OnProgramStatus: func(s ProgramStatus) { got = append(got, s) }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()

	// The terminal says it understands programs that ask.
	term.Feed([]byte("\x1b]7501;?\x1b\\"))
	if reply := pipe.take(term); reply != "\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth\x1b\\" {
		t.Fatalf("reply to the query %q", reply)
	}

	// The reports of a piece of output come at once; a child takes the
	// app of its parent.
	term.Feed([]byte(osc7501("state=working:app=deploy:msg="+b64("Deploying")) +
		osc7501("state=working:id=us:progress=40") +
		osc7501("state=blocked:id=eu:kind=permission:progress=10:msg="+b64("Approve‮?"))))
	if len(got) != 1 {
		t.Fatalf("%d calls, want 1", len(got))
	}
	st := got[0]
	if len(st.Records) != 3 {
		t.Fatalf("records %+v", st.Records)
	}
	top, _ := st.Top()
	if top.ID != "eu" || top.State != StatusBlocked || top.Kind != KindPermission || top.Progress != 10 || top.App != "deploy" || top.Message != "Approve?" {
		t.Fatalf("top %+v", top)
	}
	if r := st.Records[0]; r.ID != "" || r.Progress != -1 || r.Message != "Deploying" {
		t.Fatalf("root %+v", r)
	}

	// A report replaces its record whole; clear takes the children too.
	term.Feed([]byte(osc7501("state=done:id=eu")))
	if top, _ := term.ProgramStatus().Top(); top.ID != "eu" || top.State != StatusDone || top.Message != "" || top.Progress != -1 {
		t.Fatalf("after done %+v", top)
	}
	term.Feed([]byte(osc7501("state=working:id=us/a") + osc7501("state=clear:id=us")))
	if st := term.ProgramStatus(); len(st.Records) != 2 {
		t.Fatalf("after clear %+v", st.Records)
	}

	// A prompt drops the work going on, and keeps what finished; seeing
	// it turns it idle.
	term.Feed([]byte("\x1b]133;A\x1b\\"))
	st = term.ProgramStatus()
	if len(st.Records) != 1 || st.Records[0].State != StatusDone {
		t.Fatalf("after prompt %+v", st.Records)
	}
	n := len(got)
	term.SeeProgramStatus()
	if len(got) != n+1 || got[n].Records[0].State != StatusIdle {
		t.Fatalf("after seen %+v", got[n:])
	}

	// Unknown states and malformed reports change nothing.
	n = len(got)
	term.Feed([]byte(osc7501("state=sleeping") + osc7501("state=working:msg=A")))
	if len(got) != n {
		t.Fatalf("bad reports changed the records: %+v", got[n:])
	}
	term.Feed([]byte(osc7501("state=clear")))
	if st := term.ProgramStatus(); len(st.Records) != 0 {
		t.Fatalf("after clearing all %+v", st.Records)
	}
}

func TestProgramStatusEvicts(t *testing.T) {
	var s statuses
	for i := range maxRecords + 1 {
		s.apply(vtReport("r" + itoa(i)))
	}
	if len(s.m) != maxRecords || s.m["r000"] != nil || s.m["r256"] == nil {
		t.Fatalf("%d records, oldest kept %v", len(s.m), s.m["r000"] != nil)
	}
}

// TestProgramStatusQuiet: a terminal that takes no reports leaves the
// query unanswered, so programs see it doesn't understand them.
func TestProgramStatusQuiet(t *testing.T) {
	loadLib(t)
	pipe := newPipe()
	term, err := New(Options{Conn: pipe})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("\x1b]7501;?\x1b\\"))
	if reply := pipe.take(term); reply != "" {
		t.Fatalf("reply %q", reply)
	}
}

func vtReport(id string) vt.StatusReport {
	return vt.StatusReport{State: vt.StatusWorking, ID: id, Progress: -1}
}
