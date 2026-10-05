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
	snapshot eve.StructureSnapshot
}

func (f structureTestData) ReadStructureSnapshots(context.Context) ([]eve.StructureSnapshot, error) {
	return []eve.StructureSnapshot{f.snapshot}, nil
}

type structureTestNames struct{}

func (structureTestNames) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return map[int64]eve.StaticTypeName{4051: {ID: 4051, Name: "氮燃料块"}}, nil
}

func (structureTestNames) SolarSystemNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return map[int64]eve.StaticTypeName{}, nil
}

func TestStructuresAdministratorUsesAuthorizedSourceCharacter(t *testing.T) {
	source := eve.StructureSource{CharacterID: 2122015910, Generation: 7, CorporationID: 98530802, CorporationName: "Glory Navy", CEOID: 2122015910, OwnerHash: []byte{1, 2, 3}}
	rows := []eve.Structure{{CorporationID: source.CorporationID, Kind: "upwell", ID: 9001}}
	h := structuresHandler(structureTestAccounts{}, structureTestAccess{admin: true}, structureTestData{snapshot: eve.StructureSnapshot{Source: source, Rows: rows}}, nil)
	got, err := h.Read(context.Background(), "site-admin", 0)
	if err != nil || len(got) != 1 || got[0].ID != 9001 {
		t.Fatalf("administrator source not used: got=%v err=%v", got, err)
	}
}

func TestStructuresMemberRequiresSourceBinding(t *testing.T) {
	source := eve.StructureSource{CharacterID: 2122015910, Generation: 7, CorporationID: 98530802, CorporationName: "Glory Navy", CEOID: 2122015910, OwnerHash: []byte{1, 2, 3}}
	h := structuresHandler(structureTestAccounts{}, structureTestAccess{}, structureTestData{snapshot: eve.StructureSnapshot{Source: source, Rows: []eve.Structure{{ID: 9001}}}}, nil)
	got, err := h.Read(context.Background(), "member", 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("unbound member received structures: got=%v err=%v", got, err)
	}
}

func TestStructuresResolvesPOSFuelNames(t *testing.T) {
	source := eve.StructureSource{CharacterID: 2122015910, Generation: 7, CorporationID: 98530802, CorporationName: "Glory Navy", CEOID: 2122015910, OwnerHash: []byte{1, 2, 3}}
	rows := []eve.Structure{{CorporationID: source.CorporationID, Kind: "pos", ID: 9001, Fuel: []eve.StructureFuel{{TypeID: 4051, Quantity: 42}}}}
	h := structuresHandler(structureTestAccounts{}, structureTestAccess{admin: true}, structureTestData{snapshot: eve.StructureSnapshot{Source: source, Rows: rows}}, structureTestNames{})
	got, err := h.Read(context.Background(), "site-admin", 0)
	if err != nil || len(got) != 1 || len(got[0].Fuel) != 1 || got[0].Fuel[0].Name != "氮燃料块" {
		t.Fatalf("POS fuel name not resolved: got=%v err=%v", got, err)
	}
}
