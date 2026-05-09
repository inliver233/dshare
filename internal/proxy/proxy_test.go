package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"dshare/internal/config"
	"dshare/internal/ratelimit"
	"dshare/internal/store"
)

func TestForwardDoesNotSendGoogleAPIKeyHeaderToNewAPI(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "discord-proxy", "tester", "Tester", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	var gotGoogleHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotGoogleHeader = r.Header.Get("x-goog-api-key")
		if auth := r.Header.Get("Authorization"); auth != "Bearer upstream-key" {
			t.Fatalf("Authorization = %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	h := New(config.Config{
		NewAPIBaseURL: upstream.URL,
		NewAPIKey:     "upstream-key",
	}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("x-goog-api-key", key.PlaintextKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotGoogleHeader != "" {
		t.Fatalf("x-goog-api-key forwarded to upstream: %q", gotGoogleHeader)
	}
	time.Sleep(10 * time.Millisecond)
}

func TestForwardRequestsIdentityEncoding(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "encoding-user", "encoding", "Encoding", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "encoding")
	if err != nil {
		t.Fatal(err)
	}

	var gotEncoding string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Accept-Encoding")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("Accept-Encoding", "br, gzip, zstd")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotEncoding != "identity" {
		t.Fatalf("Accept-Encoding = %q, want identity", gotEncoding)
	}
}

