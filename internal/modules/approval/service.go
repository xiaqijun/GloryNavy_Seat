package approval

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/approval/internal/store"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid approval query")

const defaultDualReadConcurrency = 4

type Service struct {
	Sources  []reviewqueue.Source
	Names    func(context.Context, []string) (map[string]string, error)
	Index    *store.Index
	index    approvalIndex
	UseIndex bool
	DualRead bool
	// AccessCacheTTL only caches the source scope used to filter the indexed
	// candidate list. Detail and decision paths always reauthorize directly at
	// the owning source.
	AccessCacheTTL time.Duration
	// IndexAccounts is the optional P3 gray list. Empty means all authenticated
	// users retain the P1 index path; a non-empty list limits it by account.
	IndexAccounts []string
	accessMu      sync.Mutex
	accessCache   map[string]accessCacheEntry
	dualReadMu    sync.Mutex
	dualReadSem   chan struct{}
}

type accessCacheEntry struct {
	access  reviewqueue.Access
	expires time.Time
}

// normalizeItem keeps every detail response compatible with the central
// approval JSON contract. Source-owned detail adapters commonly leave actions
// nil for records the current administrator cannot act on; encoding a nil
// slice as JSON null breaks the frontend response guard, which expects an
// array even when it is empty.
func normalizeItem(item *reviewqueue.Item) {
	if item == nil {
		return
	}
	if item.Actions == nil {
		item.Actions = []string{}
	}
	if len(item.Payload) == 0 || string(item.Payload) == "null" {
		item.Payload = json.RawMessage(`{}`)
	}
}

type approvalIndex interface {
	List(context.Context, []store.Scope, string, reviewqueue.Filter, reviewqueue.Position, int) (store.Result, error)
	Get(context.Context, []store.Scope, string, string, int64) (reviewqueue.Item, error)
	StaleSources(context.Context, time.Duration) ([]string, error)
	RecordDualReadDiff(context.Context, string, string, string, string, int64, int64, int64) error
}

func logSourceTiming(source, stage string, started time.Time, err error, attrs ...any) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	base := []any{"source", source, "stage", stage, "duration_ms", time.Since(started).Milliseconds(), "outcome", outcome}
	slog.Info("approval source timing", append(base, attrs...)...)
}

// NewService is the composition boundary for the central index. The private
// store remains owned by approval; callers only provide the identity name
// resolver and database pool.
func NewService(names func(context.Context, []string) (map[string]string, error), pool *pgxpool.Pool) *Service {
	s := &Service{Names: names, AccessCacheTTL: 5 * time.Second, accessCache: map[string]accessCacheEntry{}}
	if pool != nil {
		s.Index = &store.Index{Pool: pool}
	}
	return s
}

func (s *Service) approvalIndex() approvalIndex {
	if s.index != nil {
		return s.index
	}
	return s.Index
}

func (s *Service) sourceAccess(ctx context.Context, user string, source reviewqueue.Source, indexed bool) (reviewqueue.Access, error) {
	access := source.Access
	if indexed && source.IndexAccess != nil {
		access = source.IndexAccess
	}
	if access == nil {
		return reviewqueue.Access{}, errors.New("approval source access adapter missing")
	}
	if s.AccessCacheTTL <= 0 {
		return access(ctx, user)
	}
	key := user + "\x00" + source.ID
	// Most sources use the same authorization scope for the context shell and
	// indexed list. Only an explicit IndexAccess adapter needs a separate cache
	// entry because it may include extra list-only bindings or corporations.
	if indexed && source.IndexAccess != nil {
		key += "\x00index"
	}
	now := time.Now()
	s.accessMu.Lock()
	entry, ok := s.accessCache[key]
	s.accessMu.Unlock()
	if ok && now.Before(entry.expires) {
		return entry.access, nil
	}
	value, err := access(ctx, user)
	if err != nil {
		return reviewqueue.Access{}, err
	}
	s.accessMu.Lock()
	if s.accessCache == nil {
		s.accessCache = map[string]accessCacheEntry{}
	}
	s.accessCache[key] = accessCacheEntry{access: value, expires: now.Add(s.AccessCacheTTL)}
	s.accessMu.Unlock()
	return value, nil
}

