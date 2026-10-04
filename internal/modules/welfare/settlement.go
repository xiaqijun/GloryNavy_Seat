package welfare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
	"strings"
)

var ErrSettlement = errors.New("contract settlement unavailable")
var ErrSettlementGroup = errors.New("batch settlement requires one account group")

type SettlementInput struct {
	RequestKey string                `json:"request_key"`
	Note       string                `json:"note"`
	Items      []SettlementInputItem `json:"items"`
}

type SettlementInputItem struct {
	Source string `json:"source"`
	ID     int64  `json:"id,string"`
}

type SettlementBatchView struct {
	Batch store.SettlementBatch  `json:"batch"`
	Items []store.SettlementItem `json:"items"`
}

// settlementBatchForRead keeps legacy aggregate batches from exposing the
// old multi-character recipient snapshot. The stored snapshot remains
// untouched for audit purposes; the current main character is the only
// recipient shown and used for a merged contract.
func (s *Service) settlementBatchForRead(ctx context.Context, b store.SettlementBatch) (store.SettlementBatch, error) {
	if b.SettlementReference == "" || b.AccountID == "" {
		return b, nil
	}
	if s.MainCharacterID != nil {
		recipients, err := s.settlementRecipients(ctx, b)
		if err != nil && !errors.Is(err, ErrSettlementUnsupported) {
			return b, err
		}
		if err == nil {
			b.RecipientIDs, err = json.Marshal(recipients)
			if err != nil {
				return b, err
			}
		}
	}
	if s.MainCharacterName != nil {
		name, err := s.MainCharacterName(ctx, b.AccountID)
		if err != nil {
			return b, err
		}
		if strings.TrimSpace(name) != "" {
			b.RecipientNames, err = json.Marshal([]string{name})
			if err != nil {
				return b, err
			}
		}
	}
	return b, nil
}

func (s *Service) CreateSettlementBatch(ctx context.Context, actor string, input SettlementInput) (SettlementBatchView, error) {
	var out SettlementBatchView
	if e := s.admin(ctx, actor); e != nil {
		return out, e
	}
	if !validUUID(actor) || !validUUID(input.RequestKey) || len(input.Items) == 0 || len(input.Items) > 100 || len([]rune(input.Note)) > 500 || strings.ContainsAny(input.Note, "\r\n") {
		return out, ErrInvalid
	}
	selection := make([]store.SettlementSelection, 0, len(input.Items))
	seen := map[string]bool{}
	accountGroup := ""
	for _, item := range input.Items {
		if (item.Source != "welfare" && item.Source != "exchange") || item.ID <= 0 {
			return out, ErrInvalid
		}
		key := fmt.Sprintf("%s:%d", item.Source, item.ID)
		if seen[key] {
			return out, ErrInvalid
		}
		seen[key] = true
		if item.Source == "exchange" && s.ExchangeDelivery == nil {
			return out, ErrSettlement
		}
		var account string
		if item.Source == "welfare" {
			caseRow, checkErr := store.Read(ctx, s.Pool, item.ID)
			if checkErr != nil {
				return out, checkErr
			}
			if caseRow.State != "approved" && caseRow.State != "executing" && caseRow.State != "cancel_requested" {
				return out, ErrConflict
			}
			account = caseRow.AccountID
		} else if s.ExchangeSettlementEligible != nil {
			if checkErr := s.ExchangeSettlementEligible(ctx, item.ID); checkErr != nil {
				return out, checkErr
			}
			if s.ExchangeSettlementAccount == nil {
				return out, ErrSettlement
			}
			var checkErr error
			account, checkErr = s.ExchangeSettlementAccount(ctx, item.ID)
			if checkErr != nil {
				return out, checkErr
			}
		}
		if account == "" {
			return out, ErrSettlement
		}
		if accountGroup == "" {
			accountGroup = account
		} else if accountGroup != account {
			return out, ErrSettlementGroup
		}
		selection = append(selection, store.SettlementSelection{Source: item.Source, SourceID: item.ID})
	}
	// Freeze one aggregate contract projection at batch creation. This makes the
	// copied contract deterministic even if a reward catalog or case later
	// changes, and gives the administrator one reference to put in-game.
	summaryAccount, summary, e := s.settlementSummary(ctx, input.Items)
	if e != nil {
		return out, e
	}
	if summaryAccount != accountGroup {
		return out, ErrSettlementGroup
	}
	itemsJSON, e := json.Marshal(summary.Items)
	if e != nil {
		return out, e
	}
	recipientsJSON, e := json.Marshal(summary.RecipientIDs)
	if e != nil {
		return out, e
	}
	// Replays return the original batch and never enqueue a second settlement.
	if existing, e := store.SettlementBatchByRequestKey(ctx, s.Pool, actor, input.RequestKey); e == nil {
		existing, e = s.settlementBatchForRead(ctx, existing)
		if e != nil {
			return out, e
		}
		items, itemErr := store.SettlementItems(ctx, s.Pool, existing.ID)
		return SettlementBatchView{Batch: existing, Items: items}, itemErr
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.Background())
	reference, e := settlementReference(ctx, tx)
	if e != nil {
		return out, e
	}
	b, e := store.CreateSettlementBatch(ctx, tx, actor, input.RequestKey, input.Note, summaryAccount, reference, summary.ISKMinor, itemsJSON, recipientsJSON, selection)
	if e != nil {
		var constraint *pgconn.PgError
		if errors.As(e, &constraint) && constraint.Code == "23505" {
			return out, ErrConflict
		}
		return out, e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, e
	}
	b, e = s.settlementBatchForRead(ctx, b)
	if e != nil {
		return out, e
	}
	items, e := store.SettlementItems(ctx, s.Pool, b.ID)
	return SettlementBatchView{Batch: b, Items: items}, e
}

