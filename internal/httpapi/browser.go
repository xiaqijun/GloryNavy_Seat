package httpapi

import (
	"crypto/sha256"
	"net/http"
)

const SessionCookie = "gn_session"
const FlowCookie = "gn_eve_flow"

func CookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil || len(c.Value) > 256 {
		return ""
	}
	return c.Value
}

func SetCookie(w http.ResponseWriter, name, value string, seconds int, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: seconds, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func Hash(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }

// PUBLIC_ORIGIN is authoritative; never infer it from Host or forwarded headers.
func SameOrigin(r *http.Request, origin string) bool {
	return r.Header.Get("Origin") == origin && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
