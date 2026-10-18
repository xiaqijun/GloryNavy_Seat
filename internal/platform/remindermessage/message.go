// Package remindermessage defines ephemeral, authorized mail projections.
// It owns no business state, SQL, recipients or delivery receipts.
package remindermessage

import (
	"strconv"
	"strings"
	"time"
)

type Field struct {
	Label, Value string
	// Account is resolved through the host only after the source authorized it.
	// It is never rendered as a raw account UUID.
	Account     string
	CharacterID int64
}

type Item struct {
	Key, Title string
	Fields     []Field
	Action     string
}

func Text(label, value string) Field         { return Field{Label: label, Value: value} }
func Person(label, account string) Field     { return Field{Label: label, Account: account} }
func Character(label string, id int64) Field { return Field{Label: label, CharacterID: id} }
func Date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// Money preserves every minor unit, including negative corrections and MinInt64.
func Money(minor int64, unit string) string {
	s := strconv.FormatInt(minor, 10)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	for len(s) < 3 {
		s = "0" + s
	}
	whole, fraction := s[:len(s)-2], s[len(s)-2:]
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	return sign + whole + "." + fraction + " " + unit
}

func Signed(minor int64, unit string) string {
	s := Money(minor, unit)
	if minor > 0 {
		s = "+" + s
	}
	return s
}

// Label maps only fixed business enums; user content is never translated here.
func Label(value string, labels map[string]string) string {
	if label, ok := labels[value]; ok {
		return label
	}
	return "待查看"
}
