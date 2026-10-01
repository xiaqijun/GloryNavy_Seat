package eve

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const discoveryURL = "https://login.eveonline.com/.well-known/oauth-authorization-server"
const issuer = "https://login.eveonline.com"

var ErrSSO = errors.New("EVE SSO request or verification failed")
var ErrReauthorize = errors.New("EVE authorization must be renewed")

const CorporationRolesScope = "esi-characters.read_corporation_roles.v1"

type Character struct {
	ID            int64
	Name, Owner   string
	authorization *authorization
}
type metadata struct {
	Issuer    string `json:"issuer"`
	Authorize string `json:"authorization_endpoint"`
	Token     string `json:"token_endpoint"`
	JWKS      string `json:"jwks_uri"`
}
type claims struct {
	jwt.RegisteredClaims
	Name            string   `json:"name"`
	Owner           string   `json:"owner"`
	AuthorizedParty string   `json:"azp"`
	Tenant          string   `json:"tenant"`
	Scopes          []string `json:"scp"`
}
type Client struct {
	id, secret, callback       string
	roles                      bool
	scopes                     []string
	http                       *http.Client
	mu                         sync.Mutex
	metadata                   metadata
	metadataUntil              time.Time
	keys                       map[string]*rsa.PublicKey
	keysUntil, timeKeysFetched time.Time
}

func NewClient(id, secret, callback string) *Client {
	return &Client{id: id, secret: secret, callback: callback, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Configured() bool              { return c.id != "" && c.secret != "" }
func (c *Client) EnableCorporationRoles()       { c.roles = true }
func (c *Client) CorporationRolesEnabled() bool { return c.roles }

func trustedEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "login.eveonline.com" && u.User == nil && u.Fragment == "" && u.RawQuery == ""
}
func (c *Client) fetchJSON(ctx context.Context, method, endpoint string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return ErrSSO
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GloryNavy/0.1.0 (EVE SSO)")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(c.id, c.secret)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return ErrSSO
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		if res.StatusCode == 429 {
			return retryError{Until: retryUntil(res.Header, time.Now())}
		}
		if form != nil && res.StatusCode == 400 {
			var failure struct {
				Error string `json:"error"`
			}
			if json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&failure) == nil && failure.Error == "invalid_grant" {
				return ErrReauthorize
			}
		}
		return ErrSSO
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return ErrSSO
	}
	if json.Unmarshal(data, out) != nil {
		return ErrSSO
	}
	return nil
}
func (c *Client) discover(ctx context.Context) (metadata, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.metadataUntil) {
		return c.metadata, nil
	}
	var m metadata
	if err := c.fetchJSON(ctx, "GET", discoveryURL, nil, &m); err != nil {
		return m, err
	}
	if m.Issuer != issuer || !trustedEndpoint(m.Authorize) || !trustedEndpoint(m.Token) || !trustedEndpoint(m.JWKS) {
		return m, ErrSSO
	}
	c.metadata = m
	c.metadataUntil = time.Now().Add(time.Hour)
	return m, nil
}
func (c *Client) AuthorizationURL(ctx context.Context, state, verifier string) (string, error) {
	if !c.Configured() {
		return "", ErrSSO
	}
	m, err := c.discover(ctx)
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{"response_type": {"code"}, "client_id": {c.id}, "redirect_uri": {c.callback}, "scope": {""}, "state": {state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}}
	if c.roles {
		query.Set("scope", strings.Join(c.RequestedScopes(), " "))
	}
	return m.Authorize + "?" + query.Encode(), nil
}

func (c *Client) Exchange(ctx context.Context, code, verifier string) (Character, error) {
	m, err := c.discover(ctx)
	if err != nil {
		return Character{}, err
	}
	var tokens tokenResponse
	if err = c.fetchJSON(ctx, "POST", m.Token, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.callback}, "code_verifier": {verifier}}, &tokens); err != nil {
		return Character{}, err
	}
	if !strings.EqualFold(tokens.TokenType, "Bearer") || len(tokens.AccessToken) > 32768 {
		return Character{}, ErrSSO
	}
	ch, err := c.verifiedAuthorization(ctx, m, tokens)
	if err == nil && !hasScopes(ch.authorization.Scopes, c.RequestedScopes()) {
		return Character{}, ErrReauthorize
	}
	return ch, err
}

