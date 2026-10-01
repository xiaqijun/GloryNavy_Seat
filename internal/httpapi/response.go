package httpapi

import (
	"encoding/json"
	"glorynavy.local/seat/internal/platform/locale"
	"net/http"
)

type requestKey struct{}
type envelope struct {
	Data      any       `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	RequestID string    `json:"request_id"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func RequestID(r *http.Request) string { id, _ := r.Context().Value(requestKey{}).(string); return id }
func Respond(w http.ResponseWriter, r *http.Request, status int, data any) {
	write(w, status, envelope{Data: data, RequestID: RequestID(r)})
}
func Failure(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	write(w, status, envelope{Error: &apiError{Code: code, Message: locale.Message(r.Context(), message)}, RequestID: RequestID(r)})
}
func write(w http.ResponseWriter, status int, data envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
