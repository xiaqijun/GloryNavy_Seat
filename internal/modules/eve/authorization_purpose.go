package eve

import "context"

// Explicit purpose is separate from the scopes present in an authorization.
type authorizationPurposeKey struct{}

type preserveScopesKey struct{}

// PreserveExistingScopes protects an OAuth login from narrowing an existing grant.
// Explicit credential replacement and revocation retain their existing behavior.
func PreserveExistingScopes(ctx context.Context) context.Context {
	return context.WithValue(ctx, preserveScopesKey{}, true)
}
