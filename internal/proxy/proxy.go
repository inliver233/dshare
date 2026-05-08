package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"dshare/internal/config"
	"dshare/internal/ratelimit"
	"dshare/internal/store"
)

type Handler struct {
	mu      sync.RWMutex
	cfg     config.Config
	store   *store.Store
	limiter *ratelimit.Limiter
	client  *http.Client

	streamKeepAliveAfter    time.Duration
	streamKeepAliveInterval time.Duration
}

type ContextUser struct {
	User   *store.User
	APIKey *store.APIKey
}

type forwardResult struct {
	Status              int
	UpstreamStatus      int
	BytesOut            int64
	FirstByteMS         int64
	UpstreamPath        string
	UpstreamRequestID   string
	RequestContentType  string
	ResponseContentType string
	Stream              bool
	ErrorType           string
	ErrorMessage        string
}

func New(cfg config.Config, s *store.Store, limiter *ratelimit.Limiter) *Handler {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Handler{
		cfg:     cfg,
		store:   s,
		limiter: limiter,
		client:  &http.Client{Transport: transport, Timeout: 0},

		streamKeepAliveAfter:    cfg.StreamKeepAliveAfter,
		streamKeepAliveInterval: cfg.StreamKeepAliveInterval,
	}
}

func (h *Handler) SetConfig(cfg config.Config) {
	h.mu.Lock()
	h.cfg = cfg
	h.streamKeepAliveAfter = cfg.StreamKeepAliveAfter
	h.streamKeepAliveInterval = cfg.StreamKeepAliveInterval
	h.mu.Unlock()
}

func (h *Handler) config() config.Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctxUser, err := h.authenticate(r.Context(), r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid dshare api key")
		return
	}
	if ctxUser.User.Role != "admin" && !isUnmeteredRequest(r) {
		todayCount, err := h.store.CountAcceptedRequestsToday(r.Context(), ctxUser.User.ID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "rate limit check failed")
			return
		}
		if ctxUser.User.RequestsPerDay > 0 && todayCount >= int64(ctxUser.User.RequestsPerDay) {
			w.Header().Set("Retry-After", secondsUntilNextUTCDay())
			writeOpenAIError(w, http.StatusTooManyRequests, "daily rate limit exceeded")
			result := forwardResult{Status: http.StatusTooManyRequests, ErrorType: "daily_rate_limit", ErrorMessage: "daily rate limit exceeded"}
			_ = h.recordRequest(context.Background(), r, ctxUser, result, start)
			logProxyIssue(r, ctxUser, result, start)
			return
		}
		release, ok, retryAfter := h.limiter.Allow(
			ctxUser.User.ID,
			ctxUser.User.MaxConcurrentRequests,
			ratelimit.Rule{Name: "minute", Limit: ctxUser.User.RequestsPerMinute, Window: time.Minute},
		)
		if !ok {
			if retryAfter > 0 {
				w.Header().Set("Retry-After", secondsCeil(retryAfter))
			}
			writeOpenAIError(w, http.StatusTooManyRequests, "rate limit exceeded")
			result := forwardResult{Status: http.StatusTooManyRequests, ErrorType: "rate_limit", ErrorMessage: "rate limit exceeded"}
			_ = h.recordRequest(context.Background(), r, ctxUser, result, start)
			logProxyIssue(r, ctxUser, result, start)
			return
		}
		defer release()
	}

	result := h.forward(w, r, ctxUser, start)
	_ = h.recordRequest(context.Background(), r, ctxUser, result, start)
	logProxyIssue(r, ctxUser, result, start)
}

