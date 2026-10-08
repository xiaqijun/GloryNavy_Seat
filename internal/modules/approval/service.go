package approval

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid approval query")

type Service struct {
	Sources []reviewqueue.Source
	Names   func(context.Context, []string) (map[string]string, error)
}
type Context struct {
	Allowed      bool                 `json:"allowed"`
	Sources      []string             `json:"sources"`
	Corporations []reviewqueue.Option `json:"corporations"`
	Unavailable  []string             `json:"unavailable"`
	People       []reviewqueue.Option `json:"people"`
}

func (s *Service) Context(ctx context.Context, user string) (Context, error) {
	return s.ContextWithOptions(ctx, user, true)
}

// ContextWithOptions keeps the initial approval shell lightweight. Applicant
// options are only needed when the advanced filter is opened, so callers can
// skip the member/name projection on the first request.
func (s *Service) ContextWithOptions(ctx context.Context, user string, includePeople bool) (Context, error) {
	out := Context{Sources: []string{}, Corporations: []reviewqueue.Option{}, Unavailable: []string{}, People: []reviewqueue.Option{}}
	people := map[string]bool{}
	seen := map[string]bool{}
	type sourceContextResult struct {
		id           string
		access       reviewqueue.Access
		accessError  error
		people       []string
		peopleError  error
		peopleLoaded bool
	}
	results := make([]sourceContextResult, len(s.Sources))
	var wg sync.WaitGroup
	for i, source := range s.Sources {
		wg.Add(1)
		go func(i int, source reviewqueue.Source) {
			defer wg.Done()
			result := sourceContextResult{id: source.ID}
			result.access, result.accessError = source.Access(ctx, user)
			if includePeople && result.accessError == nil && result.access.Allowed && (source.People != nil || source.PeopleAuthorized != nil) {
				if source.PeopleAuthorized != nil {
					result.people, result.peopleError = source.PeopleAuthorized(ctx, user, result.access)
				} else {
					result.people, result.peopleError = source.People(ctx, user)
				}
				result.peopleLoaded = true
			}
			results[i] = result
		}(i, source)
	}
	wg.Wait()
	for _, result := range results {
		if result.accessError != nil {
			slog.Warn("approval source unavailable", "source", result.id, "stage", "access", "error", result.accessError)
			out.Unavailable = append(out.Unavailable, result.id)
			continue
		}
		if !result.access.Allowed {
			continue
		}
		out.Allowed = true
		out.Sources = append(out.Sources, result.id)
		if result.peopleLoaded && result.peopleError != nil {
			slog.Warn("approval source unavailable", "source", result.id, "stage", "people", "error", result.peopleError)
			out.Unavailable = append(out.Unavailable, result.id)
		} else if result.peopleLoaded {
			for _, id := range result.people {
				people[id] = true
			}
		}
		for _, c := range result.access.Corporations {
			if !seen[c.ID] {
				seen[c.ID] = true
				out.Corporations = append(out.Corporations, c)
			}
		}
	}
	if includePeople && s.Names != nil && len(people) > 0 {
		ids := []string{}
		for id := range people {
			ids = append(ids, id)
		}
		names, e := s.Names(ctx, ids)
		if e == nil {
			for _, id := range ids {
				if name := names[id]; name != "" {
					out.People = append(out.People, reviewqueue.Option{ID: id, Name: name})
				}
			}
			slices.SortFunc(out.People, func(a, b reviewqueue.Option) int { return strings.Compare(a.Name, b.Name) })
		}
	}
	return out, nil
}

type Result struct {
	Items       []reviewqueue.Item `json:"items"`
	Counts      map[string]int64   `json:"counts"`
	Next        string             `json:"next_cursor"`
	Unavailable []string           `json:"unavailable"`
}
type cursor struct {
	Position    reviewqueue.Position `json:"position"`
	Fingerprint string               `json:"fingerprint"`
}

