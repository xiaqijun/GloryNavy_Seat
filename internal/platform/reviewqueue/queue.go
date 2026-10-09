// Package reviewqueue defines the read-only contract between approval sources.
// Source modules retain ownership of SQL, authorization and all mutations.
package reviewqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Filter struct {
	View        string `json:"view"`
	Sort        string `json:"sort"`
	Kind        string `json:"kind"`
	Corporation string `json:"corporation"`
	Account     string `json:"account"`
	Search      string `json:"search"`
	Status      string `json:"status"`
	From        string `json:"from"`
	Until       string `json:"until"`
	Mine        bool   `json:"mine"`
	ID          int64  `json:"-"`
}
type Position struct {
	Time   time.Time `json:"time"`
	Source string    `json:"source"`
	ID     int64     `json:"id"`
}
type Item struct {
	Source         string          `json:"source"`
	ID             int64           `json:"id,string"`
	Version        int64           `json:"version,string"`
	SummaryVersion int64           `json:"summary_version,string,omitempty"`
	DetailKind     string          `json:"detail_kind,omitempty"`
	SourceStatus   string          `json:"source_status,omitempty"`
	Stale          bool            `json:"stale,omitempty"`
	Account        string          `json:"account_id"`
	Applicant      string          `json:"applicant"`
	Corporation    string          `json:"corporation_id"`
	Kind           string          `json:"kind"`
	State          string          `json:"state"`
	Status         string          `json:"status"`
	ProcessedBy    []string        `json:"-"`
	History        bool            `json:"-"`
	Recipient      string          `json:"recipient"`
	Title          string          `json:"title"`
	Reference      string          `json:"reference"`
	Amount         int64           `json:"amount_minor"`
	Unit           string          `json:"unit"`
	Time           time.Time       `json:"time"`
	Action         string          `json:"action"`
	Actions        []string        `json:"actions"`
	Payload        json.RawMessage `json:"payload"`
}
type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Binding struct {
	Account   string `json:"account"`
	Recipient string `json:"recipient"`
}
type Access struct {
	Allowed               bool                `json:"allowed"`
	Corporations          []Option            `json:"corporations"`
	Accounts              []string            `json:"-"`
	AccountsByCorporation map[string][]string `json:"-"`
	Bindings              []Binding           `json:"-"`
	RestrictBindings      bool                `json:"-"`
}

// Capabilities describes the operations and list filters a source can expose
// in the central approval contract. It is metadata only: the source still
// owns the final authorization and decision checks.
type Capabilities struct {
	Approve           bool   `json:"approve"`
	Reject            bool   `json:"reject"`
	CancelReview      bool   `json:"cancel_review"`
	Fulfill           bool   `json:"fulfill"`
	BatchSettle       bool   `json:"batch_settle"`
	CorporationFilter bool   `json:"corporation_filter"`
	ApplicantFilter   bool   `json:"applicant_filter"`
	Amount            bool   `json:"amount"`
	DetailKind        string `json:"detail_kind"`
}
type Page struct {
	Items  []Item           `json:"items"`
	Counts map[string]int64 `json:"counts"`
}
type Source struct {
	ID           string
	Capabilities Capabilities
	Access       func(context.Context, string) (Access, error)
	// IndexAccess may load the additional scope needed by the central index;
	// the regular Access path stays lightweight for the initial context shell.
	IndexAccess func(context.Context, string) (Access, error)
	Query       func(context.Context, string, Filter, Position, int) (Page, error)
	// IndexOnly allows a new source to participate in the central indexed list
	// before it has a legacy fan-out query. Legacy fallback marks that source
	// unavailable instead of dereferencing a nil function.
	IndexOnly bool
	// QueryAuthorized receives the access result already collected by the
	// approval aggregator. Sources may use it to avoid repeating the same
	// permission and scope reads before building their projection.
	QueryAuthorized  func(context.Context, string, Filter, Position, int, Access) (Page, error)
	People           func(context.Context, string) ([]string, error)
	PeopleAuthorized func(context.Context, string, Access) ([]string, error)
	// Snapshot returns the source-owned list projection for the central approval
	// index. It must not expose source-private store types; the approval module
	// stores only the returned summary fields and still delegates details and
	// decisions back to the source.
	Snapshot func(context.Context) ([]Item, error)
	// Decorate applies actor-specific actions after an item is read from the
	// central index. It must not mutate source state.
	Decorate func(context.Context, string, *Item) error
}

// ValidateSources is called at composition time so an incomplete future
// adapter fails closed instead of silently producing an empty queue.
func ValidateSources(sources []Source) error {
	seen := map[string]bool{}
	for _, source := range sources {
		if strings.TrimSpace(source.ID) == "" || seen[source.ID] {
			return fmt.Errorf("invalid approval source id %q", source.ID)
		}
		seen[source.ID] = true
		if source.Access == nil || source.Snapshot == nil || (!source.IndexOnly && source.Query == nil) {
			return fmt.Errorf("approval source %q is missing access, snapshot, or legacy query adapter", source.ID)
		}
		if source.Capabilities.DetailKind == "" {
			return fmt.Errorf("approval source %q is missing detail kind", source.ID)
		}
	}
	return nil
}

func SortedSourceIDs(sources []Source) []string {
	ids := make([]string, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.ID)
	}
	slices.Sort(ids)
	return ids
}
