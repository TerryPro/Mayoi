package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"mayoi/internal/remote"
)

func TestHCMachineName(t *testing.T) {
	for host, want := range map[string]string{
		"vps":                "vps",
		"me@Box.example.com": "box-example-com",
		"me@box:2222":        "box-2222",
		"[::1]:22":           "1-22",
		"dev_box":            "dev-box",
	} {
		if got := hcMachineName(host); got != want {
			t.Errorf("%q: %q, want %q", host, got, want)
		}
	}
}

// A host is hc's machine by the name Mayoi gives it, or by its ssh
// target for one added by hand; a host with a port goes through ssh -p.
func TestHCMachine(t *testing.T) {
	f := useFakeHC(t, func(args []string) ([]byte, error) {
		return []byte(`{"machines":[{"name":"rohan","target":"me@rohan.lan"},{"name":"box-2222","target":"ssh -p 2222 me@box"}]}`), nil
	})
	for host, want := range map[string]string{"me@rohan.lan": "rohan", "me@box:2222": "box-2222", "me@box": "", "other": ""} {
		if got, ok := hcMachine(host); got != want || ok != (want != "") {
			t.Errorf("%q: %q %v, want %q", host, got, ok, want)
		}
	}
	f.reply = func([]string) ([]byte, error) { return nil, nil }
	hcAddMachine("me@box:2222")
	hcAddMachine("vps")
	add := f.ran("machine")
	want := [][]string{
		{"machine", "add", "--json", "--install", "--name", "box-2222", "--via", "ssh -p 2222 me@box"},
		{"machine", "add", "--json", "--install", "--name", "vps", "vps"},
	}
	if got := add[len(add)-2:]; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q", got)
	}
}

// A terminal of a host hc doesn't know uses ssh, and the first asks once
// whether to add the host; adding it says so. Once added, a terminal of
// it starts on it, its address the machine's.
func TestHCRemote(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	t.Cleanup(func() { clear(hcAsked) })
	known := false
	f := useFakeHC(t, func(args []string) ([]byte, error) {
		switch {
		case args[0] == "machine" && args[1] == "ls" && known:
			return []byte(`{"machines":[{"name":"vps","target":"vps"}]}`), nil
		case args[0] == "machine" && args[1] == "ls":
			return []byte(`{"machines":[]}`), nil
		case args[0] == "run":
			return []byte(`{"address":"t2"}`), nil
		}
		return []byte(`{}`), nil
	})
	if m := a.hcRemote("vps"); m != "" || a.confirm == nil {
		t.Fatalf("machine %q, asked %v", m, a.confirm != nil)
	}
	add := a.confirm.run
	a.confirm = nil
	if a.hcRemote("vps"); a.confirm != nil {
		t.Error("asked again")
	}
	add()
	hs.settle()
	if got := f.ran("machine"); !slices.ContainsFunc(got, func(c []string) bool { return c[1] == "add" }) {
		t.Errorf("ran %q", got)
	}
	if !slices.ContainsFunc(hs.tt.Texts(), func(s string) bool { return strings.Contains(s, "vps was added") }) {
		t.Errorf("no toast: %q", hs.tt.Texts())
	}
	known = true
	if m := a.hcRemote("vps"); m != "vps" {
		t.Fatalf("machine %q", m)
	}
	s := &session{dir: remote.Path("vps", "/srv/app")}
	a.hcBegin(s, "vps")
	run := f.ran("run")
	if s.hc != "vps/t2" || !s.hcNew || !slices.Contains(run[0], "--on") || !slices.Contains(run[0], "/srv/app") {
		t.Errorf("address %q, ran %q", s.hc, run)
	}
}

// Adding a host that fails says how to log in to it.
func TestHCAddFails(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	useFakeHC(t, func([]string) ([]byte, error) { return nil, errors.New("needs a password") })
	fails := a.fails
	a.hcAdd("me@box:2222")
	hs.settle()
	if a.fails != fails+1 {
		t.Errorf("%d errors", a.fails-fails)
	}
}

// A terminal of another machine that hc can't reach says the connection
// went, and waits: once hc says its shell is gone, it closes.
func TestHCLost(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	old := hcRetry
	hcRetry = 10 * time.Millisecond
	t.Cleanup(func() { hcRetry = old })
	gone := false
	useFakeHC(t, func(args []string) ([]byte, error) {
		if gone {
			return nil, &hcErr{"not_found", "no terminal"}
		}
		return nil, &hcErr{"unreachable", "vps is not connected"}
	})
	s := hcSession(t, a, "vps/t2")
	s.dir = remote.Path("vps", "/srv/app")
	s.attached = time.Now().Add(-time.Minute)
	a.terminalExited(s, s.term, 0)
	// It looks again and again, which settle would wait out: frames
	// rather.
	until := func(done func() bool) {
		for deadline := time.Now().Add(2 * time.Second); !done() && time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			step(hs.tt)
		}
	}
	until(func() bool { return s.lost })
	if !slices.Contains(a.terms, s) || !s.lost || !strings.Contains(s.term.Text(), "vps") {
		t.Fatalf("open %v, lost %v, text %q", slices.Contains(a.terms, s), s.lost, s.term.Text())
	}
	gone = true
	until(func() bool { return !slices.Contains(a.terms, s) })
	if slices.Contains(a.terms, s) {
		t.Error("a terminal whose shell is gone stays")
	}
}

// Starting again in a remote folder, a terminal of the host's machine
// attaches again in the folder its shell is in, as a path of the host.
func TestHCRemoteTerms(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	useFakeHC(t, func(args []string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"terminals":[{"address":"t1","cwd":"/tmp","state":"live"},{"address":"vps/t2","cwd":"/srv/app","state":"live"}]}`), nil
		}
		if args[2] == "vps/t2" {
			return []byte(`{"state":"live","cwd":"/srv/app"}`), nil
		}
		return nil, &hcErr{"not_found", "no terminal"}
	})
	root := a.root
	a.root = remote.Path("vps", "/srv")
	t.Cleanup(func() { a.root = root })
	live := a.hcTerms([]string{"t1", "vps/t2", "vps/t3"})
	if len(live) != 2 || live["t1"] != "/tmp" || live["vps/t2"] != remote.Path("vps", "/srv/app") {
		t.Errorf("live %q", live)
	}
}
