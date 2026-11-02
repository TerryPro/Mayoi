package main

import (
	"path/filepath"
	"strings"
)

// This file is the fork's share of the terminal panel's chips: the short
// name they show and the tip under the pointer. It is kept out of panel.go,
// which upstream also changes.

// termLabel is the name a terminal shows in its chip, its list row and its
// tab: the name it was given, or the name of its shell.
func termLabel(s *session) string {
	if name := strings.TrimSpace(s.name); name != "" {
		return name
	}
	return termTabLabel(s)
}

// termTabLabel is the short name a terminal's chip shows: the shell's own
// name, not the path a shell such as PowerShell puts in its window title,
// and the name people know it by.
func termTabLabel(s *session) string {
	title := strings.TrimSpace(s.title)
	if title == "" {
		title = shellName()
	}
	if path, ok := pathTitle(title); ok {
		base := filepath.Base(path)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	switch strings.ToLower(title) {
	case "pwsh":
		return "PowerShell"
	case "powershell":
		return "Windows PowerShell"
	case "cmd":
		return "Command Prompt"
	}
	return title
}

// pathTitle reports whether a window title is a path, as PowerShell's is
// that of its shell, and returns it.
func pathTitle(title string) (string, bool) {
	if len(title) > 2 && title[1] == ':' && (title[2] == '\\' || title[2] == '/') {
		return title, true
	}
	if strings.HasPrefix(title, "/") {
		return title, true
	}
	return "", false
}

// termTabTip is what a terminal's chip says under the pointer: the title the
// shell gave, and the directory it started in.
func termTabTip(s *session) string {
	title := strings.TrimSpace(s.title)
	if s.dir == "" || strings.Contains(title, s.dir) {
		return title
	}
	if title == "" {
		return s.dir
	}
	return title + " — " + s.dir
}
