package wallet

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWalletSearchNormalizesWhitespaceAndBoundsUnicode(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		valid       bool
	}{
		{"\u3000三钛合金\t Buyer  ", "三钛合金 Buyer", true}, {strings.Repeat("中", 200), strings.Repeat("中", 200), true},
		{strings.Repeat("中", 201), "", false}, {strings.Repeat("a ", 11), "", false}, {"a\x00b", "", false},
	} {
		r := httptest.NewRequest("GET", "/?owner_kind=character&owner_id=123&part=journal&search="+url.QueryEscape(tc.input), nil)
		f, err := parseFilter(r)
		if (err == nil) != tc.valid || (tc.valid && f.Search != tc.want) {
			t.Fatalf("input %q => %q %v", tc.input, f.Search, err)
		}
	}
}
