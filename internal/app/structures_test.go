package app

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/identity"
)

type structureTestAccounts struct{ bindings []identity.Binding }

func (f structureTestAccounts) Bindings(context.Context, pgx.Tx, []int64) ([]identity.Binding, error) {
	return f.bindings, nil
}

type structureTestAccess struct{ admin bool }

func (f structureTestAccess) IsAdministrator(context.Context, string) (bool, error) {
	return f.admin, nil
}
func (f structureTestAccess) Can(context.Context, string, string, access.Corporation) (bool, error) {
	return true, nil
}

type structureTestData struct {
	source eve.StructureSource
	rows   []eve.Structure
}

func (f structureTestData) StructureSources(context.Context) ([]eve.StructureSource, error) {
	return []eve.StructureSource{f.source}, nil
}
func (f structureTestData) StructureSourceValid(context.Context, eve.StructureSource) (bool, error) {
	return true, nil
}
func (f structureTestData) ReadCorporationStructures(context.Context, int64, int64) ([]eve.Structure, error) {
	return f.rows, nil
}

func TestStructuresAdministratorUsesAuthorizedSourceCharacter(t *testing.T) {
	source := eve.StructureSource{CharacterID: 2122015910, Generation: 7, CorporationID: 98530802, CorporationName: "Glory Navy", CEOID: 2122015910, OwnerHash: []byte{1, 2, 3}}
	rows := []eve.Structure{{CorporationID: source.CorporationID, Kind: "upwell", ID: 9001}}
	h := structuresHandler(structureTestAccounts{}, structureTestAccess{admin: true}, structureTestData{source: source, rows: rows}, nil)
	got, err := h.Read(context.Background(), "site-admin", 0)
	if err != nil || len(got) != 1 || got[0].ID != 9001 {
		t.Fatalf("administrator source not used: got=%v err=%v", got, err)
	}
}

func TestStructuresMemberRequiresSourceBinding(t *testing.T) {
	source := eve.StructureSource{CharacterID: 2122015910, Generation: 7, CorporationID: 98530802, CorporationName: "Glory Navy", CEOID: 2122015910, OwnerHash: []byte{1, 2, 3}}
	h := structuresHandler(structureTestAccounts{}, structureTestAccess{}, structureTestData{source: source, rows: []eve.Structure{{ID: 9001}}}, nil)
	got, err := h.Read(context.Background(), "member", 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("unbound member received structures: got=%v err=%v", got, err)
	}
}