func (s *Service) contextAccess(ctx context.Context, user string, source reviewqueue.Source) (reviewqueue.Access, error) {
	if source.ContextAccess == nil {
		return s.sourceAccess(ctx, user, source, false)
	}
	key := user + "\x00" + source.ID + "\x00context"
	if s.AccessCacheTTL <= 0 {
		return source.ContextAccess(ctx, user)
	}
	now := time.Now()
	s.accessMu.Lock()
	entry, ok := s.accessCache[key]
	s.accessMu.Unlock()
	if ok && now.Before(entry.expires) {
		return entry.access, nil
	}
	value, err := source.ContextAccess(ctx, user)
	if err != nil {
		return reviewqueue.Access{}, err
	}
	s.accessMu.Lock()
	if s.accessCache == nil {
		s.accessCache = map[string]accessCacheEntry{}
	}
	s.accessCache[key] = accessCacheEntry{access: value, expires: now.Add(s.AccessCacheTTL)}
	s.accessMu.Unlock()
	return value, nil
}

type Context struct {
	Allowed      bool                                `json:"allowed"`
	Sources      []string                            `json:"sources"`
	Capabilities map[string]reviewqueue.Capabilities `json:"capabilities"`
	Corporations []reviewqueue.Option                `json:"corporations"`
	Unavailable  []string                            `json:"unavailable"`
	People       []reviewqueue.Option                `json:"people"`
}

func (s *Service) Context(ctx context.Context, user string) (Context, error) {
	return s.ContextWithOptions(ctx, user, true)
}

// ContextWithOptions keeps the initial approval shell lightweight. Applicant
// options are only needed when the advanced filter is opened, so callers can
// skip the member/name projection on the first request.
func (s *Service) ContextWithOptions(ctx context.Context, user string, includePeople bool) (Context, error) {
	out := Context{Sources: []string{}, Capabilities: map[string]reviewqueue.Capabilities{}, Corporations: []reviewqueue.Option{}, Unavailable: []string{}, People: []reviewqueue.Option{}}
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
			accessStarted := time.Now()
			if includePeople {
				result.access, result.accessError = s.sourceAccess(ctx, user, source, false)
			} else {
				result.access, result.accessError = s.contextAccess(ctx, user, source)
			}
			logSourceTiming(source.ID, "access", accessStarted, result.accessError, "indexed", false, "allowed", result.access.Allowed)
			if includePeople && result.accessError == nil && result.access.Allowed && (source.People != nil || source.PeopleAuthorized != nil) {
				peopleStarted := time.Now()
				if source.PeopleAuthorized != nil {
					result.people, result.peopleError = source.PeopleAuthorized(ctx, user, result.access)
				} else {
					result.people, result.peopleError = source.People(ctx, user)
				}
				logSourceTiming(source.ID, "people", peopleStarted, result.peopleError, "indexed", false, "count", len(result.people))
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
		for _, source := range s.Sources {
			if source.ID == result.id {
				out.Capabilities[result.id] = source.Capabilities
				break
			}
		}
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
		namesStarted := time.Now()
		ids := []string{}
		for id := range people {
			ids = append(ids, id)
		}
		names, e := s.Names(ctx, ids)
		logSourceTiming("identity", "names", namesStarted, e, "indexed", false, "count", len(ids))
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
	// SourceStatus distinguishes an empty source from one that is unavailable
	// or whose projection has fallen behind. It is deliberately read metadata;
	// source decisions still come from the owning module.
	SourceStatus map[string]string `json:"source_status"`
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
	if s.useIndexFor(user) {
		out, err := s.listIndexed(ctx, user, f, token)
		if s.DualRead && err == nil {
			s.scheduleDualRead(user, f, token, out)
		}
		return out, err
	}
	return s.listLegacy(ctx, user, f, token)
}

func (s *Service) scheduleDualRead(user string, f reviewqueue.Filter, token string, indexed Result) {
	s.dualReadMu.Lock()
	if s.dualReadSem == nil {
		s.dualReadSem = make(chan struct{}, defaultDualReadConcurrency)
	}
	sem := s.dualReadSem
	s.dualReadMu.Unlock()
	select {
	case sem <- struct{}{}:
		go func() {
			defer func() { <-sem }()
			s.compareAsync(user, f, token, indexed)
		}()
	default:
		slog.Warn("approval dual-read skipped", "reason", "concurrency_limit")
	}
}

func (s *Service) useIndexFor(user string) bool {
	if !s.UseIndex || s.approvalIndex() == nil {
		return false
	}
	return len(s.IndexAccounts) == 0 || slices.Contains(s.IndexAccounts, user)
}

func (s *Service) compareAsync(user string, f reviewqueue.Filter, token string, indexed Result) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	legacy, err := s.listLegacy(ctx, user, f, token)
	if err != nil {
		slog.Warn("approval dual-read legacy failed", "error", err)
		s.persistDualReadDiff(ctx, user, f, "legacy_error", indexed, Result{})
		return
	}
	if reason := compareResults(indexed, legacy); reason != "" {
		slog.Warn("approval dual-read mismatch", "reason", reason, "view", f.View, "sort", f.Sort, "kind", f.Kind, "corporation", f.Corporation)
		s.persistDualReadDiff(ctx, user, f, reason, indexed, legacy)
	}
}

