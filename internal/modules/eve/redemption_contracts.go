package eve

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"time"
)

var ErrDeliveryClaimed = errors.New("contract already assigned to another delivery")

// ClaimDeliveryTx is shared by delivery consumers through host injection.
// Claims are retained even if a consumer cancels or reverses its own record.
func ClaimDeliveryTx(ctx context.Context, tx pgx.Tx, contract int64, module string, reference int64) error {
	ok, err := store.ClaimDelivery(ctx, tx, contract, module, reference)
	if err != nil {
		return err
	}
	if !ok {
		return ErrDeliveryClaimed
	}
	return nil
}

// RedemptionContracts reads only the recipient's authorised personal contract cache.
// Matching the complete description avoids substring collisions and truncated scans.
func (h *ContractHTTP) RedemptionContracts(ctx context.Context, tx pgx.Tx, actor string, recipient int64, reference string, since time.Time) ([]DeliveryContract, error) {
	if _, err := h.LookupOwner(ctx, actor, "character", recipient); err != nil {
		return nil, err
	}
	var db store.DBTX = h.pool
	if tx != nil {
		db = tx
	}
	ids, err := store.RedemptionContractIDs(ctx, db, recipient, reference, since)
	if err != nil {
		return nil, err
	}
	out := []DeliveryContract{}
	for _, id := range ids {
		rows, err := store.DeliveryContracts(ctx, db, "character", recipient, 0, 0, id, time.Time{}, tx != nil)
		if err != nil {
			return nil, err
		}
		if len(rows) != 1 {
			return nil, pgx.ErrNoRows
		}
		c, err := decodeDelivery(rows[0])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
