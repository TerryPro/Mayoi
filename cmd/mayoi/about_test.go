package main

import "testing"

// The About dialog says which version this build runs.
func TestAboutShowsTheVersion(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.run("help.about")
	hs.settle()
	if !a.about.open {
		t.Fatal("no About dialog")
	}
	if !hs.tt.HasText("Version " + appVersion()) {
		t.Errorf("no version in the About dialog: %q", hs.tt.Texts())
	}
}
