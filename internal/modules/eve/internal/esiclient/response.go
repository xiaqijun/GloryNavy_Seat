package esiclient

import "io"

// Read failures are transient transport failures, not evidence of oversized
// content. Never cache or publish an incomplete response, even if it parses.
func readResponseBody(body io.Reader, status, limit int) ([]byte, error) {
	if limit <= 0 {
		limit = 1 << 20
	}
	data, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if len(data) > limit {
		return nil, Fault{"response_too_large", status, false}
	}
	if err != nil {
		return nil, Fault{"network_error", status, true}
	}
	return data, nil
}