func (h *Handler) recordRequest(ctx context.Context, r *http.Request, ctxUser *ContextUser, result forwardResult, start time.Time) error {
	return h.store.RecordRequest(ctx, store.RequestLog{
		UserID:              ctxUser.User.ID,
		APIKeyID:            ctxUser.APIKey.ID,
		Method:              r.Method,
		Path:                r.URL.Path,
		Query:               r.URL.RawQuery,
		UpstreamPath:        result.UpstreamPath,
		Status:              result.Status,
		UpstreamStatus:      result.UpstreamStatus,
		DurationMS:          int64(time.Since(start).Milliseconds()),
		FirstByteMS:         result.FirstByteMS,
		BytesIn:             r.ContentLength,
		BytesOut:            result.BytesOut,
		Stream:              result.Stream,
		IP:                  clientIP(r),
		UserAgent:           limitString(r.UserAgent(), 500),
		Referer:             limitString(r.Referer(), 500),
		RequestID:           limitString(r.Header.Get("X-Request-Id"), 120),
		UpstreamRequestID:   limitString(result.UpstreamRequestID, 160),
		RequestContentType:  limitString(result.RequestContentType, 160),
		ResponseContentType: limitString(result.ResponseContentType, 160),
		ErrorType:           result.ErrorType,
		ErrorMessage:        result.ErrorMessage,
		CreatedAt:           time.Now().UTC(),
	})
}

func (h *Handler) authenticate(ctx context.Context, r *http.Request) (*ContextUser, error) {
	key := r.Header.Get("Authorization")
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	if key == "" {
		key = r.Header.Get("x-api-key")
	}
	if key == "" {
		key = r.Header.Get("x-goog-api-key")
	}
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	if key == "" {
		key = extractRealtimeProtocolKey(r.Header.Get("Sec-WebSocket-Protocol"))
	}
	user, apiKey, err := h.store.GetAPIKeyUser(ctx, key)
	if err != nil {
		return nil, err
	}
	return &ContextUser{User: user, APIKey: apiKey}, nil
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, ctxUser *ContextUser, start time.Time) forwardResult {
	result := forwardResult{
		Status:             http.StatusOK,
		RequestContentType: r.Header.Get("Content-Type"),
	}
	cfg := h.config()
	if cfg.NewAPIBaseURL == "" || cfg.NewAPIKey == "" {
		writeOpenAIError(w, http.StatusBadGateway, "new-api upstream is not configured")
		result.Status = http.StatusBadGateway
		result.ErrorType = "config_error"
		result.ErrorMessage = "new-api upstream is not configured"
		return result
	}
	upstreamURL, err := url.Parse(cfg.NewAPIBaseURL)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "invalid new-api upstream url")
		result.Status = http.StatusBadGateway
		result.ErrorType = "config_error"
		result.ErrorMessage = err.Error()
		return result
	}
	cleanPath := normalizeProxyPath(r.URL.Path)
	result.UpstreamPath = cleanPath
	upstreamURL.Path = singleJoiningSlash(upstreamURL.Path, cleanPath)
	q := cloneValues(r.URL.Query())
	preserveGeminiAuth := shouldPreserveGeminiAuthHeader(cleanPath, r)
	replaceClientKeyQuery(q, cfg.NewAPIKey, preserveGeminiAuth)
	upstreamURL.RawQuery = q.Encode()

	if isWebSocketRequest(r) {
		return h.forwardWebSocket(w, r, ctxUser, start, upstreamURL, &result)
	}
	likelyStream := isLikelyStreamingRequest(r)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "failed to build upstream request")
		result.Status = http.StatusInternalServerError
		result.ErrorType = "build_request_error"
		result.ErrorMessage = err.Error()
		return result
	}
	copyHeader(req.Header, r.Header)
	req.Host = upstreamURL.Host
	prepareUpstreamHeaders(req.Header, cfg.NewAPIKey, preserveGeminiAuth)
	req.Header.Set("X-Dshare-User-ID", stringInt(ctxUser.User.ID))
	req.Header.Set("X-Dshare-Client-IP", clientIP(r))

	keepAlive := h.maybeStartStreamKeepAlive(w, r, req, likelyStream)
	resp, err := h.client.Do(req)
	if keepAlive != nil {
		keepAlive.stop()
	}
	if err != nil {
		result.ErrorMessage = err.Error()
		result.ErrorType = classifyProxyError(err)
		if errors.Is(r.Context().Err(), context.Canceled) {
			result.Status = 499
			return result
		}
		if keepAlive != nil && keepAlive.wrote() {
			result.Status = http.StatusOK
			result.Stream = true
			writeSSEError(w, "upstream request failed", true)
			return result
		}
		writeOpenAIError(w, http.StatusBadGateway, "upstream request failed")
		result.Status = http.StatusBadGateway
		return result
	}
	defer resp.Body.Close()

	result.UpstreamStatus = resp.StatusCode
	result.Status = resp.StatusCode
	result.UpstreamRequestID = firstHeader(resp.Header, "X-Oneapi-Request-Id", "X-Request-Id", "Cf-Ray")
	result.ResponseContentType = resp.Header.Get("Content-Type")
	result.Stream = isStreamingResponse(resp)
	result.FirstByteMS = int64(time.Since(start).Milliseconds())
	if keepAlive != nil && keepAlive.wrote() {
		result.Status = http.StatusOK
		result.Stream = true
	} else {
		copyHeader(w.Header(), resp.Header)
		removeHopByHopHeaders(w.Header())
		if result.Stream {
			w.Header().Del("Content-Length")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("X-Accel-Buffering", "no")
		}
		w.WriteHeader(resp.StatusCode)
	}
	if keepAlive != nil && keepAlive.wrote() && resp.StatusCode >= 400 {
		result.Status = http.StatusOK
		result.Stream = true
		result.ErrorType = "upstream_status"
		result.ErrorMessage = readBodyPreview(resp.Body, 4096)
		result.BytesOut = int64(len(result.ErrorMessage))
		writeSSEError(w, upstreamStatusMessage(resp.StatusCode, result.ErrorMessage), true)
		return result
	}
	var preview bytes.Buffer
	n, firstBodyMS, copyErr := copyResponse(w, resp.Body, start, &preview, resp.StatusCode >= 400)
	result.BytesOut = n
	if firstBodyMS > 0 {
		result.FirstByteMS = firstBodyMS
	}
	if copyErr != nil {
		result.ErrorType = classifyProxyError(copyErr)
		result.ErrorMessage = copyErr.Error()
		if result.ErrorType == "client_canceled" {
			result.Status = 499
		}
	} else if resp.StatusCode >= 400 {
		result.ErrorType = "upstream_status"
		result.ErrorMessage = limitString(preview.String(), 1000)
	}
	return result
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		if isSkippedProxyHeader(key) {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func removeHopByHopHeaders(h http.Header) {
	for _, key := range []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		h.Del(key)
	}
}