func TestForwardStripsBrowserCookieAndContentLengthHeaders(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "header-user", "header", "Header", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "header")
	if err != nil {
		t.Fatal(err)
	}

	var gotCookie, gotContentLength string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotContentLength = r.Header.Get("Content-Length")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"m"}`))
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("Cookie", "dshare_session=private")
	req.Header.Set("Content-Length", "999")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotCookie != "" {
		t.Fatalf("Cookie forwarded to upstream: %q", gotCookie)
	}
	if gotContentLength != "" {
		t.Fatalf("Content-Length header forwarded to upstream: %q", gotContentLength)
	}
}

func TestForwardStripsClientProxyHeadersAndAddsDshareHeaders(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "proxy-header-user", "header", "Header", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "header")
	if err != nil {
		t.Fatal(err)
	}

	got := http.Header{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("CF-Connecting-IP", "198.51.100.20")
	req.Header.Set("CF-Ray", "ray-id")
	req.Header.Set("X-Forwarded-For", "198.51.100.20, 203.0.113.10")
	req.Header.Set("X-Forwarded-Host", "share.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Real-IP", "198.51.100.21")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, header := range []string{"CF-Connecting-IP", "CF-Ray", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP"} {
		if got.Get(header) != "" {
			t.Fatalf("%s forwarded to upstream: %q", header, got.Get(header))
		}
	}
	if got.Get("X-Dshare-User-ID") != stringInt(user.ID) {
		t.Fatalf("X-Dshare-User-ID = %q", got.Get("X-Dshare-User-ID"))
	}
	if got.Get("X-Dshare-Client-IP") != "198.51.100.20" {
		t.Fatalf("X-Dshare-Client-IP = %q", got.Get("X-Dshare-Client-IP"))
	}
}

func TestForwardReplacesGeminiQueryKeyWithUpstreamKey(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "gemini-user", "gemini", "Gemini", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "gemini")
	if err != nil {
		t.Fatal(err)
	}

	var gotQueryKey string
	var gotGoogleHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueryKey = r.URL.Query().Get("key")
		gotGoogleHeader = r.Header.Get("x-goog-api-key")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:generateContent?key="+url.QueryEscape(key.PlaintextKey), nil)
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("x-goog-api-key", key.PlaintextKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotQueryKey != "upstream-key" {
		t.Fatalf("query key = %q, want upstream-key", gotQueryKey)
	}
	if gotGoogleHeader != "upstream-key" {
		t.Fatalf("x-goog-api-key = %q, want upstream-key", gotGoogleHeader)
	}
}

func TestForwardRemovesQueryKeyFromOpenAIModelsList(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "models-key-user", "models", "Models", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "models")
	if err != nil {
		t.Fatal(err)
	}

	var gotQueryKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueryKey = r.URL.Query().Get("key")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodGet, "/v1/models?key="+url.QueryEscape(key.PlaintextKey), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotQueryKey != "" {
		t.Fatalf("query key forwarded to OpenAI models list: %q", gotQueryKey)
	}
}

func TestAdminBypassesRateLimits(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	admin, err := s.UpsertDiscordUser(ctx, "admin-test", "admin", "Admin", "", store.UserLimits{
		RequestsPerMinute:     1,
		RequestsPerDay:        1,
		MaxConcurrentRequests: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAdminByDiscordID(ctx, admin.DiscordID); err != nil {
		t.Fatal(err)
	}
	admin, err = s.GetUserByID(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, admin.ID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin request %d status = %d, body = %s", i+1, rec.Code, rec.Body.String())
		}
	}
}

func TestUserStillUsesRateLimits(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "user-test", "user", "User", "", store.UserLimits{
		RequestsPerMinute:     1,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "user")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if i == 0 && rec.Code != http.StatusOK {
			t.Fatalf("first user request status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("second user request status = %d, want 429", rec.Code)
		}
		if i == 1 && !bytes.Contains(rec.Body.Bytes(), []byte("RPM")) {
			t.Fatalf("second user request body = %s", rec.Body.String())
		}
	}
}

func TestConcurrentLimitReturnsClearMessage(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "concurrent-user", "concurrent", "Concurrent", "", store.UserLimits{
		RequestsPerMinute:     100,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "concurrent")
	if err != nil {
		t.Fatal(err)
	}
	releaseUpstream := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseUpstream
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	server := httptest.NewServer(h)
	defer server.Close()

	firstReq, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	firstReq.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		resp, err := server.Client().Do(firstReq)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	time.Sleep(20 * time.Millisecond)

	secondReq, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	secondReq.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	resp, err := server.Client().Do(secondReq)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	close(releaseUpstream)
	<-firstDone
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	if !bytes.Contains(body, []byte("并发")) {
		t.Fatalf("body = %s", string(body))
	}
}

func TestModelsEndpointBypassesUserRateLimit(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "models-user", "models", "Models", "", store.UserLimits{
		RequestsPerMinute:     1,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "models")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("models request %d status = %d, body = %s", i+1, rec.Code, rec.Body.String())
		}
	}
}

func TestUnmeteredModelsDoesNotDisableChatRateLimit(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "mixed-user", "mixed", "Mixed", "", store.UserLimits{
		RequestsPerMinute:     1,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("models status = %d", rec.Code)
		}
	}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if i == 0 && rec.Code != http.StatusOK {
			t.Fatalf("first chat status = %d", rec.Code)
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("second chat status = %d, want 429", rec.Code)
		}
	}
}

func TestForwardFlushesStreamingResponse(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "stream-user", "stream", "Stream", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "stream")
	if err != nil {
		t.Fatal(err)
	}

	firstChunkSent := make(chan struct{})
	releaseSecondChunk := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream has no flusher")
		}
		_, _ = fmt.Fprint(w, "data: first\n\n")
		flusher.Flush()
		close(firstChunkSent)
		<-releaseSecondChunk
		_, _ = fmt.Fprint(w, "data: second\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	server := httptest.NewServer(h)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	select {
	case <-firstChunkSent:
	case <-time.After(time.Second):
		t.Fatal("upstream did not send first chunk")
	}
	buf := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "data: first\n\n" {
		t.Fatalf("first chunk = %q", string(buf))
	}
	close(releaseSecondChunk)
}

func TestForwardSendsKeepAliveBeforeSlowStreamingUpstreamHeaders(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "slow-stream-user", "stream", "Stream", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "stream")
	if err != nil {
		t.Fatal(err)
	}

	releaseUpstream := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseUpstream
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: done\n\n")
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	h.streamKeepAliveAfter = 10 * time.Millisecond
	h.streamKeepAliveInterval = time.Second
	server := httptest.NewServer(h)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"m","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, len(": dshare keepalive\n\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		close(releaseUpstream)
		t.Fatal(err)
	}
	if string(buf) != ": dshare keepalive\n\n" {
		close(releaseUpstream)
		t.Fatalf("keepalive = %q", string(buf))
	}
	close(releaseUpstream)
}

func TestForwardDoesNotRewriteHeaderAfterStreamKeepAlive(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "slow-error-stream-user", "stream", "Stream", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "stream")
	if err != nil {
		t.Fatal(err)
	}

	releaseUpstream := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseUpstream
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = fmt.Fprint(w, "<html>cloudflare timeout</html>")
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	h.streamKeepAliveAfter = 10 * time.Millisecond
	h.streamKeepAliveInterval = time.Second
	server := httptest.NewServer(h)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"m","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		close(releaseUpstream)
		t.Fatalf("status = %d, want already-committed 200", resp.StatusCode)
	}
	buf := make([]byte, len(": dshare keepalive\n\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		close(releaseUpstream)
		t.Fatal(err)
	}
	close(releaseUpstream)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("<html>")) || bytes.Contains(body, []byte("cloudflare timeout</html>")) {
		t.Fatalf("raw upstream HTML was forwarded: %q", string(body))
	}
	if !bytes.Contains(body, []byte(`"type":"dshare_error"`)) {
		t.Fatalf("body = %q", string(body))
	}
	if !bytes.Contains(body, []byte("upstream returned 504")) {
		t.Fatalf("body = %q", string(body))
	}
	if !bytes.Contains(body, []byte("data: [DONE]")) {
		t.Fatalf("body = %q", string(body))
	}
}

func TestForwardKeepsStreamingRequestBodyAfterDetection(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "body-user", "body", "Body", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "body")
	if err != nil {
		t.Fatal(err)
	}

	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: done\n\n")
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"m","stream":true}`))
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotBody != `{"model":"m","stream":true}` {
		t.Fatalf("upstream body = %q", gotBody)
	}
}

