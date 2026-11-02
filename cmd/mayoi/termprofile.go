package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// termProfile is a shell a new terminal can start, and the command that
// starts it.
type termProfile struct {
	id, label string
	command   []string
}

// termProfiles lists the shells this machine has, the one a new terminal
// starts first where the settings choose none: PowerShell 7, Windows
// PowerShell, the command prompt, Git Bash and WSL on Windows; the shells
// Linux and macOS have, $SHELL's first.
func (a *app) termProfiles() []termProfile {
	if hostOS == "windows" {
		return windowsTermProfiles()
	}
	return unixTermProfiles()
}

// defaultTermProfile is the shell a new terminal starts: the one the
// settings choose, when this machine has it, else the first it has. A
// terminal with no profile starts the shell the terminal itself picks.
func (a *app) defaultTermProfile() termProfile {
	profiles := a.termProfiles()
	chosen := a.prefs().TerminalProfile
	for _, p := range profiles {
		if p.id == chosen {
			return p
		}
	}
	if len(profiles) > 0 {
		return profiles[0]
	}
	return termProfile{label: shellName()}
}

// windowsTermProfiles lists the shells Windows has, in the order a new
// terminal starts them.
func windowsTermProfiles() []termProfile {
	var out []termProfile
	add := func(id, label string, command ...string) {
		out = append(out, termProfile{id: id, label: label, command: command})
	}
	if p, err := lookPath("pwsh.exe"); err == nil && p != "" {
		add("pwsh", "PowerShell", p)
	}
	if p, err := lookPath("powershell.exe"); err == nil && p != "" {
		add("powershell", "Windows PowerShell", p)
	}
	if p, err := lookPath("cmd.exe"); err == nil && p != "" {
		add("cmd", "Command Prompt", p)
	} else if p := os.Getenv("COMSPEC"); p != "" {
		add("cmd", "Command Prompt", p)
	}
	if p := gitBash(); p != "" {
		add("gitbash", "Git Bash", p, "--login", "-i")
	}
	if p, err := lookPath("wsl.exe"); err == nil && p != "" {
		add("wsl", "WSL", p)
	}
	return out
}

// gitBash finds Git's bash, on the path or where Git for Windows installs it.
func gitBash() string {
	if p, err := lookPath("bash.exe"); err == nil && p != "" {
		return p
	}
	for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), filepath.Join(os.Getenv("LocalAppData"), "Programs")} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, "Git", "bin", "bash.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// unixTermProfiles lists the shells Linux and macOS have: $SHELL's first,
// then those their folders hold.
func unixTermProfiles() []termProfile {
	var out []termProfile
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		name := filepath.Base(path)
		out = append(out, termProfile{id: name, label: name, command: []string{path}})
	}
	add(os.Getenv("SHELL"))
	for _, name := range []string{"bash", "zsh", "fish", "sh"} {
		if p, err := lookPath(name); err == nil {
			add(p)
		}
	}
	return out
}

// termNewButton is the panel's button that starts a terminal: a click
// starts one with the default profile, and its menu the others.
func (a *app) termNewButton(c *ui.Context) {
	b := a.iconButton(c, iconPlus, a.withKey(locales.CmdViewNewTerminal(), "view.newTerminal"), false)
	if b.Clicked() {
		a.newTerminal()
	}
	b.ContextMenu(a.termProfileMenu)
}

// termProfileMenu lists the shells a new terminal can start, and the one
// to start by default.
func (a *app) termProfileMenu(m *ui.Menu) {
	profiles := a.termProfiles()
	for _, p := range profiles {
		p := p
		if m.Item(locales.TermNewWith(p.label)).Chosen() {
			a.post(func() { a.openTerminalWith(a.workDir(), p) })
		}
	}
	if len(profiles) > 0 {
		m.Separator()
	}
	m.Submenu(locales.TermSetDefault(), func(m *ui.Menu) {
		chosen := a.defaultTermProfile().id
		for _, p := range profiles {
			p := p
			if m.Item(p.label).Checked(chosen == p.id).Chosen() {
				a.post(func() {
					a.settings.TerminalProfile = p.id
					a.settings.Changed()
				})
			}
		}
	})
}

// openTerminalWith starts a terminal in dir with a profile, where
// terminalLocation says: in the panel, or in a tab of the file column.
func (a *app) openTerminalWith(dir string, p termProfile) {
	if a.settings.TerminalLocation == "editor" {
		a.newTerminalTabWith(a.fileColumn(), dir, p, true)
		return
	}
	a.newTerminalInProfile(dir, p)
}

// newTerminalTabWith starts a terminal in dir with a profile in a new tab
// of a column, after the one shown, and reports whether it did.
func (a *app) newTerminalTabWith(col *column, dir string, p termProfile, focus bool) bool {
	return a.terminalTab(col, col.active+1, a.startTerminalWith(dir, p), focus)
}