func cleanProxyHeaders(h http.Header) {
	for key := range h {
		if isSkippedProxyHeader(key) {
			h.Del(key)
		}
	}
}

func cleanForwardedClientHeaders(h http.Header) {
	for key := range h {
		if isClientProxyHeader(key) {
			h.Del(key)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func isSkippedProxyHeader(key string) bool {
	if isHopByHopHeader(key) {
		return true
	}
	switch strings.ToLower(key) {
	case "cookie", "host", "content-length", "accept-encoding",
		"x-request-id":
		return true
	default:
		return isClientProxyHeader(key)
	}
}

func isClientProxyHeader(key string) bool {
	switch strings.ToLower(key) {
	case "cf-connecting-ip", "cf-ipcountry", "cf-ray", "cf-visitor",
		"forwarded", "remote-host", "true-client-ip",
		"x-forwarded-for", "x-forwarded-host", "x-forwarded-port", "x-forwarded-proto",
		"x-real-ip":
		return true
	default:
		return false
	}
}

func prepareUpstreamHeaders(h http.Header, upstreamKey string, preserveGeminiHeader bool) {
	h.Del("Cookie")
	h.Del("Content-Length")
	h.Del("Accept-Encoding")
	h.Set("Accept-Encoding", "identity")
	h.Set("Authorization", "Bearer "+upstreamKey)
	h.Set("X-API-Key", upstreamKey)
	h.Set("x-api-key", upstreamKey)
	if preserveGeminiHeader {
		h.Set("x-goog-api-key", upstreamKey)
		h.Set("X-Goog-Api-Key", upstreamKey)
	} else {
		h.Del("x-goog-api-key")
		h.Del("X-Goog-Api-Key")
	}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

func normalizeProxyPath(path string) string {
	if path == "" {
		return "/"
	}
	for strings.HasPrefix(path, "/v1/v1/") {
		path = "/v1/" + strings.TrimPrefix(path, "/v1/v1/")
	}
	if path == "/v1/v1" {
		path = "/v1"
	}
	for strings.HasPrefix(path, "/v1/v1beta/") {
		path = "/v1beta/" + strings.TrimPrefix(path, "/v1/v1beta/")
	}
	if path == "/v1/v1beta" {
		path = "/v1beta"
	}
	return path
}

func shouldPreserveGeminiAuthHeader(path string, r *http.Request) bool {
	if r.Header.Get("anthropic-version") != "" {
		return false
	}
	if strings.HasPrefix(path, "/v1beta/models") {
		return true
	}
	if strings.HasPrefix(path, "/v1/models/") && strings.Contains(path, ":") {
		return true
	}
	if strings.HasPrefix(path, "/v1/models/") && (r.Header.Get("x-goog-api-key") != "" || r.URL.Query().Get("key") != "") {
		return true
	}
	return false
}

func cloneValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, items := range values {
		out[key] = append([]string(nil), items...)
	}
	return out
}

func replaceClientKeyQuery(values url.Values, upstreamKey string, preserveGeminiAuth bool) {
	if _, ok := values["key"]; ok && preserveGeminiAuth {
		values.Set("key", upstreamKey)
	} else if !preserveGeminiAuth {
		values.Del("key")
	}
}

func isUnmeteredRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch normalizeProxyPath(r.URL.Path) {
	case "/v1/models", "/v1beta/models":
		return true
	default:
		return false
	}
}

func copyResponse(w http.ResponseWriter, src io.Reader, start time.Time, preview *bytes.Buffer, capturePreview bool) (int64, int64, error) {
	if flusher, ok := w.(http.Flusher); ok {
		buf := make([]byte, 32*1024)
		var written int64
		var firstByteMS int64
		for {
			nr, er := src.Read(buf)
			if nr > 0 {
				if firstByteMS == 0 {
					firstByteMS = int64(time.Since(start).Milliseconds())
				}
				if capturePreview && preview != nil && preview.Len() < 4096 {
					remaining := 4096 - preview.Len()
					if remaining > nr {
						remaining = nr
					}
					_, _ = preview.Write(buf[:remaining])
				}
				nw, ew := w.Write(buf[:nr])
				if nw > 0 {
					written += int64(nw)
					flusher.Flush()
				}
				if ew != nil {
					return written, firstByteMS, ew
				}
				if nr != nw {
					return written, firstByteMS, io.ErrShortWrite
				}
			}
			if er != nil {
				if er == io.EOF {
					return written, firstByteMS, nil
				}
				return written, firstByteMS, er
			}
		}
	}
	var firstByteMS int64
	writer := io.Writer(w)
	if capturePreview && preview != nil {
		writer = io.MultiWriter(w, previewLimitWriter{buf: preview, limit: 4096})
	}
	n, err := io.Copy(writer, &firstReadTracker{
		reader: src,
		onFirst: func() {
			firstByteMS = int64(time.Since(start).Milliseconds())
		},
	})
	return n, firstByteMS, err
}

type streamKeepAlive struct {
	done    chan struct{}
	stopped chan struct{}
	once    sync.Once
	mu      sync.Mutex
	written bool
}

func (k *streamKeepAlive) stop() {
	k.once.Do(func() {
		close(k.done)
		select {
		case <-k.stopped:
		case <-time.After(2 * time.Second):
		}
	})
}

func (k *streamKeepAlive) markWritten() {
	k.mu.Lock()
	k.written = true
	k.mu.Unlock()
}

func (k *streamKeepAlive) wrote() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.written
}

func (h *Handler) maybeStartStreamKeepAlive(w http.ResponseWriter, r *http.Request, req *http.Request, likelyStream bool) *streamKeepAlive {
	if !likelyStream {
		return nil
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}
	after := h.streamKeepAliveAfter
	interval := h.streamKeepAliveInterval
	if after <= 0 || interval <= 0 {
		return nil
	}
	req.Header.Set("Accept", "text/event-stream")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	keepAlive := &streamKeepAlive{
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go func() {
		defer close(keepAlive.stopped)
		timer := time.NewTimer(after)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-keepAlive.done:
			return
		case <-r.Context().Done():
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := io.WriteString(w, ": dshare keepalive\n\n"); err != nil {
				return
			}
			keepAlive.markWritten()
			flusher.Flush()
			select {
			case <-ticker.C:
			case <-keepAlive.done:
				return
			case <-r.Context().Done():
				return
			}
		}
	}()
	return keepAlive
}

func isLikelyStreamingRequest(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		return false
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/v1/realtime") {
		return true
	}
	switch normalizeProxyPath(r.URL.Path) {
	case "/v1/chat/completions", "/v1/completions", "/v1/messages", "/v1/responses":
	default:
		return false
	}
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "json") {
		return false
	}
	return bodyHasJSONBool(r, "stream")
}

