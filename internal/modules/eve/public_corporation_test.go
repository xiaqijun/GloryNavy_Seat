package eve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type publicESIStub struct {
	calls        []ESIRequest
	fail         bool
	allianceFail bool
	missingCount bool
}

func (s *publicESIStub) Request(_ context.Context, r ESIRequest, out any) (ESIResponse, error) {
	s.calls = append(s.calls, r)
	if s.fail {
		return ESIResponse{}, errors.New("private upstream detail")
	}
	body := `{"name":"Glory Navy","ticker":"G.N.V","member_count":379,"date_founded":"2017-09-27T11:40:29Z","alliance_id":99003581,"tax_rate":0.125,"ceo_id":123,"description":"private unwanted projection"}`
	if s.missingCount {
		body = `{"name":"Glory Navy","date_founded":"2017-09-27T11:40:29Z"}`
	}
	if strings.HasPrefix(r.Path, "/alliances/") {
		if s.allianceFail {
			return ESIResponse{}, errors.New("failed")
		}
		body = `{"name":"Fraternity.","ticker":"FRT"}`
		if strings.HasSuffix(r.Path, "/corporations/") {
			body = `[98530802,2,3]`
		}
	}
	return ESIResponse{ExpiresAt: time.Now().Add(time.Hour), ValidatedAt: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)}, json.Unmarshal([]byte(body), out)
}

func TestPublicCorporationProjectionAndCache(t *testing.T) {
	stub := &publicESIStub{}
	service := &PublicCorporationService{esi: stub}
	for range 2 {
		response := httptest.NewRecorder()
		service.ServeHTTP(response, httptest.NewRequest("GET", "/public/corporation?character_id=999&corporation_id=1", nil))
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		body := response.Body.String()
		for _, secret := range []string{"tax_rate", "ceo_id", "description", "private unwanted"} {
			if strings.Contains(body, secret) {
				t.Fatal("unexpected projection", secret)
			}
		}
		if !strings.Contains(body, `"member_count":379`) || !strings.Contains(body, `"corporation_count":3`) {
			t.Fatal(body)
		}
	}
	if len(stub.calls) != 3 {
		t.Fatal("cache was not reused", len(stub.calls))
	}
	for _, r := range stub.calls {
		if r.CharacterID != 0 || r.Generation != 0 || len(r.Scopes) != 0 || len(r.Body) != 0 || r.Method != "GET" {
			t.Fatal("public request used credential context", r)
		}
	}
	if stub.calls[0].Path != "/corporations/98530802/" {
		t.Fatal("visitor changed fixed corporation")
	}
}

func TestPublicCorporationFailurePreservesSnapshotAndBacksOff(t *testing.T) {
	stub := &publicESIStub{}
	service := &PublicCorporationService{esi: stub}
	original, err := service.load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	service.next = time.Time{}
	stub.fail = true
	stale, err := service.load(context.Background())
	if err != nil || !stale.Stale || stale.MemberCount != original.MemberCount || !stale.UpdatedAt.Equal(original.UpdatedAt) {
		t.Fatal("lost previous observation", stale, err)
	}
	_, _ = service.load(context.Background())
	if len(stub.calls) != 4 {
		t.Fatal("failed refresh did not back off", len(stub.calls))
	}
}

func TestPublicCorporationMissingDataIsNotZero(t *testing.T) {
	for _, stub := range []*publicESIStub{{fail: true}, {missingCount: true}} {
		service := &PublicCorporationService{esi: stub}
		response := httptest.NewRecorder()
		service.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
		if response.Code != 503 || strings.Contains(response.Body.String(), "private upstream") {
			t.Fatal(response.Body.String())
		}
	}
	service := &PublicCorporationService{esi: &publicESIStub{allianceFail: true}}
	data, err := service.load(context.Background())
	if err != nil || data.MemberCount != 379 || data.Alliance != nil {
		t.Fatal(data, err)
	}
}
