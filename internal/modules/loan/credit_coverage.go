package loan

// requiredCreditCoverage uses subtraction so even a maximum-size principal
// cannot overflow when combined with existing commitments.
func requiredCreditCoverage(limit, used, principal int64) int64 {
	remaining := int64(0)
	if limit > used {
		remaining = limit - used
	}
	if principal <= remaining {
		return 0
	}
	return principal - remaining
}
