package welfare

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/eve"
	"glorynavy.local/seat/internal/modules/welfare/internal/store"
)

func TestActivityProjectApplicationAndContractFulfillment(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.LibraryReward = func(_ context.Context, actor string, corp, id, version int64) (*GrowthRewards, error) {
		if actor != adminID || corp != 10 || id != 7 || version != 1 {
			return nil, ErrRule
		}
		return &GrowthRewards{ISKMinor: 10000, Items: []GrowthItem{{ID: 34, Quantity: 2, Name: "Tritanium"}}, Fittings: []GrowthFitting{}}, nil
	}
	input := ActivityProjectInput{CorporationID: 10, RequestKey: key(8800), Name: "Fleet contest", Enabled: true, RewardID: 7, RewardVersion: 1, CoinsMinor: 125}
	if _, err := s.SaveActivityProject(ctx, userID, input); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("member edited project: %v", err)
	}
	p, err := s.SaveActivityProject(ctx, adminID, input)
	if err != nil || !isActivity(p.Kind) {
		t.Fatalf("project: %+v %v", p, err)
	}
	replay, err := s.SaveActivityProject(ctx, adminID, input)
	if err != nil || replay.Kind != p.Kind {
		t.Fatalf("project replay: %+v %v", replay, err)
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y3cpxsAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	command := Command{Action: "apply", Kind: p.Kind, CorporationID: 10, RequestKey: key(8801), Detail: Detail{CharacterID: 123, Description: "Participated in fleet contest", Rewards: &GrowthRewards{Coins: 999999}}, Images: []ActivityImage{{MIME: "image/png", Content: png}}}
	out, err := s.Execute(ctx, userID, command)
	if err != nil {
		t.Fatal(err)
	}
	var application Case
	if err = json.Unmarshal(out, &application); err != nil {
		t.Fatal(err)
	}
	var detail Detail
	if err = json.Unmarshal(application.Detail, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Rewards == nil || detail.Rewards.Coins != 125 || detail.Rewards.ISKMinor != 10000 || detail.ImageCount != 1 {
		t.Fatalf("untrusted reward or missing image: %+v", detail)
	}
	if _, _, err = s.ActivityImage(ctx, otherID, application.ID, 1); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other member read image: %v", err)
	}
	mime, got, err := s.ActivityImage(ctx, userID, application.ID, 1)
	if err != nil || mime != "image/png" || string(got) != string(png) {
		t.Fatalf("image read: %s %v", mime, err)
	}
	repeated, err := s.Execute(ctx, userID, command)
	var replayed Case
	if err == nil {
		err = json.Unmarshal(repeated, &replayed)
	}
	if err != nil || replayed.ID != application.ID {
		t.Fatalf("apply replay: %v", err)
	}
	command.Images[0].Content = append([]byte(nil), png...)
	command.Images[0].Content = append(command.Images[0].Content, 0)
	if _, err = s.Execute(ctx, userID, command); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed image replay: %v", err)
	}
	command.Images[0].Content = png
	input.ProjectID, input.Version, input.RequestKey, input.CoinsMinor = activityProjectID(p.Kind), p.Version, key(8803), 250
	updated, err := s.SaveActivityProject(ctx, adminID, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Version, input.RequestKey, input.Enabled = updated.Version, key(8804), false
	if _, err = s.SaveActivityProject(ctx, adminID, input); err != nil {
		t.Fatal(err)
	}
	repeated, err = s.Execute(ctx, userID, command)
	if err == nil {
		err = json.Unmarshal(repeated, &replayed)
	}
	if err != nil || replayed.ID != application.ID {
		t.Fatalf("replay after project closure: %v", err)
	}
	command.RequestKey = key(8805)
	if _, err = s.Execute(ctx, userID, command); !errors.Is(err, ErrRule) {
		t.Fatalf("closed project accepted: %v", err)
	}

	approved, err := s.Execute(ctx, adminID, Command{Action: "approve", ID: application.ID, Version: application.Version, RequestKey: key(8802), Note: "Verified"})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(approved, &application); err != nil {
		t.Fatal(err)
	}
	if application.State != "approved" {
		t.Fatal(application.State)
	}
	var total int64
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger`).Scan(&total); err != nil || total != 0 {
		t.Fatalf("coins before contract: %d %v", total, err)
	}
	c := eve.DeliveryContract{ID: 991, Title: application.Reference, Type: "item_exchange", Status: "finished", IssuerID: 789, IssuerCorporationID: 10, AssigneeID: 123, AcceptorID: 123, Price: "0", Reward: "100", Issued: application.CreatedAt.Add(time.Second), Completed: application.CreatedAt.Add(2 * time.Second).Format(time.RFC3339), ItemsReady: true, ContentToken: "terms", Items: []eve.DeliveryItem{{TypeID: 34, Quantity: 2, Included: true}}}
	var approvedDetail Detail
	if err = json.Unmarshal(application.Detail, &approvedDetail); err != nil {
		t.Fatal(err)
	}
	wrong := c
	wrong.Reward = "99"
	if state := fulfillmentState(application, approvedDetail, wrong, s.MatchReward); state != "mismatch" {
		t.Fatalf("underpaid contract: %s", state)
	}
	s.PaymentContracts = func(context.Context, pgx.Tx, string, int64, string, time.Time) ([]eve.DeliveryContract, error) {
		return []eve.DeliveryContract{c}, nil
	}
	s.PaymentBindings = func(context.Context, pgx.Tx, []int64) (map[int64]string, error) {
		return map[int64]string{123: userID, 789: adminID}, nil
	}
	s.ClaimDelivery = eve.ClaimDeliveryTx
	if err = s.CheckDelivery(ctx, application.ID); err != nil {
		t.Fatal(err)
	}
	current, err := store.Read(ctx, s.Pool, application.ID)
	if err != nil || current.State != "completed" {
		t.Fatalf("contract: %s %v", current.State, err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger`).Scan(&total); err != nil || total != 125 {
		t.Fatalf("coins after contract: %d %v", total, err)
	}
	if err = s.CheckDelivery(ctx, application.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger`).Scan(&total); err != nil || total != 125 {
		t.Fatalf("duplicate coins: %d %v", total, err)
	}
}

func TestActivityCoinOnlyApprovalAndGenericConfigGuard(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.Execute(ctx, adminID, Command{Action: "configure", CorporationID: 10, Kind: "activity_1", RequestKey: key(8810), Config: Config{Enabled: true, Rewards: &GrowthRewards{Coins: 999}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("generic reward injection: %v", err)
	}
	p, err := s.SaveActivityProject(ctx, adminID, ActivityProjectInput{CorporationID: 10, RequestKey: key(8811), Name: "Coin contest", Enabled: true, CoinsMinor: 50})
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y3cpxsAAAAASUVORK5CYII=")
	out, err := s.Execute(ctx, userID, Command{Action: "apply", Kind: p.Kind, CorporationID: 10, RequestKey: key(8812), Detail: Detail{CharacterID: 123, Description: "Joined the event"}, Images: []ActivityImage{{MIME: "image/png", Content: png}}})
	if err != nil {
		t.Fatal(err)
	}
	var application Case
	if err = json.Unmarshal(out, &application); err != nil {
		t.Fatal(err)
	}
	out, err = s.Execute(ctx, adminID, Command{Action: "approve", ID: application.ID, Version: application.Version, RequestKey: key(8813), Note: "Verified"})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(out, &application); err != nil {
		t.Fatal(err)
	}
	if application.State != "completed" {
		t.Fatal(application.State)
	}
	var total int64
	if err = s.Pool.QueryRow(ctx, `SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger`).Scan(&total); err != nil || total != 50 {
		t.Fatalf("coin-only credit: %d %v", total, err)
	}
}

func TestActivityApplicationsCanTargetMultipleBoundCharacters(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.Characters = func(_ context.Context, id string) ([]Character, error) {
		return []Character{{ID: 123, Name: "主角色", AccountID: id, CorporationID: 10}, {ID: 456, Name: "子角色", AccountID: id, CorporationID: 10}}, nil
	}
	p, err := s.SaveActivityProject(ctx, adminID, ActivityProjectInput{CorporationID: 10, RequestKey: key(8820), Name: "Multi-role event", Enabled: true, CoinsMinor: 50})
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y3cpxsAAAAASUVORK5CYII=")
	first := Command{Action: "apply", Kind: p.Kind, CorporationID: 10, RequestKey: activityChildRequestKey(key(8821), 123), Detail: Detail{CharacterID: 123, Description: "Joined together"}, Images: []ActivityImage{{MIME: "image/png", Content: png}}}
	second := first
	second.RequestKey = activityChildRequestKey(key(8821), 456)
	second.Detail.CharacterID = 456
	firstOut, err := s.Execute(ctx, userID, first)
	if err != nil {
		t.Fatal(err)
	}
	secondOut, err := s.Execute(ctx, userID, second)
	if err != nil {
		t.Fatal(err)
	}
	var firstCase, secondCase Case
	if err = json.Unmarshal(firstOut, &firstCase); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(secondOut, &secondCase); err != nil {
		t.Fatal(err)
	}
	if firstCase.ID == secondCase.ID || firstCase.ID == 0 || secondCase.ID == 0 {
		t.Fatalf("expected independent cases: %d %d", firstCase.ID, secondCase.ID)
	}
	var firstDetail, secondDetail Detail
	if err = json.Unmarshal(firstCase.Detail, &firstDetail); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(secondCase.Detail, &secondDetail); err != nil {
		t.Fatal(err)
	}
	if firstDetail.CharacterID != 123 || secondDetail.CharacterID != 456 {
		t.Fatalf("wrong recipients: %d %d", firstDetail.CharacterID, secondDetail.CharacterID)
	}
	replay, err := s.Execute(ctx, userID, first)
	if err != nil {
		t.Fatal(err)
	}
	var replayCase Case
	if err = json.Unmarshal(replay, &replayCase); err != nil || replayCase.ID != firstCase.ID {
		t.Fatalf("batch child replay: %d %v", replayCase.ID, err)
	}
}

func TestActivityClaimLimitCountsOneMultiRoleBatch(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	s.Characters = func(_ context.Context, id string) ([]Character, error) {
		return []Character{{ID: 123, Name: "主角色", AccountID: id, CorporationID: 10}, {ID: 456, Name: "子角色", AccountID: id, CorporationID: 10}}, nil
	}
	limit := int64(1)
	p, err := s.SaveActivityProject(ctx, adminID, ActivityProjectInput{CorporationID: 10, RequestKey: key(8830), Name: "One claim", Enabled: true, CoinsMinor: 50, ClaimLimit: limit})
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y3cpxsAAAAASUVORK5CYII=")
	batch := key(8831)
	first := Command{Action: "apply", Kind: p.Kind, CorporationID: 10, RequestKey: activityChildRequestKey(batch, 123), Detail: Detail{CharacterID: 123, ActivityBatchKey: batch}, Images: []ActivityImage{{MIME: "image/png", Content: png}}}
	second := first
	second.RequestKey = activityChildRequestKey(batch, 456)
	second.Detail.CharacterID = 456
	if _, err = s.Execute(ctx, userID, first); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(ctx, userID, second); err != nil {
		t.Fatalf("same multi-role batch should count once: %v", err)
	}
	third := first
	third.RequestKey = activityChildRequestKey(key(8832), 123)
	third.Detail.ActivityBatchKey = key(8832)
	if _, err = s.Execute(ctx, userID, third); !errors.Is(err, ErrActivityClaimed) {
		t.Fatalf("claim limit not enforced: %v", err)
	}
	fourth := second
	fourth.RequestKey = activityChildRequestKey(key(8833), 456)
	fourth.Detail.ActivityBatchKey = key(8833)
	if _, err = s.Execute(ctx, userID, fourth); !errors.Is(err, ErrActivityClaimed) {
		t.Fatalf("claim limit should apply independently to character 456: %v", err)
	}
}

func TestActivityImageValidationAndMissingSnapshot(t *testing.T) {
	s := &Service{}
	for _, images := range [][]ActivityImage{nil, {{MIME: "text/html", Content: []byte("<script>alert(1)</script>")}}, {{MIME: "image/png", Content: []byte("<svg></svg>")}}} {
		if validateActivityImages(images) == nil {
			t.Fatal("invalid image accepted")
		}
	}
	if err := s.prepareFulfillment("activity_1", &Detail{}); !errors.Is(err, ErrRule) {
		t.Fatalf("missing frozen reward: %v", err)
	}
}
