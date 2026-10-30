package main

import (
	"encoding/json"
	"strings"

	"mayoi/internal/bg"
	"mayoi/internal/remote"
	"mayoi/locales"
)

// In a remote folder, with terminalBackend "hc", a terminal's shell runs
// in herdr-core on the host, which hc reaches as one of its machines: it
// outlives the connection too. A host hc doesn't know is added once the
// user agrees, which may copy hc to it; its terminals use ssh until then.

// hcAsked are the hosts asked about adding to hc as Mayoi runs: those
// declined keep ssh until it quits. The main thread's.
var hcAsked = map[string]bool{}

// hcMachineName is the name Mayoi gives a host as one of hc's machines:
// its name without the user, and its port.
func hcMachineName(host string) string {
	target, port := remote.SSHTarget(host)
	if _, h, ok := strings.Cut(target, "@"); ok {
		target = h
	}
	if port != "" {
		target += "-" + port
	}
	name := strings.Map(func(r rune) rune {
		if 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			return r
		}
		return '-'
	}, strings.ToLower(target))
	return strings.Trim(name, "-")
}

// hcMachine returns the name of host's machine in hc: the one Mayoi
// added, or one added by hand that ssh reaches the same way. False when
// hc doesn't know the host.
func hcMachine(host string) (string, bool) {
	out, err := hcRun("machine", "ls", "--json")
	if err != nil {
		return "", false
	}
	var l struct {
		Machines []struct{ Name, Target string }
	}
	if json.Unmarshal(out, &l) != nil {
		return "", false
	}
	target, port := remote.SSHTarget(host)
	name := hcMachineName(host)
	for _, m := range l.Machines {
		if m.Name == name || port == "" && m.Target == target {
			return m.Name, true
		}
	}
	return "", false
}

// hcAddMachine adds host to hc as a machine, copying hc there when it
// has none or one too old. A port goes to ssh, which hc then runs.
func hcAddMachine(host string) error {
	target, port := remote.SSHTarget(host)
	args := []string{"machine", "add", "--json", "--install", "--name", hcMachineName(host)}
	if port != "" {
		args = append(args, "--via", "ssh -p "+port+" "+target)
	} else {
		args = append(args, target)
	}
	_, err := hcRun(args...)
	return err
}

// hcRemote returns the machine of hc a terminal of host runs on, or ""
// for ssh: once for a host hc doesn't know, it asks whether to add it.
func (a *app) hcRemote(host string) string {
	if m, ok := hcMachine(host); ok {
		return m
	}
	if !hcAsked[host] && a.confirm == nil {
		hcAsked[host] = true
		a.confirm = &confirm{
			title: locales.HcAddTitle(host), message: locales.HcAddDetail(host), ok: locales.HcAdd(),
			run: func() { a.hcAdd(host) },
		}
	}
	return ""
}

// hcAdd adds host to hc in the background, and says how it went.
func (a *app) hcAdd(host string) {
	bg.Go(func() {
		err := hcAddMachine(host)
		a.post(func() {
			if err != nil {
				a.fail("%s", locales.FailHCAdd(host, err.Error(), hcMachineName(host)))
				return
			}
			a.tell(note{message: locales.HcAdded(host)})
		})
	})
}
