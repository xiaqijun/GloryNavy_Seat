package app

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
)

func publicActivity(reader *eve.AuthorizationService, accounts *identity.Service) *eve.PublicActivityService {
	s := eve.NewPublicActivity()
	s.Online = func(ctx context.Context) (*eve.PublicOnline, error) {
		ids, err := reader.ActivityCharacters(ctx, 98530802)
		if err != nil {
			return nil, err
		}
		bindings, err := accounts.Bindings(ctx, nil, ids)
		if err != nil {
			return nil, err
		}
		rows := make([]eve.PublicOnlineBinding, 0, len(bindings))
		for _, b := range bindings {
			rows = append(rows, eve.PublicOnlineBinding{ID: b.ID, OwnerHash: b.OwnerHash})
		}
		return reader.PublicOnline(ctx, rows)
	}
	return s
}
