package eve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type brokenContractBody struct{}

func (brokenContractBody) Read(p []byte) (int, error) { return copy(p, `[]`), io.ErrUnexpectedEOF }
func (brokenContractBody) Close() error               { return nil }

func TestContractInterruptedBodyRetriesWithoutPublishingOrBlocking(t *testing.T) {
	s, ch := contractFixture(t)
	ctx := context.Background()
	broken := true
	contractTransport(t, s, func(*http.Request) *http.Response {
		r := contractListResponse(nil, "1")
		if broken {
			r.Body = brokenContractBody{}
		} else {
			r = contractListResponse([]json.RawMessage{contractJSON(88, "courier")}, "1")
		}
		return r
	})
	target := contractTarget(t, s, ch.ID, "character_contracts")
	args := syncArgs{target.ID, target.Generation}
	if err := s.work(ctx, args, target.ActiveJobID.Int64, target.Resource); err == nil {
		t.Fatal("interrupted body should defer")
	}
	current := contractTarget(t, s, ch.ID, target.Resource)
	if current.State != "deferred" || current.Reason != "network_error" || current.LastSuccessAt.Valid {
		t.Fatalf("wrong failure state: %s/%s", current.State, current.Reason)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM eve_contracts)+(SELECT count(*) FROM eve_esi_cache)`).Scan(&count); err != nil || count != 0 {
		t.Fatal("incomplete response published or cached", count, err)
	}
	broken = false
	if _, err := s.pool.Exec(ctx, `UPDATE eve_sync_targets SET next_due_at=now() WHERE id=$1`, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.work(ctx, args, target.ActiveJobID.Int64, target.Resource); err != nil {
		t.Fatal(err)
	}
	current = contractTarget(t, s, ch.ID, target.Resource)
	if current.State != "idle" || !current.LastSuccessAt.Valid {
		t.Fatal("retry did not recover", current.State)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM eve_contracts`).Scan(&count); err != nil || count != 1 {
		t.Fatal("retry did not publish contract", count, err)
	}
}