func (c *Client) key(ctx context.Context, endpoint, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if now.Before(c.keysUntil) {
		if key := c.keys[kid]; key != nil {
			return key, nil
		}
		// An unknown kid may trigger a refresh, but not an unbounded request storm.
		if now.Sub(c.timeKeysFetched) < time.Minute {
			return nil, ErrSSO
		}
	}
	var jwks struct {
		Keys []struct{ Kty, Kid, Use, Alg, N, E string }
	}
	c.timeKeysFetched = now
	if err := c.fetchJSON(ctx, "GET", endpoint, nil, &jwks); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, j := range jwks.Keys {
		if j.Kty != "RSA" || j.Alg != "RS256" || (j.Use != "" && j.Use != "sig") || j.Kid == "" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(j.N)
		e, errE := base64.RawURLEncoding.DecodeString(j.E)
		if errN != nil || errE != nil || len(n) < 256 || len(n) > 1024 || len(e) == 0 || len(e) > 4 {
			continue
		}
		exp := new(big.Int).SetBytes(e).Int64()
		if exp < 3 || exp > 2147483647 || exp%2 == 0 {
			continue
		}
		if keys[j.Kid] != nil {
			return nil, ErrSSO
		}
		keys[j.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exp)}
	}
	c.keys = keys
	c.keysUntil = now.Add(time.Hour)
	if key := keys[kid]; key != nil {
		return key, nil
	}
	return nil, ErrSSO
}
func (c *Client) verify(ctx context.Context, m metadata, raw string) (Character, error) {
	cl := claims{}
	token, err := jwt.ParseWithClaims(raw, &cl, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" || len(kid) > 200 {
			return nil, ErrSSO
		}
		return c.key(ctx, m.JWKS, kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(m.Issuer), jwt.WithAudience("EVE Online"), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !token.Valid || !slices.Contains(cl.Audience, c.id) || cl.Name == "" || len([]rune(cl.Name)) > 200 || cl.Owner == "" || len(cl.Owner) > 512 {
		return Character{}, ErrSSO
	}
	if (cl.AuthorizedParty != "" && cl.AuthorizedParty != c.id) || (cl.Tenant != "" && cl.Tenant != "tranquility") {
		return Character{}, ErrSSO
	}
	if !strings.HasPrefix(cl.Subject, "CHARACTER:EVE:") {
		return Character{}, ErrSSO
	}
	text := strings.TrimPrefix(cl.Subject, "CHARACTER:EVE:")
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text {
		return Character{}, ErrSSO
	}
	return Character{ID: id, Name: cl.Name, Owner: cl.Owner, authorization: &authorization{AccessToken: raw, ExpiresAt: cl.ExpiresAt.Time, Scopes: cl.Scopes}}, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}
type authorization struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes"`
}

func (c *Client) verifiedAuthorization(ctx context.Context, m metadata, t tokenResponse) (Character, error) {
	if !strings.EqualFold(t.TokenType, "Bearer") || len(t.AccessToken) > 32768 || len(t.RefreshToken) > 8192 {
		return Character{}, ErrSSO
	}
	ch, err := c.verify(ctx, m, t.AccessToken)
	if err != nil {
		return Character{}, err
	}
	ch.authorization.RefreshToken = t.RefreshToken
	if c.roles && (!slices.Contains(ch.authorization.Scopes, CorporationRolesScope) || t.RefreshToken == "") {
		return Character{}, ErrReauthorize
	}
	return ch, nil
}
func (c *Client) refresh(ctx context.Context, refreshToken string) (Character, error) {
	m, err := c.discover(ctx)
	if err != nil {
		return Character{}, err
	}
	var tokens tokenResponse
	if err = c.fetchJSON(ctx, "POST", m.Token, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}, &tokens); err != nil {
		return Character{}, err
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}
	return c.verifiedAuthorization(ctx, m, tokens)
}