func bodyHasJSONBool(r *http.Request, field string) bool {
	if r.Body == nil {
		return false
	}
	data, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	r.ContentLength = int64(len(data))
	var payload map[string]json.RawMessage
	if err != nil {
		return false
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&payload); err != nil {
		return false
	}
	raw, ok := payload[field]
	if !ok {
		return false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return value
}

func isStreamingResponse(resp *http.Response) bool {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	return strings.Contains(contentType, "text/event-stream")
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func (h *Handler) forwardWebSocket(w http.ResponseWriter, r *http.Request, ctxUser *ContextUser, start time.Time, upstreamURL *url.URL, result *forwardResult) forwardResult {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusBadRequest, "websocket upgrade requires GET")
		result.Status = http.StatusBadRequest
		result.ErrorType = "bad_websocket_request"
		result.ErrorMessage = "websocket upgrade requires GET"
		return *result
	}
	target := *upstreamURL
	switch target.Scheme {
	case "https", "http":
	default:
		writeOpenAIError(w, http.StatusBadGateway, "invalid websocket upstream url")
		result.Status = http.StatusBadGateway
		result.ErrorType = "config_error"
		result.ErrorMessage = "invalid websocket upstream url"
		return *result
	}
	director := func(req *http.Request) {
		req.URL = &target
		req.Host = target.Host
		cleanForwardedClientHeaders(req.Header)
		prepareUpstreamHeaders(req.Header, h.config().NewAPIKey, shouldPreserveGeminiAuthHeader(target.Path, r))
		req.Header["X-Forwarded-For"] = nil
		req.Header.Set("Sec-WebSocket-Protocol", replaceRealtimeProtocolKey(req.Header.Get("Sec-WebSocket-Protocol"), h.config().NewAPIKey))
		req.Header.Set("X-Dshare-User-ID", stringInt(ctxUser.User.ID))
		req.Header.Set("X-Dshare-Client-IP", clientIP(r))
	}
	proxy := &httputil.ReverseProxy{
		Director:       director,
		Transport:      h.client.Transport,
		FlushInterval:  -1,
		ModifyResponse: h.captureProxyResponse(result, start),
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			result.ErrorType = classifyProxyError(err)
			result.ErrorMessage = err.Error()
			if result.ErrorType == "client_canceled" {
				result.Status = 499
				return
			}
			result.Status = http.StatusBadGateway
			writeOpenAIError(rw, http.StatusBadGateway, "upstream websocket failed")
		},
	}
	proxy.ServeHTTP(w, r)
	if result.Status == http.StatusOK && result.UpstreamStatus == 0 {
		result.Status = http.StatusSwitchingProtocols
	}
	return *result
}

