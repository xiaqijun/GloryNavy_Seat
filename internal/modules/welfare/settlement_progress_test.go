package welfare

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"glorynavy.local/seat/migrations"
)

func TestSettlementRepairedReferenceLifecycle(t *testing.T) {
	s, coins := fixture(t)
	ctx := context.Background()
	s.MainCharacterID = func(context.Context, string) (int64, error) { return 123, nil }
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{456: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	s.SettlementCompleteTx = s.CompleteMergedDeliveryTx
	s.ExchangeSettlementCompleteTx = coins.CompleteMergedDeliveryTx
	var selection []SettlementInputItem
	for _, amount := range []int64{10099, 20099} {
		row, err := store.Save(ctx, s.Pool, Case{AccountID: userID, CorporationID: 10, Kind: "solo", State: "approved", Award: amount, Detail: raw(Detail{CharacterID: 123}), Keys: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		selection = append(selection, SettlementInputItem{Source: "welfare", ID: row.ID})
	}
	view, err := s.CreateSettlementBatch(ctx, adminID, SettlementInput{RequestKey: key(801), Items: selection})
	if err != nil {
		t.Fatal(err)
	}
	b := view.Batch
	legacy := "BATCH-20261013-4D12658A-3998-4E78-B853-7E31D991529A"
	if _, err = s.Pool.Exec(ctx, `INSERT INTO welfare_contract_settlement_reference_repairs(batch_id,previous_reference,replacement_reference,operator_label,reason) VALUES($1,$2,$3,'test','explicit legacy repair')`, b.ID, legacy, b.SettlementReference); err != nil {
		t.Fatal(err)
	}
	// Only the recorded replacement may read the alias; a stale caller cannot
	// attach an arbitrary old title to a different current reference.
	aliases, err := store.SettlementReferenceAliases(ctx, s.Pool, b.ID, "BATCH-20261013-999999")
	if err != nil || len(aliases) != 0 {
		t.Fatal(aliases, err)
	}
	contract := eve.DeliveryContract{ID: 900, Type: "item_exchange", Title: legacy[:50], IssuerID: 456, AssigneeID: 123, Price: "0", Reward: "301", ItemsReady: true, Status: "outstanding", Issued: time.Now().Add(-time.Hour), ContentToken: "fixed"}
	var candidates []eve.DeliveryContract
	var readErr error
	s.PaymentContracts = func(_ context.Context, _ pgx.Tx, account string, recipient int64, ref string, _ time.Time) ([]eve.DeliveryContract, error) {
		if account != userID || recipient != 123 {
			t.Fatalf("wrong scope %s/%d", account, recipient)
		}
		if readErr != nil {
			return nil, readErr
		}
		var out []eve.DeliveryContract
		for _, c := range candidates {
			if settlementContractTitleMatches(c.Title, ref) {
				out = append(out, c)
			}
		}
		return out, nil
	}
	check := func(want string, id int64) {
		t.Helper()
		if _, err := s.Pool.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET next_run_at=now() WHERE id=$1`, b.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.ProcessSettlementBatch(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
		got, err := s.SettlementBatch(ctx, adminID, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Batch.DeliveryStatus != want || got.Batch.CompletedCount != 0 {
			t.Fatalf("progress %+v", got.Batch)
		}
		if id == 0 && got.Batch.ContractID != nil || id != 0 && (got.Batch.ContractID == nil || *got.Batch.ContractID != id) {
			t.Fatalf("contract=%v want %d", got.Batch.ContractID, id)
		}
		for _, it := range selection {
			row, err := store.Read(ctx, s.Pool, it.ID)
			if err != nil || row.State != "approved" {
				t.Fatalf("premature payment %+v %v", row, err)
			}
		}
	}
	check("waiting_contract", 0)
	dead := contract
	dead.Status = "deleted"
	candidates = []eve.DeliveryContract{dead}
	check("contract_deleted", 900)
	live := contract
	live.ID = 901
	live.Title = b.SettlementReference
	candidates = append(candidates, live)
	check("awaiting_acceptance", 901)
	// Two distinct live contracts spanning both references must block settlement.
	candidates = []eve.DeliveryContract{contract, live}
	check("multiple_contracts", 0)
	for _, tc := range []struct {
		name string
		edit func(*eve.DeliveryContract)
	}{
		{"amount", func(c *eve.DeliveryContract) { c.Reward = "302" }},
		{"recipient", func(c *eve.DeliveryContract) { c.AssigneeID = 999 }},
		{"items", func(c *eve.DeliveryContract) { c.Items = []eve.DeliveryItem{{TypeID: 34, Quantity: 1, Included: true}} }},
		{"requested cash", func(c *eve.DeliveryContract) { c.Price = "1" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := contract
			tc.edit(&c)
			candidates = []eve.DeliveryContract{c}
			check("contract_mismatch", c.ID)
		})
	}
	candidates = []eve.DeliveryContract{contract, contract}
	check("awaiting_acceptance", 900) // duplicated observations are one contract
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{456: otherID}, nil
	}
	check("", 900) // no longer an administrator, never reported ready to accept
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{456: adminID}, nil
	}
	check("awaiting_acceptance", 900)
	readErr = errors.New("cache unavailable")
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET next_run_at=now() WHERE id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessSettlementBatch(ctx, b.ID); !errors.Is(err, readErr) {
		t.Fatal(err)
	}
	got, err := s.SettlementBatch(ctx, adminID, b.ID)
	if err != nil || got.Batch.ContractID == nil || *got.Batch.ContractID != 900 || got.Batch.DeliveryStatus != "awaiting_acceptance" {
		t.Fatal("read failure erased evidence", got, err)
	}
	readErr = nil
	contract.Status = "finished"
	contract.AcceptorID = 123
	contract.Completed = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	candidates = []eve.DeliveryContract{contract}
	if _, err = s.Pool.Exec(ctx, `UPDATE welfare_contract_settlement_batches SET next_run_at=now() WHERE id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessSettlementBatch(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessSettlementBatch(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.SettlementBatch(ctx, adminID, b.ID)
	if err != nil || got.Batch.State != "completed" || got.Batch.CompletedCount != 2 || got.Batch.ContractID == nil || *got.Batch.ContractID != 900 {
		t.Fatal(got, err)
	}
	for _, it := range selection {
		row, err := store.Read(ctx, s.Pool, it.ID)
		if err != nil || row.State != "completed" {
			t.Fatal(row, err)
		}
	}
	var claims int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM eve_delivery_claims WHERE contract_id=900`).Scan(&claims); err != nil || claims != 1 {
		t.Fatal(claims, err)
	}
	// A delayed progress write cannot reopen a completed batch or erase evidence.
	if err = store.RecordSettlementProgress(ctx, s.Pool, b.ID, nil, nil, store.SettlementWaitingContractError); err != nil {
		t.Fatal(err)
	}
	got, err = s.SettlementBatch(ctx, adminID, b.ID)
	if err != nil || got.Batch.State != "completed" || got.Batch.ContractID == nil || *got.Batch.ContractID != 900 {
		t.Fatal(got, err)
	}
	db := stdlib.OpenDB(*s.Pool.Config().ConnConfig)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 94); err == nil {
		t.Fatal("rollback discarded active aliases")
	}
}

func TestSettlementProgressStatuses(t *testing.T) {
	for _, tc := range []struct{ message, status string }{
		{store.SettlementWaitingContractError, "waiting_contract"},
		{store.SettlementAwaitingAcceptanceError, "awaiting_acceptance"},
		{store.SettlementDeletedError, "contract_deleted"},
		{store.SettlementCancelledError, "contract_cancelled"},
		{store.SettlementRejectedError, "contract_rejected"},
		{store.SettlementFailedError, "contract_failed"},
		{store.SettlementMismatchError, "contract_mismatch"},
		{store.SettlementMultipleError, "multiple_contracts"},
	} {
		entries := []store.SettlementItem{{LastError: tc.message}, {LastError: tc.message}}
		if got := store.SettlementDeliveryStatus(2, entries); got != tc.status {
			t.Fatal(got, tc.status)
		}
		entries[1].LastError = ""
		if got := store.SettlementDeliveryStatus(2, entries); got != "" {
			t.Fatal("mixed evidence", got)
		}
	}
}