func TestForwardPreservesStreamingRequestBodyBytes(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "body-byte-user", "body", "Body", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "body")
	if err != nil {
		t.Fatal(err)
	}

	originalBody := "{\n  \"stream\" : true,\n  \"model\" : \"m\",\n  \"messages\" : []\n}"
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: done\n\n")
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(originalBody))
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotBody != originalBody {
		t.Fatalf("upstream body changed:\n got: %q\nwant: %q", gotBody, originalBody)
	}
}

func TestRealtimeProtocolKeyHelpers(t *testing.T) {
	raw := "realtime, openai-insecure-api-key.dsh-abc, openai-beta.realtime-v1"
	if got := extractRealtimeProtocolKey(raw); got != "dsh-abc" {
		t.Fatalf("extractRealtimeProtocolKey = %q, want dsh-abc", got)
	}
	replaced := replaceRealtimeProtocolKey(raw, "upstream-key")
	want := "realtime, openai-insecure-api-key.upstream-key, openai-beta.realtime-v1"
	if replaced != want {
		t.Fatalf("replaceRealtimeProtocolKey = %q, want %q", replaced, want)
	}
}

func TestForwardNormalizesDoubleV1Path(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "path-user", "path", "Path", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "path")
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodPost, "/v1/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("upstream path = %q, want /v1/messages", gotPath)
	}
}

func TestClassifyClientCanceledWriteErrors(t *testing.T) {
	for _, errText := range []string{
		"write tcp 127.0.0.1:8080->127.0.0.1:12345: write: broken pipe",
		"read tcp 127.0.0.1:8080->127.0.0.1:12345: read: connection reset by peer",
		"client disconnected",
	} {
		if got := classifyProxyError(errors.New(errText)); got != "client_canceled" {
			t.Fatalf("classifyProxyError(%q) = %q, want client_canceled", errText, got)
		}
	}
}

func TestForwardRecordsUpstreamErrorPreview(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, err := s.UpsertDiscordUser(ctx, "error-user", "error", "Error", "", store.UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "error")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"upstream overloaded"}`))
	}))
	defer upstream.Close()

	h := New(config.Config{NewAPIBaseURL: upstream.URL, NewAPIKey: "upstream-key"}, s, ratelimit.New())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key.PlaintextKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"error":"upstream overloaded"}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
