package eve

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/migrations"
)

func TestContractCompletedItemsSurviveSourceAndGrantChanges(t *testing.T) {
	for _, change := range []string{"grant", "source", "legacy_pending"} {
		t.Run(change, func(t *testing.T) {
			s, ch := contractFixture(t)
			ctx := context.Background()
			calls := 0
			contractTransport(t, s, func(r *http.Request) *http.Response {
				if strings.HasSuffix(r.URL.Path, "/items/") {
					calls++
					return jsonResponse(json.RawMessage(`[{"record_id":100,"type_id":34,"quantity":7,"is_included":true,"is_singleton":false}]`))
				}
				return contractListResponse([]json.RawMessage{contractJSON(1, "auction")}, "1")
			})
			target := contractTarget(t, s, ch.ID, "character_contracts")
			if err := s.work(ctx, syncArgs{target.ID, target.Generation}, target.ActiveJobID.Int64, target.Resource); err != nil {
				t.Fatal(err)
			}
			if err := s.workContractDetail(ctx, contractDetailArgs{"character", ch.ID, 1, "items", ch.ID, target.Generation}, 1); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "grant":
				if err := s.auth.Save(ctx, ch); err != nil {
					t.Fatal(err)
				}
			case "source":
				if _, err := s.pool.Exec(ctx, "UPDATE eve_contract_details SET source_character_id=456"); err != nil {
					t.Fatal(err)
				}
			case "legacy_pending":
				if _, err := s.pool.Exec(ctx, "UPDATE eve_contract_details SET state='pending',reason='rate_limited' WHERE part='items'"); err != nil {
					t.Fatal(err)
				}
			}
			q := store.New(s.pool)
			c, err := q.GetCredential(ctx, ch.ID)
			if err != nil {
				t.Fatal(err)
			}
			target = contractTarget(t, s, ch.ID, "character_contracts")
			// A newly authorized list sees the same immutable contract contents.
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			var record contractRecord
			raw := contractJSON(1, "auction")
			if err = json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			record.raw = raw
			if err = s.saveContractPage(ctx, tx, target, c, &contractPage{kind: "character", owner: ch.ID, page: 1, pages: 1, records: []contractRecord{record}}); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			item, err := q.GetContractDetail(ctx, store.GetContractDetailParams{OwnerKind: "character", OwnerID: ch.ID, ContractID: 1, Part: "items"})
			if err != nil || item.State != "ready" || item.Reason != "" || !item.LastSuccessAt.Valid || item.Generation != c.GrantGeneration || item.SourceCharacterID != ch.ID {
				t.Fatalf("completed items became unavailable: %+v %v", item, err)
			}
			if err = s.workContractDetail(ctx, contractDetailArgs{"character", ch.ID, 1, "items", ch.ID, c.GrantGeneration}, 1); err != nil {
				t.Fatal(err)
			}
			var quantity int64
			if err = s.pool.QueryRow(ctx, "SELECT quantity FROM eve_contract_items WHERE contract_id=1").Scan(&quantity); err != nil || quantity != 7 || calls != 1 {
				t.Fatal("immutable items lost or refetched", quantity, calls, err)
			}
			bids, err := q.GetContractDetail(ctx, store.GetContractDetailParams{OwnerKind: "character", OwnerID: ch.ID, ContractID: 1, Part: "bids"})
			if err != nil || bids.State == "ready" {
				t.Fatal("unfetched bids marked complete", bids.State, err)
			}
		})
	}
}

func TestContractItemStateRepairIsNarrowAndIdempotent(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	target := contractTarget(t, s, ch.ID, "character_contracts")
	_, err := s.pool.Exec(ctx, `INSERT INTO eve_contracts(owner_kind,owner_id,contract_id,source_character_id,source_generation,contract_type,status,payload) VALUES('character',$1,1,$1,1,'auction','outstanding','{}');`, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO eve_contract_details(owner_kind,owner_id,contract_id,part,target_id,source_character_id,generation,state,last_success_at) VALUES('character',$1,1,'items',$2,$1,1,'pending',now()),('character',$1,1,'bids',$2,$1,1,'pending',now())`, ch.ID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.Files.ReadFile("00016_contract_item_state.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Split(strings.Split(string(raw), "-- +goose Up")[1], "-- +goose Down")[0]
	for range 2 {
		if _, err = s.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var items, bids string
	if err = s.pool.QueryRow(ctx, "SELECT max(state) FILTER(WHERE part='items'),max(state) FILTER(WHERE part='bids') FROM eve_contract_details").Scan(&items, &bids); err != nil || items != "ready" || bids != "pending" {
		t.Fatal(items, bids, err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE eve_contract_details SET state='blocked'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM eve_contract_details WHERE state='blocked'").Scan(&count); err != nil || count != 2 {
		t.Fatal("repair cleared an access failure", count, err)
	}
}
