package install_test

import (
	"testing"

	"github.com/rifanid/agentmap-go/internal/install"
)

func TestStripJSONComments(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`{"a":1}`, `{"a":1}`},
		// The space before // is not part of the comment and is preserved.
		{"{\"a\":1 // line comment\n}", "{\"a\":1 \n}"},
		{`{"a": /* block */ 1}`, `{"a":  1}`},
		// URL inside a string must NOT be stripped
		{`{"url": "https://example.com"}`, `{"url": "https://example.com"}`},
	}
	for _, c := range cases {
		got := install.StripJSONComments(c.in)
		if got != c.want {
			t.Errorf("StripJSONComments(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