func (h *Handler) captureProxyResponse(result *forwardResult, start time.Time) func(*http.Response) error {
	return func(resp *http.Response) error {
		result.Status = resp.StatusCode
		result.UpstreamStatus = resp.StatusCode
		result.FirstByteMS = int64(time.Since(start).Milliseconds())
		result.ResponseContentType = resp.Header.Get("Content-Type")
		result.UpstreamRequestID = firstHeader(resp.Header, "X-Oneapi-Request-Id", "X-Request-Id", "Cf-Ray")
		return nil
	}
}

func extractRealtimeProtocolKey(value string) string {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "openai-insecure-api-key.") {
			return strings.TrimPrefix(part, "openai-insecure-api-key.")
		}
		if strings.HasPrefix(part, "openai-insecure-api-key") {
			return strings.TrimPrefix(part, "openai-insecure-api-key")
		}
	}
	return ""
}

func replaceRealtimeProtocolKey(value, upstreamKey string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	parts := strings.Split(value, ",")
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(trimmed, "openai-insecure-api-key") {
			parts[i] = "openai-insecure-api-key." + upstreamKey
		} else {
			parts[i] = trimmed
		}
	}
	return strings.Join(parts, ", ")
}

type firstReadTracker struct {
	reader  io.Reader
	onFirst func()
	seen    bool
}

