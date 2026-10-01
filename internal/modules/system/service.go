package system

import (
	"context"
	"errors"

	"glorynavy.local/seat/internal/modules/system/internal/store"
)

type Reader interface {
	GetPlatformMetadata(context.Context) (store.GetPlatformMetadataRow, error)
}

type Status struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Environment   string `json:"environment"`
	Database      string `json:"database"`
	SchemaVersion int32  `json:"schema_version"`
}

type Service struct {
	Reader  Reader
	Version string
}

func (s Service) Status(ctx context.Context) (Status, error) {
	m, err := s.Reader.GetPlatformMetadata(ctx)
	if err != nil {
		return Status{}, err
	}
	if m.SchemaVersion != 1 || m.Environment != "tranquility" {
		return Status{}, errors.New("unsupported database schema or environment")
	}
	return Status{Name: "GloryNavy", Version: s.Version, Environment: m.Environment, Database: "ready", SchemaVersion: m.SchemaVersion}, nil
}
