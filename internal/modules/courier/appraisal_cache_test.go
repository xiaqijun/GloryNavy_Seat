package courier

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const completeAppraisal = `{"success":true,"from_cache":false,"appraisal":{"buy_price":100,"sell_price":120,"volume":3,"failed_items":{}}}`

func TestAppraisalReuseExpiryAndValueIsolation(t *testing.T) {
	var landing, posts atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			landing.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "token"})
			return
		}
		posts.Add(1)
		fmt.Fprint(w, completeAppraisal)
	})
	first, e := s.Appraise(context.Background(), "Tritanium 1")
	if e != nil || first.FromCache {
		t.Fatal(first, e)
	}
	*first.Appraisal.Sell = 999
	first.Appraisal.Failed["bad"] = 1
	hit, e := s.Appraise(context.Background(), " Tritanium 1 ")
	if e != nil || !hit.FromCache || *hit.Appraisal.Sell != 120 || len(hit.Appraisal.Failed) != 0 {
		t.Fatal(hit, e)
	}
	if _, e = s.Appraise(context.Background(), "Tritanium 2"); e != nil {
		t.Fatal(e)
	}
	if landing.Load() != 1 || posts.Load() != 2 {
		t.Fatal("repeated handshake or cache miss", landing.Load(), posts.Load())
	}
	key := sha256.Sum256([]byte("Tritanium 1"))
	s.quoteMu.Lock()
	v := s.quotes[key]
	v.expires = time.Now().Add(-time.Second)
	s.quotes[key] = v
	s.quoteMu.Unlock()
	s.sessionMu.Lock()
	s.session.expires = time.Time{}
	s.sessionMu.Unlock()
	if _, e = s.Appraise(context.Background(), "Tritanium 1"); e != nil {
		t.Fatal(e)
	}
	if landing.Load() != 2 || posts.Load() != 3 {
		t.Fatal("expired result or session reused", landing.Load(), posts.Load())
	}
}

func TestAppraisalFailurePartialAndForbiddenAreNotCached(t *testing.T) {
	var landing, posts atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			landing.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "token"})
			return
		}
		switch posts.Add(1) {
		case 1:
			w.WriteHeader(403)
		case 2:
			fmt.Fprint(w, appraisalFixture)
		case 3:
			fmt.Fprint(w, "invalid")
		default:
			fmt.Fprint(w, completeAppraisal)
		}
	})
	for i := 1; i <= 5; i++ {
		v, e := s.Appraise(context.Background(), "Tritanium 1")
		if i == 1 || i == 3 {
			if !errors.Is(e, ErrUnavailable) {
				t.Fatal(i, e)
			}
		} else if e != nil {
			t.Fatal(i, e)
		}
		if i == 5 && !v.FromCache {
			t.Fatal("complete result not cached")
		}
	}
	if posts.Load() != 4 || landing.Load() != 2 {
		t.Fatal("failure replayed or cached", landing.Load(), posts.Load())
	}
}

func TestAppraisalConcurrentReadersAndCancellation(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var posts atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "token"})
			return
		}
		posts.Add(1)
		entered <- struct{}{}
		<-release
		fmt.Fprint(w, completeAppraisal)
	})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { _, e := s.Appraise(ctx, "same"); first <- e }()
	<-entered
	peer := make(chan error, 1)
	go func() { _, e := s.Appraise(context.Background(), "same"); peer <- e }()
	other := make(chan error, 1)
	go func() { _, e := s.Appraise(context.Background(), "different"); other <- e }()
	<-entered
	if _, e := s.Appraise(context.Background(), "third"); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	cancel()
	if e := <-first; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(release)
	if e := <-peer; e != nil {
		t.Fatal(e)
	}
	if e := <-other; e != nil {
		t.Fatal(e)
	}
	if posts.Load() != 2 {
		t.Fatal("duplicate upstream request", posts.Load())
	}
}

func TestAppraisalCacheBoundAndExpiredCookie(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			http.SetCookie(w, &http.Cookie{Name: "_xsrf", Value: "token"})
			return
		}
		fmt.Fprint(w, completeAppraisal)
	})
	for i := 0; i < 130; i++ {
		if _, e := s.Appraise(context.Background(), fmt.Sprint(i)); e != nil {
			t.Fatal(e)
		}
	}
	if len(s.quotes) > 128 {
		t.Fatal("unbounded cache", len(s.quotes))
	}
	expired := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("expired cookie sent")
		}
		w.Header().Set("Set-Cookie", "_xsrf=token; Expires=Thu, 01 Jan 1970 00:00:00 GMT")
	})
	if _, e := expired.Appraise(context.Background(), strings.Repeat("x", 1)); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
