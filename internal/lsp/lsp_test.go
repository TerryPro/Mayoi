package lsp

import (
	"encoding/json"
	"testing"
)

func TestHoverContents(t *testing.T) {
	cases := []struct {
		json string
		want string
	}{
		{`{"contents":"plain text"}`, "plain text"},
		{`{"contents":{"kind":"markdown","value":"# Title"}}`, "# Title"},
		{`{"contents":["first","second"]}`, "first\n\nsecond"},
		{`{"contents":[{"language":"go","value":"func f()"},"note"]}`, "func f()\n\nnote"},
	}
	for _, c := range cases {
		var h Hover
		if err := json.Unmarshal([]byte(c.json), &h); err != nil {
			t.Errorf("%s: %v", c.json, err)
			continue
		}
		if got := h.Text(); got != c.want {
			t.Errorf("%s: %q, want %q", c.json, got, c.want)
		}
	}
}
