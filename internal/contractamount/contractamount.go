// Package contractamount converts stored hundredths of ISK to the whole ISK
// amount entered in an in-game contract.
package contractamount

import "math/big"

// WholeISK rounds a positive stored amount down to the game's whole ISK input.
func WholeISK(minor int64) int64 {
	if minor < 100 {
		return 0
	}
	return minor / 100
}

// Matches accepts the whole ISK amount shown to the issuer. An already issued
// historical contract with the original exact decimal amount also remains valid.
func Matches(minor int64, reward string) bool {
	want := WholeISK(minor)
	if want == 0 {
		return false
	}
	amount, ok := new(big.Rat).SetString(reward)
	return ok && (amount.Cmp(big.NewRat(want, 1)) == 0 || amount.Cmp(big.NewRat(minor, 100)) == 0)
}