func (s *Service) persistDualReadDiff(ctx context.Context, user string, f reviewqueue.Filter, reason string, indexed, legacy Result) {
	index := s.approvalIndex()
	if index == nil {
		return
	}
	filter, _ := json.Marshal(f)
	source, sourceID, indexedVersion, legacyVersion := mismatchCoordinate(reason, indexed, legacy)
	if err := index.RecordDualReadDiff(ctx, user, string(filter), reason, source, sourceID, indexedVersion, legacyVersion); err != nil {
		slog.Warn("approval dual-read diff persistence failed", "reason", reason, "error", err)
	}
}

func mismatchCoordinate(reason string, indexed, legacy Result) (string, int64, int64, int64) {
	if !strings.HasPrefix(reason, "item_") {
		return "", 0, 0, 0
	}
	parts := strings.SplitN(reason, "_", 3)
	if len(parts) < 3 {
		return "", 0, 0, 0
	}
	index, err := strconv.Atoi(parts[1])
	if err != nil || index < 0 || index >= len(indexed.Items) || index >= len(legacy.Items) {
		return "", 0, 0, 0
	}
	left, right := indexed.Items[index], legacy.Items[index]
	source, sourceID := left.Source, left.ID
	if source == "" {
		source, sourceID = right.Source, right.ID
	}
	return source, sourceID, left.Version, right.Version
}

func compareResults(indexed, legacy Result) string {
	if reason := compareStringSet(indexed.Unavailable, legacy.Unavailable); reason != "" {
		return "unavailable_" + reason
	}
	countKeys := map[string]bool{}
	for bucket := range indexed.Counts {
		countKeys[bucket] = true
	}
	for bucket := range legacy.Counts {
		countKeys[bucket] = true
	}
	for bucket := range countKeys {
		if indexed.Counts[bucket] != legacy.Counts[bucket] {
			return "count_" + bucket
		}
	}
	if indexed.Next != legacy.Next {
		return "next_cursor"
	}
	if len(indexed.Items) != len(legacy.Items) {
		return "item_count"
	}
	for i := range indexed.Items {
		if field := compareItem(indexed.Items[i], legacy.Items[i]); field != "" {
			return "item_" + strconv.Itoa(i) + "_" + field
		}
	}
	return ""
}

func compareStringSet(a, b []string) string {
	if len(a) != len(b) {
		return "count"
	}
	left, right := append([]string(nil), a...), append([]string(nil), b...)
	slices.Sort(left)
	slices.Sort(right)
	for i := range left {
		if left[i] != right[i] {
			return "source"
		}
	}
	return ""
}

func compareItem(a, b reviewqueue.Item) string {
	checks := []struct {
		name string
		diff bool
	}{
		{"source", a.Source != b.Source},
		{"id", a.ID != b.ID},
		{"version", a.Version != b.Version},
		{"account", a.Account != b.Account},
		{"applicant", a.Applicant != b.Applicant},
		{"corporation", a.Corporation != b.Corporation},
		{"kind", a.Kind != b.Kind},
		{"state", a.State != b.State},
		{"status", a.Status != b.Status},
		// ProcessedBy and History are internal authorization/counting metadata;
		// the legacy source adapter does not serialize them in its page items.
		// Counts and Mine filtering are compared through their observable result.
		{"recipient", a.Recipient != b.Recipient},
		{"title", a.Title != b.Title},
		{"reference", a.Reference != b.Reference},
		{"amount", a.Amount != b.Amount},
		{"unit", a.Unit != b.Unit},
		{"time", !a.Time.UTC().Equal(b.Time.UTC())},
		{"action", a.Action != b.Action},
		{"actions", !slices.Equal(a.Actions, b.Actions)},
	}
	for _, check := range checks {
		if check.diff {
			return check.name
		}
	}
	return ""
}

