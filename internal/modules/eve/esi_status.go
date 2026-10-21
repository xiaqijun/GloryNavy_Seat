package eve

import (
	"errors"
	"time"
)

// IsESINotFound distinguishes a confirmed public-resource absence from a
// timeout, a shared budget deferral or an upstream server error.
func IsESINotFound(err error) bool {
	var fault syncFault
	return errors.As(err, &fault) && fault.Status == 404
}

// ESIRetryTime exposes the shared gateway's requested delay without exposing
// credentials or inventing a new rate-limit budget in the caller.
func ESIRetryTime(err error) time.Time {
	var retry retryError
	if errors.As(err, &retry) {
		return retry.Until
	}
	return time.Time{}
}