func (s *Service) SettlementBatches(ctx context.Context, actor string) ([]store.SettlementBatch, error) {
	if e := s.admin(ctx, actor); e != nil {
		return nil, e
	}
	return store.ListSettlementBatches(ctx, s.Pool, 100)
}

func (s *Service) SettlementBatch(ctx context.Context, actor string, id int64) (SettlementBatchView, error) {
	if e := s.admin(ctx, actor); e != nil {
		return SettlementBatchView{}, e
	}
	b, e := store.SettlementBatchByID(ctx, s.Pool, id)
	if e != nil {
		return SettlementBatchView{}, e
	}
	b, e = s.settlementBatchForRead(ctx, b)
	if e != nil {
		return SettlementBatchView{}, e
	}
	items, e := store.SettlementItems(ctx, s.Pool, id)
	if e == nil {
		b.DeliveryStatus = store.SettlementDeliveryStatus(b.TotalCount, items)
	}
	return SettlementBatchView{Batch: b, Items: items}, e
}

func (s *Service) RetrySettlementBatch(ctx context.Context, actor string, id int64) error {
	if e := s.admin(ctx, actor); e != nil {
		return e
	}
	return store.RetrySettlementBatch(ctx, s.Pool, id)
}

// ProcessSettlementBatch is deliberately item-by-item. A failed contract read
// cannot mark other contracts as delivered, and a later retry only revisits
// failed items that have not exhausted their attempts.
func (s *Service) ProcessSettlementBatch(ctx context.Context, id int64) error {
	b, e := store.SettlementBatchByID(ctx, s.Pool, id)
	if e != nil {
		return e
	}
	if b.State == "completed" || b.State == "failed" {
		return nil
	}
	claimed, e := store.ClaimSettlementBatch(ctx, s.Pool, id)
	if e != nil || !claimed {
		return e
	}
	items, e := store.SettlementItems(ctx, s.Pool, id)
	if e != nil {
		return e
	}
	if handled, aggregateErr := s.processAggregateSettlement(ctx, b, items); handled {
		return aggregateErr
	}
	for _, candidate := range items {
		if candidate.State == "completed" || (candidate.State == "failed" && candidate.Attempts >= 3) {
			continue
		}
		item, ok, e := store.ClaimSettlementItem(ctx, s.Pool, candidate.ID)
		if e != nil {
			return e
		}
		if !ok {
			continue
		}
		var checkErr error
		complete := false
		switch item.Source {
		case "welfare":
			checkErr = s.CheckDelivery(ctx, item.SourceID)
			if checkErr == nil {
				var row Case
				row, checkErr = store.Read(ctx, s.Pool, item.SourceID)
				complete = checkErr == nil && row.State == "completed"
			}
		case "exchange":
			if s.ExchangeDelivery == nil || s.ExchangeSettlementComplete == nil {
				checkErr = ErrSettlement
			} else {
				checkErr = s.ExchangeDelivery(ctx, item.SourceID)
				if checkErr == nil {
					complete, checkErr = s.ExchangeSettlementComplete(ctx, item.SourceID)
				}
			}
		}
		if checkErr != nil {
			if e = store.MarkSettlementItem(ctx, s.Pool, item.ID, "failed", checkErr.Error()); e != nil {
				return e
			}
		} else if complete {
			if e = store.MarkSettlementItem(ctx, s.Pool, item.ID, "completed", ""); e != nil {
				return e
			}
		} else if e = store.MarkSettlementItem(ctx, s.Pool, item.ID, "pending", "合同尚未完成，等待下一轮核验"); e != nil {
			return e
		}
	}
	_, e = store.RefreshSettlementBatch(ctx, s.Pool, id)
	return e
}
