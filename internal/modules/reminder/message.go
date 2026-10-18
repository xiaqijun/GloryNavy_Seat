package reminder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"glorynavy.local/seat/internal/platform/remindermessage"
)

type DetailSource func(context.Context, string, time.Time) ([]remindermessage.Item, error)

const mailBatchSize = 5

func detailKey(kind, key string) string {
	if kind == "approval_pending" {
		return key
	}
	return fmt.Sprintf("%s:%x", kind, sha256.Sum256([]byte(key)))
}

func (s *Service) message(ctx context.Context, account string, event Event, since time.Time, due []string) (string, error) {
	source := s.Details[event.ID]
	if source == nil {
		return "", ErrEvents
	}
	floor := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if since.Before(floor) {
		since = floor
	}
	items, err := source(ctx, account, since)
	if err != nil {
		return "", err
	}
	byKey := map[string]remindermessage.Item{}
	for _, item := range items {
		if item.Key == "" {
			return "", ErrEvents
		}
		key := detailKey(event.ID, item.Key)
		if _, exists := byKey[key]; exists {
			return "", ErrEvents
		}
		byKey[key] = item
	}
	selected := make([]remindermessage.Item, 0, len(due))
	people := []string{}
	characters := []int64{}
	for _, key := range due {
		item, ok := byKey[key]
		if !ok || item.Title == "" {
			return "", ErrEvents
		}
		selected = append(selected, item)
		for _, f := range item.Fields {
			if f.Account != "" {
				people = append(people, f.Account)
			}
			if f.CharacterID > 0 {
				characters = append(characters, f.CharacterID)
			}
		}
	}
	names := map[string]string{}
	if len(people) > 0 && s.Names != nil {
		names, err = s.Names(ctx, people)
		if err != nil {
			return "", err
		}
	}
	characterNames := map[int64]string{}
	if len(characters) > 0 && s.CharacterNames != nil {
		characterNames = s.CharacterNames(ctx, characters)
	}
	for i := range selected {
		// Do not mutate a source's shared slice.
		selected[i].Fields = append([]remindermessage.Field(nil), selected[i].Fields...)
		for j := range selected[i].Fields {
			f := &selected[i].Fields[j]
			if f.Account != "" {
				f.Value = names[f.Account]
				if f.Value == "" {
					f.Value = "角色名暂不可用"
				}
			}
			if f.CharacterID > 0 {
				f.Value = characterNames[f.CharacterID]
				if f.Value == "" {
					f.Value = "角色 #" + strconv.FormatInt(f.CharacterID, 10)
				}
			}
		}
	}
	return renderMessage(event.Name, s.Origin+event.Path, selected)
}

func mailText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, s)
	var out strings.Builder
	used := 0
	for _, r := range strings.TrimSpace(s) {
		encoded := html.EscapeString(string(r))
		n := utf8.RuneCountInString(encoded)
		if used+n > limit-1 {
			out.WriteString("…")
			break
		}
		out.WriteString(encoded)
		used += n
	}
	return out.String()
}

func renderMessage(title, link string, items []remindermessage.Item) (string, error) {
	if len(items) == 0 || len(items) > mailBatchSize {
		return "", ErrEvents
	}
	var body strings.Builder
	fmt.Fprintf(&body, "%s：%d 项。<br>", mailText(title, 40), len(items))
	for i, item := range items {
		fmt.Fprintf(&body, "<br>%d. %s<br>", i+1, mailText(item.Title, 100))
		if len(item.Fields) > 12 {
			return "", ErrEvents
		}
		for _, f := range item.Fields {
			if f.Value == "" {
				continue
			}
			fmt.Fprintf(&body, "%s：%s<br>", mailText(f.Label, 24), mailText(f.Value, 100))
		}
		if item.Action != "" {
			fmt.Fprintf(&body, "%s<br>", mailText(item.Action, 160))
		}
	}
	fmt.Fprintf(&body, "<br><a href=\"%s\">查看详情</a>", html.EscapeString(link))
	// Fail before sending rather than cutting a tag, amount or an unlisted receipt.
	if utf8.RuneCountInString(body.String()) > 9500 {
		return "", ErrEvents
	}
	return body.String(), nil
}
