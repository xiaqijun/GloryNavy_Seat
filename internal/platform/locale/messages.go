package locale

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed messages.en.json
var englishJSON []byte
var englishMessages = func() map[string]string {
	var v map[string]string
	if err := json.Unmarshal(englishJSON, &v); err != nil {
		panic(err)
	}
	return v
}()

// Message is for server-owned messages only, never user-authored text.
// Unknown messages keep their source text instead of guessing a translation.
func Message(ctx context.Context, message string) string {
	if !English(ctx) {
		return message
	}
	if translated, ok := englishMessages[message]; ok {
		return translated
	}
	for _, prefix := range []string{"无法识别物品行：", "未识别物品：", "无法识别装填弹药：", "静态数据缺少物品 #"} {
		if value, ok := strings.CutPrefix(message, prefix); ok {
			return englishMessages[prefix] + value
		}
	}
	if value, ok := strings.CutPrefix(message, "请在 EFT 货舱中明确填写弹药数量："); ok {
		return "Specify the ammunition quantity in the EFT cargo section: " + strings.TrimSuffix(value, " x数量") + " xQuantity"
	}
	return message
}
