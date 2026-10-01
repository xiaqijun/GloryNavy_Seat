package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/migrations"
)

func scopeContract(id int, issuerCorp, assignee, acceptor int64, forCorp bool) json.RawMessage {
	var data map[string]any
	_ = json.Unmarshal(contractJSON(id, "item_exchange"), &data)
	data["issuer_corporation_id"] = issuerCorp
	data["assignee_id"] = assignee
	data["acceptor_id"] = acceptor
	data["for_corporation"] = forCorp
	raw, _ := json.Marshal(data)
	return raw
}

func TestCorporationContractScopeAndAllReadEndpoints(t *testing.T) {
	s, _ := contractFixture(t)
	ctx := context.Background()
	q := store.New(s.pool)
	cases := []struct {
		name                       string
		issuer, assignee, acceptor int64
		forCorp, want              bool
	}{
		{"own issuer to alliance", 10, 99, 0, true, true},
		{"assigned corporation", 20, 10, 0, false, true},
		{"accepted corporation", 20, 99, 10, false, true},
		{"alliance only", 20, 99, 0, false, false},
		{"member issued personally", 10, 99, 0, false, false},
		{"member accepted personally", 20, 99, 123, false, false},
		{"other corporation issuer", 20, 99, 0, true, false},
	}
	for i, tc := range cases {
		raw := scopeContract(i+1, tc.issuer, tc.assignee, tc.acceptor, tc.forCorp)
		got, err := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "corporation", OwnerID: 10, ContractID: int64(i + 1), SourceCharacterID: 123, SourceGeneration: 2, ContractType: "item_exchange", Status: "outstanding", Payload: raw})
		if err != nil || got != tc.want {
			t.Fatalf("%s: scope=%v want=%v err=%v", tc.name, got, tc.want, err)
		}
		// Personal contract visibility must not inherit the corporation predicate.
		personal, err := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "character", OwnerID: 123, ContractID: int64(i + 1), SourceCharacterID: 123, SourceGeneration: 2, ContractType: "item_exchange", Status: "outstanding", Payload: raw})
		if err != nil || !personal {
			t.Fatal("personal scope changed", err)
		}
	}
	// Out-of-scope high IDs must not consume the SQL limit and hide eligible rows.
	for i := 100; i < 135; i++ {
		if _, err := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "corporation", OwnerID: 10, ContractID: int64(i), ContractType: "item_exchange", Status: "outstanding", Payload: scopeContract(i, 20, 99, 0, false)}); err != nil {
			t.Fatal(err)
		}
	}
	h := NewContractHTTP(s.pool, nil, func(*http.Request) string { return "authorized-manager" })
	h.Owners = func(context.Context, string) ([]ContractOwner, error) {
		return []ContractOwner{{Kind: "corporation", ID: 10}, {Kind: "character", ID: 123}}, nil
	}
	router := chi.NewRouter()
	for _, r := range h.Routes() {
		router.Method(r.Method, r.Path, r.Handler)
	}
	get := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	var page struct {
		Data struct {
			Items []contractView `json:"items"`
			Next  string         `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(get("/contracts/corporation/10", 200), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data.Items) != 3 || page.Data.Next != "" || page.Data.Items[0].ID != "3" {
		t.Fatalf("incorrect filtered pagination: %+v", page)
	}
	for _, path := range []string{"/contracts/corporation/10/4", "/contracts/corporation/10/4/items", "/contracts/corporation/10/4/bids"} {
		get(path, 404)
	}
	get("/contracts/corporation/10/1", 200)
	get("/contracts/character/123/4", 200)
	get("/contracts/corporation/11/1", 404)
}

func TestExcludedContractPageCompletesWithoutDetailJobs(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	setContractCorporation(t, s, ch, 10)
	target := contractTarget(t, s, ch.ID, "corporation_contracts")
	contractTransport(t, s, func(*http.Request) *http.Response {
		return contractListResponse([]json.RawMessage{scopeContract(1, 20, 99, 0, false)}, "2")
	})
	if err := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource); err == nil {
		t.Fatal("partial page should continue")
	}
	q := store.New(s.pool)
	cursor, err := q.GetContractCursor(ctx, target.ID)
	if err != nil || cursor.Page != 2 {
		t.Fatal("filtered page lost upstream cursor", cursor, err)
	}
	var n int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE kind='eve.contract-detail.v1'").Scan(&n); err != nil || n != 0 {
		t.Fatal("queued irrelevant details", n, err)
	}
	contractTransport(t, s, func(*http.Request) *http.Response {
		return contractListResponse([]json.RawMessage{scopeContract(2, 20, 99, 0, false)}, "2")
	})
	if _, err = s.pool.Exec(ctx, "UPDATE eve_sync_targets SET next_due_at=now() WHERE id=$1", target.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource); err != nil {
		t.Fatal(err)
	}
	if _, err = q.GetContractCursor(ctx, target.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cursor not completed", err)
	}
	if !contractTarget(t, s, ch.ID, target.Resource).LastSuccessAt.Valid {
		t.Fatal("filtered round not marked successful")
	}
	called := false
	contractTransport(t, s, func(*http.Request) *http.Response { called = true; return jsonResponse([]any{}) })
	err = s.workContractDetail(ctx, contractDetailArgs{"corporation", 10, 1, "items", ch.ID, target.Generation}, 1)
	var cancelled *river.JobCancelError
	if !errors.As(err, &cancelled) || called {
		t.Fatal("old excluded job accessed ESI", err, called)
	}
}

func TestContractScopeMigrationPreservesMetadataAndStopsDetails(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	q := store.New(s.pool)
	target := contractTarget(t, s, ch.ID, "corporation_contracts")
	for i, assignee := range []int64{99, 10} {
		id := int64(i + 1)
		if _, err := q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "corporation", OwnerID: 10, ContractID: id, SourceCharacterID: ch.ID, SourceGeneration: target.Generation, ContractType: "item_exchange", Status: "outstanding", Payload: scopeContract(i+1, 20, assignee, 0, false)}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.PrepareContractDetail(ctx, store.PrepareContractDetailParams{OwnerKind: "corporation", OwnerID: 10, ContractID: id, Part: "items", TargetID: target.ID, SourceCharacterID: ch.ID, Generation: target.Generation}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := migrations.Files.ReadFile("00017_corporation_contract_scope.sql")
	if err != nil {
		t.Fatal(err)
	}
	sections := strings.Split(strings.Split(string(raw), "-- +goose Up")[1], "-- +goose Down")
	if _, err = s.pool.Exec(ctx, sections[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, sections[0]); err != nil {
		t.Fatal(err)
	}
	d, err := q.GetContractDetail(ctx, store.GetContractDetailParams{OwnerKind: "corporation", OwnerID: 10, ContractID: 1, Part: "items"})
	if err != nil || d.State != "blocked" || d.Reason != "out_of_scope" || d.Fence != 1 {
		t.Fatal("migration failed to fence excluded detail", d, err)
	}
	stats, err := q.ContractDetailStats(ctx, target.ID)
	if err != nil || stats.Pending != 1 || stats.Failed != 0 {
		t.Fatal("excluded details counted as sync failure", stats, err)
	}
	var n int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contracts").Scan(&n); err != nil || n != 2 {
		t.Fatal("historical metadata removed", n, err)
	}
	// An alliance contract later accepted by this corporation becomes relevant.
	if _, err = q.UpsertContract(ctx, store.UpsertContractParams{OwnerKind: "corporation", OwnerID: 10, ContractID: 1, SourceCharacterID: ch.ID, SourceGeneration: target.Generation, ContractType: "item_exchange", Status: "finished", Payload: scopeContract(1, 20, 99, 10, false)}); err != nil {
		t.Fatal(err)
	}
	next, err := q.PrepareContractDetail(ctx, store.PrepareContractDetailParams{OwnerKind: "corporation", OwnerID: 10, ContractID: 1, Part: "items", TargetID: target.ID, SourceCharacterID: ch.ID, Generation: target.Generation})
	if err != nil || next.State != "pending" || next.Reason != "" {
		t.Fatal(fmt.Sprintf("relevant detail did not resume: %+v", next), err)
	}
}
