package courier

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/url"
	"time"
)

type upstreamSession struct {
	cookie  http.Cookie
	token   string
	expires time.Time
}
type cachedAppraisal struct {
	value   AppraisalResult
	expires time.Time
}

// This is an anonymous upstream-only XSRF session, never a member's session.
func (s *Service) appraisalSession(ctx context.Context) (upstreamSession, error) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return upstreamSession{}, err
	}
	if time.Now().Before(s.session.expires) {
		return s.session, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.request(ctx, "GET", "/", nil)
	if err != nil {
		return upstreamSession{}, ErrUnavailable
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return upstreamSession{}, ErrUnavailable
	}
	var cookie *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == "_xsrf" {
			cookie = c
			break
		}
	}
	if cookie == nil || cookie.MaxAge < 0 || len(cookie.Value) > 4096 {
		return upstreamSession{}, ErrUnavailable
	}
	token, err := url.QueryUnescape(cookie.Value)
	if err != nil || token == "" {
		return upstreamSession{}, ErrUnavailable
	}
	expires := time.Now().Add(5 * time.Minute)
	if cookie.MaxAge > 0 && cookie.MaxAge < 300 {
		expires = time.Now().Add(time.Duration(cookie.MaxAge) * time.Second)
	}
	if !cookie.Expires.IsZero() && cookie.Expires.Before(expires) {
		expires = cookie.Expires
	}
	if !time.Now().Before(expires) {
		return upstreamSession{}, ErrUnavailable
	}
	// Only the cookie is needed. Reading the unrelated homepage body can take
	// several seconds; HTTP/2 still reuses the connection after closing this stream.
	s.session = upstreamSession{cookie: *cookie, token: token, expires: expires}
	return s.session, nil
}

func cloneAppraisal(v AppraisalResult) AppraisalResult {
	copyNumber := func(p *float64) *float64 {
		if p == nil {
			return nil
		}
		n := *p
		return &n
	}
	v.Appraisal.Buy = copyNumber(v.Appraisal.Buy)
	v.Appraisal.Sell = copyNumber(v.Appraisal.Sell)
	v.Appraisal.Volume = copyNumber(v.Appraisal.Volume)
	if v.Appraisal.URL != nil {
		u := *v.Appraisal.URL
		v.Appraisal.URL = &u
	}
	failed := make(map[string]float64, len(v.Appraisal.Failed))
	for k, n := range v.Appraisal.Failed {
		failed[k] = n
	}
	v.Appraisal.Failed = failed
	return v
}

func (s *Service) cachedAppraise(ctx context.Context, text string) (AppraisalResult, error) {
	if err := ctx.Err(); err != nil {
		return AppraisalResult{}, err
	}
	key := sha256.Sum256([]byte(text))
	result := s.quoteGroup.DoChan(string(key[:]), func() (any, error) {
		s.quoteMu.Lock()
		cached, ok := s.quotes[key]
		s.quoteMu.Unlock()
		if ok && time.Now().Before(cached.expires) {
			v := cached.value
			v.FromCache = true
			return v, nil
		}
		// Shared work has a hard bound; a disconnected reader does not cancel peers.
		fetchCtx, cancel := context.WithTimeout(context.Background(), 28*time.Second)
		defer cancel()
		v, err := s.appraiseUpstream(fetchCtx, text)
		if err != nil {
			return AppraisalResult{}, err
		}
		if len(v.Appraisal.Failed) == 0 {
			s.quoteMu.Lock()
			if s.quotes == nil {
				s.quotes = make(map[[32]byte]cachedAppraisal)
			}
			for k, item := range s.quotes {
				if !time.Now().Before(item.expires) {
					delete(s.quotes, k)
				}
			}
			if len(s.quotes) >= 128 {
				for k := range s.quotes {
					delete(s.quotes, k)
					break
				}
			}
			s.quotes[key] = cachedAppraisal{value: cloneAppraisal(v), expires: time.Now().Add(time.Minute)}
			s.quoteMu.Unlock()
		}
		return v, nil
	})
	select {
	case <-ctx.Done():
		return AppraisalResult{}, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return AppraisalResult{}, r.Err
		}
		return cloneAppraisal(r.Val.(AppraisalResult)), nil
	}
}
