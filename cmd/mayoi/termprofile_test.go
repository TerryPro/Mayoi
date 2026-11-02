package main

import (
	"os"
	"testing"

	"mayoi/internal/settings"
)

// Windows lists its shells in the order a terminal starts them: PowerShell
// 7 first, and the one the settings choose whenever it is there.
func TestTermProfilesOnWindows(t *testing.T) {
	onOS(t, "windows")
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(name string) (string, error) {
		switch name {
		case "pwsh.exe":
			return `C:\Program Files\PowerShell\7\pwsh.exe`, nil
		case "cmd.exe":
			return `C:\Windows\System32\cmd.exe`, nil
		}
		return "", os.ErrNotExist
	}
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		t.Setenv(key, t.TempDir())
	}
	profiles := windowsTermProfiles()
	if len(profiles) != 2 {
		t.Fatalf("profiles %+v", profiles)
	}
	if p := profiles[0]; p.id != "pwsh" || p.label != "PowerShell" || len(p.command) != 1 {
		t.Errorf("first profile %+v", p)
	}
	if p := profiles[1]; p.id != "cmd" || len(p.command) != 1 {
		t.Errorf("second profile %+v", p)
	}
	a := &app{settings: &settings.Settings{}}
	a.settings.TerminalProfile = "cmd"
	if got := a.defaultTermProfile().id; got != "cmd" {
		t.Errorf("default profile %q, want cmd", got)
	}
}

// The shells of Linux and macOS start with $SHELL's, then those found.
func TestTermProfilesOnUnix(t *testing.T) {
	onOS(t, "linux")
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(name string) (string, error) {
		if name == "bash" {
			return "/bin/bash", nil
		}
		return "", os.ErrNotExist
	}
	t.Setenv("SHELL", "/usr/bin/zsh")
	profiles := unixTermProfiles()
	if len(profiles) != 2 || profiles[0].id != "zsh" || profiles[1].id != "bash" {
		t.Fatalf("profiles %+v", profiles)
	}
}