func (r *firstReadTracker) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && !r.seen {
		r.seen = true
		if r.onFirst != nil {
			r.onFirst()
		}
	}
	return n, err
}

type previewLimitWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w previewLimitWriter) Write(p []byte) (int, error) {
	if w.buf != nil && w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.buf.Write(p[:remaining])
	}
	return len(p), nil
}

func firstHeader(h http.Header, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(h.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func readBodyPreview(r io.Reader, limit int64) string {
	if r == nil || limit <= 0 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil && len(data) == 0 {
		return ""
	}
	return limitString(string(data), int(limit))
}

func upstreamStatusMessage(status int, preview string) string {
	message := "upstream returned " + stringInt(int64(status))
	preview = humanUpstreamErrorPreview(preview)
	if preview != "" {
		message += ": " + preview
	}
	return limitString(message, 500)
}

func humanUpstreamErrorPreview(preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(preview), "<html") || strings.Contains(strings.ToLower(preview), "<!doctype html") {
		return "received an HTML error page from upstream"
	}
	var payload any
	if json.Unmarshal([]byte(preview), &payload) == nil {
		if msg := extractErrorMessage(payload); msg != "" {
			return msg
		}
	}
	preview = strings.Join(strings.Fields(preview), " ")
	return limitString(preview, 300)
}

func extractErrorMessage(value any) string {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"message", "detail"} {
			if msg, ok := v[key].(string); ok && strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
		}
		if nested, ok := v["error"]; ok {
			if msg := extractErrorMessage(nested); msg != "" {
				return msg
			}
		}
	case string:
		return strings.TrimSpace(v)
	}
	return ""
}

func classifyProxyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "client_canceled"
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
		return "client_canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "network_timeout"
	}
	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "context canceled") ||
		strings.Contains(errText, "broken pipe") ||
		strings.Contains(errText, "connection reset by peer") ||
		strings.Contains(errText, "client disconnected") {
		return "client_canceled"
	}
	return "proxy_error"
}

func logProxyIssue(r *http.Request, ctxUser *ContextUser, result forwardResult, start time.Time) {
	duration := time.Since(start)
	if result.Status < 400 && duration < 30*time.Second {
		return
	}
	log.Printf(
		"proxy user=%d role=%s key=%d ip=%s method=%s path=%s upstream_path=%s status=%d upstream_status=%d duration_ms=%d first_byte_ms=%d bytes_out=%d stream=%t err_type=%s err=%q upstream_req=%s ua=%q",
		ctxUser.User.ID,
		ctxUser.User.Role,
		ctxUser.APIKey.ID,
		clientIP(r),
		r.Method,
		r.URL.Path,
		result.UpstreamPath,
		result.Status,
		result.UpstreamStatus,
		duration.Milliseconds(),
		result.FirstByteMS,
		result.BytesOut,
		result.Stream,
		result.ErrorType,
		limitString(result.ErrorMessage, 200),
		result.UpstreamRequestID,
		limitString(r.UserAgent(), 120),
	)
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `{"error":{"message":`+quoteJSON(message)+`,"type":"dshare_error"}}`)
}

func writeSSEError(w http.ResponseWriter, message string, done bool) {
	_, _ = io.WriteString(w, `data: {"error":{"message":`+quoteJSON(message)+`,"type":"dshare_error"}}`+"\n\n")
	if done {
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func secondsCeil(d time.Duration) string {
	n := int64(d / time.Second)
	if d%time.Second != 0 {
		n++
	}
	if n < 1 {
		n = 1
	}
	return stringInt(n)
}

func secondsUntilNextUTCDay() string {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return secondsCeil(next.Sub(now))
}

func stringInt(n int64) string {
	var buf [20]byte
	i := len(buf)
	if n == 0 {
		return "0"
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
		raw := strings.TrimSpace(r.Header.Get(header))
		if raw == "" {
			continue
		}
		if idx := strings.Index(raw, ","); idx >= 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
		if ip := net.ParseIP(raw); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return limitString(r.RemoteAddr, 120)
}

func limitString(value string, max int) string {
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}
