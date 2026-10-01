package skills

import (
	"context"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/platform/locale"
	"testing"
)

type emptyNames struct{}

func (emptyNames) TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error) {
	return map[int64]eve.StaticTypeName{}, nil
}
func TestLocalizedCatalogDoesNotMutateSharedReference(t *testing.T) {
	s := Service{Names: emptyNames{}}
	en, err := s.Types(locale.With(context.Background(), "en"))
	if err != nil {
		t.Fatal(err)
	}
	zh, err := s.Types(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range en {
		if v.Name != v.English || v.GroupEnglish == "" || v.Group != v.GroupEnglish {
			t.Fatalf("missing English reference: %+v", v)
		}
	}
	for _, v := range zh {
		for _, original := range catalog {
			if original.ID == v.ID && original.Name != v.Name {
				t.Fatal("shared catalog mutated")
			}
		}
	}
	snap, err := s.nameSnapshot(locale.With(context.Background(), "en"), Snapshot{Skills: []Skill{{ID: 3300}, {ID: 999999999}}, Queue: []QueueItem{{ID: 3300}}})
	if err != nil || snap.Skills[0].Name != catalog[3300].English || snap.Skills[0].Group != catalog[3300].GroupEnglish || snap.Skills[1].Name != "Skill #999999999" || snap.Skills[1].Group != "Other" {
		t.Fatal(snap, err)
	}
}