func (s *Service) listIndexed(ctx context.Context, user string, f reviewqueue.Filter, token string) (Result, error) {
	out := Result{Items: []reviewqueue.Item{}, Counts: map[string]int64{}, Unavailable: []string{}, SourceStatus: map[string]string{}}
	index := s.approvalIndex()
	if index == nil {
		return out, store.ErrUnavailable
	}
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
	type result struct {
		id string
		a  reviewqueue.Access
		e  error
	}
	results := make([]result, len(s.Sources))
	var wg sync.WaitGroup
	for i, source := range s.Sources {
		wg.Add(1)
		go func(i int, source reviewqueue.Source) {
			defer wg.Done()
			accessStarted := time.Now()
			a, e := s.sourceAccess(ctx, user, source, true)
			logSourceTiming(source.ID, "access", accessStarted, e, "indexed", true, "allowed", a.Allowed)
			results[i] = result{id: source.ID, a: a, e: e}
		}(i, source)
	}
	wg.Wait()
	access := map[string]reviewqueue.Access{}
	authorized := false
	for _, r := range results {
		if r.e != nil {
			out.Unavailable = append(out.Unavailable, r.id)
			out.SourceStatus[r.id] = "unavailable"
			continue
		}
		if r.a.Allowed {
			authorized = true
			access[r.id] = r.a
			out.SourceStatus[r.id] = "available"
		}
	}
	if !authorized && len(out.Unavailable) == 0 {
		return out, pgx.ErrNoRows
	}
	if stale, err := index.StaleSources(ctx, 5*time.Minute); err == nil {
		for _, source := range stale {
			if _, ok := access[source]; ok && !slices.Contains(out.Unavailable, source) {
				out.SourceStatus[source] = "stale"
			}
		}
	}
	indexStarted := time.Now()
	page, e := index.List(ctx, scopes(access), user, f, c.Position, 30)
	logSourceTiming("approval", "index", indexStarted, e, "indexed", true, "count", len(page.Items))
	if e != nil {
		return out, e
	}
	out.Items, out.Counts = page.Items, page.Counts
	if len(out.Items) > 0 {
		ids := make([]string, 0, len(out.Items))
		seen := map[string]bool{}
		for _, item := range out.Items {
			if item.Account != "" && !seen[item.Account] {
				seen[item.Account] = true
				ids = append(ids, item.Account)
			}
		}
		names := map[string]string{}
		if s.Names != nil && len(ids) > 0 {
			namesStarted := time.Now()
			if resolved, err := s.Names(ctx, ids); err == nil {
				names = resolved
				logSourceTiming("identity", "names", namesStarted, nil, "indexed", true, "count", len(ids))
			} else {
				logSourceTiming("identity", "names", namesStarted, err, "indexed", true, "count", len(ids))
			}
		}
		for i := range out.Items {
			out.Items[i].SummaryVersion = out.Items[i].Version
			out.Items[i].SourceStatus = out.SourceStatus[out.Items[i].Source]
			out.Items[i].Stale = out.Items[i].SourceStatus == "stale"
			for _, source := range s.Sources {
				if source.ID == out.Items[i].Source {
					out.Items[i].DetailKind = source.Capabilities.DetailKind
					break
				}
			}
			if name := names[out.Items[i].Account]; name != "" {
				out.Items[i].Applicant = name
			}
			if out.Items[i].Applicant == "" {
				out.Items[i].Applicant = out.Items[i].Recipient
			}
			for _, source := range s.Sources {
				if source.ID == out.Items[i].Source && source.Decorate != nil {
					if err := source.Decorate(ctx, user, &out.Items[i]); err != nil {
						if !slices.Contains(out.Unavailable, source.ID) {
							out.Unavailable = append(out.Unavailable, source.ID)
						}
						out.SourceStatus[source.ID] = "unavailable"
						break
					}
				}
			}
		}
	}
	if len(out.Unavailable) == 0 && page.HasNext {
		b, _ := json.Marshal(cursor{Position: page.Next, Fingerprint: fingerprint(user, f)})
		out.Next = base64.RawURLEncoding.EncodeToString(b)
	}
	return out, nil
}

