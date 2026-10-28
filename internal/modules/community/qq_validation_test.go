package community

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOfficialQQValidationCannotSignEvents(t *testing.T) {
	s := New(nil)
	s.SetQQBotOfficial("test-app", "test-secret", defaultQQBotAPIBase)
	for _, tc := range []struct{ token, timestamp string }{
		{`{"op":0,"t":"GROUP_MESSAGE_CREATE","id":"forged"}`, "1725442341"},
		{" \n{\"op\":0}", "1725442341"},
		{"valid-token", "1725442341{\"op\":0}"},
	} {
		body, err := json.Marshal(map[string]any{"op": 13, "d": map[string]string{"plain_token": tc.token, "event_ts": tc.timestamp}})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		Handler{Service: s}.officialQQWebhook(w, httptest.NewRequest("POST", "/", strings.NewReader(string(body))))
		if w.Code != 400 {
			t.Fatalf("validation signed event material: status=%d", w.Code)
		}
	}
}