func fingerprint(user string, f reviewqueue.Filter) string {
	b, _ := json.Marshal(f)
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(user), b...)))
}
func validate(f reviewqueue.Filter) error {
	if !slices.Contains([]string{"pending", "information", "fulfillment", "exceptions", "history"}, f.View) || !slices.Contains([]string{"", "time_asc", "time_desc", "id_asc", "id_desc"}, f.Sort) || len(f.Search) > 200 || len(f.Kind) > 80 || len(f.Status) > 80 {
		return ErrInvalid
	}
	if f.Mine && f.View != "history" {
		return ErrInvalid
	}
	for _, d := range []string{f.From, f.Until} {
		if d != "" {
			if _, e := time.Parse(time.RFC3339, d); e != nil {
				return ErrInvalid
			}
		}
	}
	if f.From != "" && f.Until != "" {
		a, _ := time.Parse(time.RFC3339, f.From)
		b, _ := time.Parse(time.RFC3339, f.Until)
		if !a.Before(b) {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Service) List(ctx context.Context, user string, f reviewqueue.Filter, token string) (Result, error) {
	out := Result{Items: []reviewqueue.Item{}, Counts: map[string]int64{}, Unavailable: []string{}}
	if e := validate(f); e != nil {
		return out, e
	}
	var c cursor
	if token != "" {
		b, e := base64.RawURLEncoding.DecodeString(token)
		if e != nil || len(b) > 2048 || json.Unmarshal(b, &c) != nil || c.Fingerprint != fingerprint(user, f) || invalidCursorPosition(f.Sort, c.Position) {
			return out, ErrInvalid
		}
	}
	type sourceResult struct {
		index       int
		id          string
		allowed     bool
		accessError error
		page        reviewqueue.Page
		queryError  error
	}
	results := make([]sourceResult, len(s.Sources))
	var wg sync.WaitGroup
	for i, source := range s.Sources {
		wg.Add(1)
		go func(i int, source reviewqueue.Source) {
			defer wg.Done()
			result := sourceResult{index: i, id: source.ID}
			access, e := source.Access(ctx, user)
			if e != nil {
				result.accessError = e
				results[i] = result
				return
			}
			if access.Allowed {
				result.allowed = true
				if source.QueryAuthorized != nil {
					result.page, result.queryError = source.QueryAuthorized(ctx, user, f, c.Position, 31, access)
				} else {
					result.page, result.queryError = source.Query(ctx, user, f, c.Position, 31)
				}
			}
			results[i] = result
		}(i, source)
	}
	wg.Wait()
	authorized := false
	for _, result := range results {
		if result.accessError != nil || result.queryError != nil {
			if result.accessError != nil {
				slog.Warn("approval source unavailable", "source", result.id, "stage", "access", "error", result.accessError)
			} else {
				slog.Warn("approval source unavailable", "source", result.id, "stage", "query", "error", result.queryError)
			}
			out.Unavailable = append(out.Unavailable, result.id)
			continue
		}
		if !result.allowed {
			continue
		}
		authorized = true
		out.Items = append(out.Items, result.page.Items...)
		for k, n := range result.page.Counts {
			out.Counts[k] += n
		}
	}
	if !authorized && len(out.Unavailable) == 0 {
		return out, pgx.ErrNoRows
	}
	idSort := f.Sort == "id_asc" || f.Sort == "id_desc"
	descending := f.Sort == "time_desc" || f.Sort == "id_desc" || (f.Sort == "" && f.View == "history")
	slices.SortFunc(out.Items, func(a, b reviewqueue.Item) int {
		n := a.Time.Compare(b.Time)
		if idSort {
			switch {
			case a.ID < b.ID:
				n = -1
			case a.ID > b.ID:
				n = 1
			default:
				n = strings.Compare(a.Source, b.Source)
			}
		}
		if n == 0 {
			n = strings.Compare(a.Source, b.Source)
		}
		if n == 0 {
			if a.ID < b.ID {
				n = -1
			} else if a.ID > b.ID {
				n = 1
			}
		}
		if descending {
			n = -n
		}
		return n
	})
	// Do not advance past an unavailable source: retry the current page after recovery.
	if len(out.Unavailable) == 0 && len(out.Items) > 30 {
		out.Items = out.Items[:30]
		last := out.Items[29]
		b, _ := json.Marshal(cursor{reviewqueue.Position{Time: last.Time, Source: last.Source, ID: last.ID}, fingerprint(user, f)})
		out.Next = base64.RawURLEncoding.EncodeToString(b)
	} else if len(out.Items) > 30 {
		out.Items = out.Items[:30]
	}
	if s.Names != nil && len(out.Items) > 0 {
		ids := []string{}
		seen := map[string]bool{}
		for _, i := range out.Items {
			if i.Account != "" && !seen[i.Account] {
				seen[i.Account] = true
				ids = append(ids, i.Account)
			}
		}
		names, e := s.Names(ctx, ids)
		for i := range out.Items {
			if e == nil {
				out.Items[i].Applicant = names[out.Items[i].Account]
			}
			if out.Items[i].Applicant == "" {
				out.Items[i].Applicant = out.Items[i].Recipient
			}
		}
	}
	return out, nil
}

func invalidCursorPosition(sort string, p reviewqueue.Position) bool {
	if sort == "id_asc" || sort == "id_desc" {
		return p.ID <= 0
	}
	return p.Time.IsZero()
}

func (s *Service) Detail(ctx context.Context, user, source, id string) (reviewqueue.Item, error) {
	n, e := strconv.ParseInt(id, 10, 64)
	if e != nil || n <= 0 {
		return reviewqueue.Item{}, ErrInvalid
	}
	for _, p := range s.Sources {
		if p.ID != source {
			continue
		}
		a, e := p.Access(ctx, user)
		if e != nil {
			return reviewqueue.Item{}, e
		}
		if !a.Allowed {
			return reviewqueue.Item{}, pgx.ErrNoRows
		}
		filter := reviewqueue.Filter{View: "history", ID: n}
		var page reviewqueue.Page
		if p.QueryAuthorized != nil {
			page, e = p.QueryAuthorized(ctx, user, filter, reviewqueue.Position{}, 1, a)
		} else {
			page, e = p.Query(ctx, user, filter, reviewqueue.Position{}, 1)
		}
		if e != nil {
			return reviewqueue.Item{}, e
		}
		if len(page.Items) == 1 {
			return page.Items[0], nil
		}
	}
	return reviewqueue.Item{}, pgx.ErrNoRows
}
