package esiclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestFleetMutationWhitelistRejectsUnsafeRequests(t *testing.T) {
	c := New(nil, nil)
	mutation := context.WithValue(context.Background(), MutationKey{}, true)
	for _, tc := range []struct {
		ctx                 context.Context
		method, path, token string
	}{
		{context.Background(), "PUT", "/fleets/123/", "token"},
		{mutation, "PUT", "/fleets/123/members/", "token"},
		{mutation, "PUT", "/fleets/123/", ""},
		{mutation, "GET", "/fleets/123/", "token"},
	} {
		_, err := c.Request(tc.ctx, http.DefaultClient, tc.method, tc.path, tc.token, nil, nil)
		var fault Fault
		if !errors.As(err, &fault) || fault.Reason != "invalid_request" {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
	}
}
