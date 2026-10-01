package esiclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type interruptedResponse struct {
	data string
	err  error
}

func (r *interruptedResponse) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func TestReadResponseBodyDistinguishesTruncationFromSize(t *testing.T) {
	for _, tt := range []struct {
		name      string
		body      io.Reader
		reason    string
		temporary bool
	}{
		{"complete", strings.NewReader(`[]`), "", false},
		{"at limit", strings.NewReader(strings.Repeat(" ", 1<<20)), "", false},
		{"oversized", strings.NewReader(strings.Repeat(" ", (1<<20)+1)), "response_too_large", false},
		{"truncated", &interruptedResponse{`[{`, io.ErrUnexpectedEOF}, "network_error", true},
		{"timeout after valid JSON", &interruptedResponse{`[]`, context.DeadlineExceeded}, "network_error", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := readResponseBody(tt.body, 200)
			if tt.reason == "" {
				if err != nil || len(data) == 0 {
					t.Fatal(err)
				}
				return
			}
			var fault Fault
			if !errors.As(err, &fault) || fault.Reason != tt.reason || fault.Temporary != tt.temporary || data != nil {
				t.Fatalf("data=%d, fault=%+v", len(data), err)
			}
		})
	}
}
