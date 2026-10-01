package locale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocaleNegotiation(t *testing.T) {
	for input, want := range map[string]string{"": "zh-CN", "en-US": "en", "zh-TW": "zh-CN", "fr, en-GB;q=0.7, zh-CN;q=0.8": "zh-CN", "en;q=0,zh;q=0": "zh-CN", "en;q=NaN": "zh-CN", "en;q=2": "zh-CN", "de": "zh-CN", "EN;q=0.9,zh;q=0.2": "en"} {
		if got := Parse(input); got != want {
			t.Errorf("%q: %s != %s", input, got, want)
		}
	}
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Language(r.Context()) != r.Header.Get("Accept-Language") {
			t.Error("request context language missing")
		}
		if r.Header.Get("X-CSRF-Token") != "original" {
			t.Error("request headers changed")
		}
	}))
	for _, lang := range []string{"en", "zh-CN", "en"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Language", lang)
		r.Header.Set("X-CSRF-Token", "original")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Content-Language") != lang || w.Header().Get("Vary") != "Accept-Language" {
			t.Fatal(w.Header())
		}
	}
	if Language(context.Background()) != "zh-CN" {
		t.Fatal("request locale leaked into background")
	}
}

func TestMessagesKeepUnknownAndDynamicPayload(t *testing.T) {
	ctx := With(context.Background(), "en")
	for input, want := range map[string]string{
		"未识别物品：我的装备 #9007199254740993":   "Unrecognized item: 我的装备 #9007199254740993",
		"静态数据缺少物品 #123":                  "Static data is missing item #123",
		"请在 EFT 货舱中明确填写弹药数量：Scourge x数量": "Specify the ammunition quantity in the EFT cargo section: Scourge xQuantity",
		"未来未知提示":                         "未来未知提示",
		"合同已关联补损单":                       "The contract is already linked to a reimbursement.",
	} {
		if got := Message(ctx, input); got != want {
			t.Errorf("%q: %q != %q", input, got, want)
		}
		if Message(context.Background(), input) != input {
			t.Error("Chinese default changed")
		}
	}
}