func (s *Service) listLegacy(ctx context.Context, user string, f reviewqueue.Filter, token string) (Result, error) {
	out := Result{Items: []reviewqueue.Item{}, Counts: map[string]int64{}, Unavailable: []string{}, SourceStatus: map[string]string{}}
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
			accessStarted := time.Now()
			access, e := s.sourceAccess(ctx, user, source, false)
			logSourceTiming(source.ID, "access", accessStarted, e, "indexed", false, "allowed", access.Allowed)
			if e != nil {
				result.accessError = e
				results[i] = result
				return
			}
			if access.Allowed {
				result.allowed = true
				if source.Query == nil && source.QueryAuthorized == nil {
					result.queryError = fmt.Errorf("source is index-only")
				} else if source.QueryAuthorized != nil {
					queryStarted := time.Now()
					result.page, result.queryError = source.QueryAuthorized(ctx, user, f, c.Position, 31, access)
					logSourceTiming(source.ID, "query", queryStarted, result.queryError, "indexed", false, "count", len(result.page.Items))
				} else {
					queryStarted := time.Now()
					result.page, result.queryError = source.Query(ctx, user, f, c.Position, 31)
					logSourceTiming(source.ID, "query", queryStarted, result.queryError, "indexed", false, "count", len(result.page.Items))
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
			out.SourceStatus[result.id] = "unavailable"
			continue
		}
		if !result.allowed {
			continue
		}
		authorized = true
		out.SourceStatus[result.id] = "available"
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
	for i := range out.Items {
		out.Items[i].SummaryVersion = out.Items[i].Version
		out.Items[i].SourceStatus = out.SourceStatus[out.Items[i].Source]
		for _, source := range s.Sources {
			if source.ID == out.Items[i].Source {
				out.Items[i].DetailKind = source.Capabilities.DetailKind
				break
			}
		}
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
		namesStarted := time.Now()
		names, e := s.Names(ctx, ids)
		logSourceTiming("identity", "names", namesStarted, e, "indexed", false, "count", len(ids))
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
		started := time.Now()
		if p.Detail != nil {
			// Keep the approval center's management boundary before entering a
			// source detail adapter. Some source-owned read APIs also allow an
			// applicant to read their own record, which must not widen this
			// management-only route.
			a, err := p.Access(ctx, user)
			if err != nil {
				logSourceTiming(source, "detail_access", started, err, "path", "source")
				return reviewqueue.Item{}, err
			}
			if !a.Allowed {
				return reviewqueue.Item{}, pgx.ErrNoRows
			}
			item, err := p.Detail(ctx, user, n)
			logSourceTiming(source, "detail", started, err, "path", "source")
			if err != nil {
				return reviewqueue.Item{}, err
			}
			normalizeItem(&item)
			item.Source = source
			item.DetailKind = p.Capabilities.DetailKind
			item.SummaryVersion = item.Version
			return item, nil
		}
		// A source without private enrichment can use the central projection's
		// single-row read. This deliberately avoids List's counted CTE and the
		// legacy source queue fan-out.
		if s.UseIndex && s.approvalIndex() != nil {
			a, err := s.sourceAccess(ctx, user, p, true)
			if err != nil {
				logSourceTiming(source, "detail_access", started, err, "path", "index")
				return reviewqueue.Item{}, err
			}
			if !a.Allowed {
				return reviewqueue.Item{}, pgx.ErrNoRows
			}
			item, err := s.approvalIndex().Get(ctx, scopes(map[string]reviewqueue.Access{source: a}), user, source, n)
			if err != nil {
				logSourceTiming(source, "detail_index", started, err, "path", "index")
				return reviewqueue.Item{}, err
			}
			item.DetailKind = p.Capabilities.DetailKind
			item.SummaryVersion = item.Version
			if p.Decorate != nil {
				if err = p.Decorate(ctx, user, &item); err != nil {
					return reviewqueue.Item{}, err
				}
			}
			normalizeItem(&item)
			logSourceTiming(source, "detail_index", started, nil, "path", "index")
			return item, nil
		}
		a, e := p.Access(ctx, user)
		if e != nil {
			return reviewqueue.Item{}, e
		}
		if !a.Allowed {
			return reviewqueue.Item{}, pgx.ErrNoRows
		}
		// IndexOnly sources may participate in the central list before their
		// legacy detail adapter is available. Fail closed with a normal service
		// error instead of dereferencing a nil query function.
		if p.QueryAuthorized == nil && p.Query == nil {
			return reviewqueue.Item{}, store.ErrUnavailable
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
			normalizeItem(&page.Items[0])
			return page.Items[0], nil
		}
	}
	return reviewqueue.Item{}, pgx.ErrNoRows
}
