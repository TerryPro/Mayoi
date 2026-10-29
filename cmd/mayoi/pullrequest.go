package main

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"mayoi/internal/bg"
	"mayoi/internal/git"
	"mayoi/internal/remote"
	"mayoi/locales"
)

// How often the pull request of the branch is asked for again: sooner
// while its checks run, and seldom once gh could not tell.
const (
	prEvery        = 60 * time.Second
	prPendingEvery = 30 * time.Second
	prOffEvery     = 10 * time.Minute
)

// refreshPR asks gh in the background for the pull request of the branch
// checked out, as the branch or its commit changes, or as the last answer
// gets old; only while the window shows the project.
func (a *gitContext) refreshPR() {
	g := a.git
	st := g.status
	if !a.live() || st == nil || st.Branch == "" || g.prRunning || !a.app.online() || !a.app.host.shown() {
		return
	}
	key := st.Branch + "\x00" + st.Head + "\x00" + st.Upstream
	if g.prRepo == g.repo && key == g.prOf && time.Since(g.prRead) < a.prEvery() {
		return
	}
	g.prRunning = true
	r, branch := g.repo, st.Branch
	bg.Go(func() {
		defer recoverCrash()
		pr, err := r.PullRequest()
		a.app.post(func() {
			if g.repo != r {
				return
			}
			g.prRunning = false
			if g.status == nil || g.status.Branch != branch {
				// Switched meanwhile: the next frame asks for the new one.
				return
			}
			g.prRepo, g.prBranch, g.prOf, g.prRead = r, branch, key, time.Now()
			switch {
			case err == nil:
				g.pr, g.prOff = pr, false
			case errors.Is(err, git.ErrNoPR):
				g.pr, g.prOff = nil, false
			case errors.Is(err, remote.ErrOffline):
				// The host is away: what was read stays until it is back.
			default:
				g.pr, g.prOff = nil, true
			}
			if g.prWake != nil {
				g.prWake.Stop()
			}
			g.prWake = time.AfterFunc(a.prEvery(), a.app.invalidate)
		})
	})
}

func (a *gitContext) prEvery() time.Duration {
	g := a.git
	switch {
	case g.prOff:
		return prOffEvery
	case g.pr != nil && g.pr.Checks == git.ChecksPending:
		return prPendingEvery
	}
	return prEvery
}

// currentPR is the pull request of the branch checked out, nil while none
// is known.
func (a *gitContext) currentPR() *git.PR {
	g := a.git
	if g.pr == nil || g.prRepo != g.repo || g.status == nil || g.status.Branch != g.prBranch {
		return nil
	}
	return g.pr
}

// prBadge shows the pull request of the branch, its number, how its
// checks went and its review, which a click opens in the browser. In the
// status bar, it is in the bar's color, and short says only the number.
func (a *gitContext) prBadge(c *ui.Context, inStatus, short bool) {
	pr := a.currentPR()
	if pr == nil {
		return
	}
	p := a.app.pal
	fg, size, key, h := p.muted, float32(11.5), "scm-pr", float32(22)
	if inStatus {
		fg, key, h = p.statusFg, "status:pr", statusBarH-6
	}
	icon, col := prChecksIcon(p, pr.Checks)
	if inStatus {
		col = fg
	}
	b := ui.ButtonBase(c.Key(key)).Height(h).Padding(0, 6).Gap(4).Radius(4).AlignItems(ui.Center).
		Shrink(0).Label(prTip(pr)).Tooltip(prTip(pr))
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Transition(hoverFade)
	b.Children(func() {
		text := func(s string) { ui.Text(c, s).FontSize(size).TextColor(fg).SingleLine() }
		text("#" + strconv.Itoa(pr.Number))
		if short {
			return
		}
		if icon != nil {
			text("·")
			ui.Icon(c, icon).FontSize(11).TextColor(col)
		}
		if review := prReviewLabel(pr.Review); review != "" {
			text("·")
			text(review)
		}
	})
	if b.Clicked() && pr.URL != "" {
		a.app.services.OpenURL(pr.URL)
	}
}

// prBadgeLabel is what the badge says, for the status bar to measure.
func prBadgeLabel(pr *git.PR) string {
	s := "#" + strconv.Itoa(pr.Number)
	if pr.Checks != git.ChecksNone {
		s += " · ✓"
	}
	if review := prReviewLabel(pr.Review); review != "" {
		s += " · " + review
	}
	return s
}

func prChecksIcon(p *palette, checks git.Checks) (*ui.SVG, ui.Color) {
	switch checks {
	case git.ChecksPass:
		return iconCheck, p.addedFg
	case git.ChecksFail:
		return iconX, p.dangerFg
	case git.ChecksPending:
		return iconClock, p.modifiedFg
	}
	return nil, p.muted
}

func prReviewLabel(review string) string {
	switch review {
	case "APPROVED":
		return locales.PrApproved()
	case "CHANGES_REQUESTED":
		return locales.PrChangesRequested()
	case "REVIEW_REQUIRED":
		return locales.PrReviewRequired()
	}
	return ""
}

// prTip says the state of the pull request, then its checks and review.
func prTip(pr *git.PR) string {
	state := locales.PrOpen()
	switch pr.State {
	case "MERGED":
		state = locales.PrMerged()
	case "CLOSED":
		state = locales.PrClosed()
	}
	parts := []string{locales.PrTip("#"+strconv.Itoa(pr.Number), state)}
	if pr.Passed > 0 {
		parts = append(parts, locales.PrChecksPassed(pr.Passed))
	}
	if pr.Failed > 0 {
		parts = append(parts, locales.PrChecksFailed(pr.Failed))
	}
	if pr.Pending > 0 {
		parts = append(parts, locales.PrChecksPending(pr.Pending))
	}
	if review := prReviewLabel(pr.Review); review != "" {
		parts = append(parts, review)
	}
	return strings.Join(parts, " · ")
}
