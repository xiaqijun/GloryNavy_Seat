package welfare

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/platform/locale"
	"strings"
	"testing"
)

type presentationNames struct{ calls int }

func (n *presentationNames) TypeNames(ctx context.Context, ids []int64) (map[int64]eve.StaticTypeName, error) {
	n.calls++
	return map[int64]eve.StaticTypeName{34: {Name: locale.Choose(ctx, "三钛合金", "Tritanium")}, 587: {Name: locale.Choose(ctx, "裂谷级", "Rifter")}}, nil
}
func (n *presentationNames) SolarSystemNames(ctx context.Context, ids []int64) (map[int64]eve.StaticTypeName, error) {
	n.calls++
	return map[int64]eve.StaticTypeName{30000142: {Name: locale.Choose(ctx, "吉他", "Jita")}}, nil
}
func TestPresentationPreservesEvidenceAndBusinessFields(t *testing.T) {
	original := json.RawMessage(`{"future":{"number":9007199254740993},"description":"用户原文","loss_evidence":{"ship_type_id":"587","ship_name":"裂谷级","solar_system_id":"30000142","solar_system_name":"吉他","items":[{"type_id":"34","name":"三钛合金","quantity":"9007199254740993"}]},"valuation":{"reason":"多船合同需人工拆分核价","amount_minor":9007199254740993,"market":{"lines":[{"type_id":"34","name":"三钛合金","input":"三钛合金","mid":"12.3400"}]}},"delivery":{"contract":{"content_token":"immutable","title":"用户合同描述","items":[{"type_id":"34","name":"三钛合金"}]}}}`)
	names := &presentationNames{}
	s := Service{Names: names}
	cases := []Case{{ID: 1, Version: 2, Detail: original}, {ID: 2, Version: 3, Detail: original}}
	out := s.presentCases(locale.With(context.Background(), "en"), cases)
	if names.calls != 2 {
		t.Fatalf("expected batched local reads, got %d", names.calls)
	}
	if string(cases[0].Detail) != string(original) || string(cases[1].Detail) != string(original) {
		t.Fatal("stored input mutated")
	}
	text := string(out[0].Detail)
	for _, want := range []string{"Tritanium", "Rifter", "Jita", "9007199254740993", "12.3400", "用户原文", "用户合同描述", "immutable", "manual valuation", `"input":"三钛合金"`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing preserved/localized field: %s", want)
		}
	}
	if out[0].Version != 2 || out[1].ID != 2 {
		t.Fatal("business identity changed")
	}
	back := s.presentCases(context.Background(), out)
	if !strings.Contains(string(back[0].Detail), "三钛合金") {
		t.Fatal("names did not switch back")
	}
}

func TestDetailEnrichesKillmailWithoutChangingFrozenEvidence(t *testing.T) {
	original := json.RawMessage(`{"character_id":"123","killmail_id":"456","loss_evidence":{"id":"456","ship_name":"裂谷级","items":[{"type_id":"34","quantity":2}],"unknown":{"keep":true}},"valuation":{"amount_minor":12345},"unknown_root":9007199254740993}`)
	s := Service{LatestLoss: func(_ context.Context, account string, char, id int64) (*Loss, error) {
		if account != "member" || char != 123 || id != 456 {
			t.Fatal("wrong identity")
		}
		return &Loss{ID: 456, CharacterID: 123, DamageTaken: 5000, Attackers: []eve.KillAttacker{{CharacterID: 9001, Name: "Attacker", DamageDone: 5000, FinalBlow: true}}}, nil
	}}
	c := Case{AccountID: "member", Detail: original}
	out := s.enrichCaseLoss(context.Background(), c)
	if string(c.Detail) != string(original) {
		t.Fatal("stored evidence mutated")
	}
	for _, want := range []string{`"damage_taken":5000`, `"name":"Attacker"`, `"amount_minor":12345`, `"keep":true`, `9007199254740993`} {
		if !strings.Contains(string(out.Detail), want) {
			t.Errorf("missing %s", want)
		}
	}
}
