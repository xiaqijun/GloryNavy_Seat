// Package locale selects presentation language only; it never changes business identifiers.
package locale

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

type key struct{}

// Parse supports the site's two languages, including region variants and q weights.
// Unsupported/malformed input and background jobs keep the Chinese default.
func Parse(header string) string {
	best, quality := "zh-CN", -1.0
	for _, entry := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ";")
		tag, q := strings.ToLower(strings.TrimSpace(parts[0])), 1.0
		for _, p := range parts[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if !ok || k != "q" {
				q = 0
				break
			}
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || n < 0 || n > 1 {
				q = 0
				break
			}
			q = n
		}
		language := ""
		if tag == "en" || strings.HasPrefix(tag, "en-") {
			language = "en"
		}
		if tag == "zh" || strings.HasPrefix(tag, "zh-") {
			language = "zh-CN"
		}
		if language != "" && q > 0 && q > quality {
			best, quality = language, q
		}
	}
	return best
}

func With(ctx context.Context, language string) context.Context {
	return context.WithValue(ctx, key{}, Parse(language))
}
func Language(ctx context.Context) string {
	if v, _ := ctx.Value(key{}).(string); v == "en" {
		return "en"
	}
	return "zh-CN"
}
func English(ctx context.Context) bool { return Language(ctx) == "en" }
func Choose(ctx context.Context, chinese, english string) string {
	if English(ctx) {
		if english != "" {
			return english
		}
		return chinese
	}
	if chinese != "" {
		return chinese
	}
	return english
}
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := With(r.Context(), r.Header.Get("Accept-Language"))
		w.Header().Set("Content-Language", Language(ctx))
		w.Header().Add("Vary", "Accept-Language")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
