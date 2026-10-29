package main

import (
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// TestFindCountFits checks that find's count of matches has room for
// every text it shows, in every language, as in Chinese it had not for
// one match of five.
func TestFindCountFits(t *testing.T) {
	locale := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return locale }) })
	for _, loc := range []string{"en", "ja", "zh-Hans"} {
		i18n.SetLocaleFunc(func() string { return loc })
		m := locales.In(loc)
		a := &app{}
		ui.NewTester(func(c *ui.Context) {
			w := a.findCountWidth(c)
			for _, s := range []string{
				m.FindCount(1, 5), m.FindCount(19999, 19999), m.FindCountMore(maxMatches, maxMatches),
				m.FindInvalid(), m.FindNoResults(),
			} {
				if tw, _ := c.MeasureText(0, ui.Span{Text: s, Size: findCountSize}); tw > w {
					t.Errorf("%s: %q is %.1f wide, the count %.1f", loc, s, tw, w)
				}
			}
		}, 400, 100)
	}
}
