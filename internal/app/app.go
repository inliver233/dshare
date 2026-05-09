package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	discordauth "dshare/internal/auth"
	"dshare/internal/config"
	"dshare/internal/ds2api"
	"dshare/internal/httpx"
	"dshare/internal/proxy"
	"dshare/internal/ratelimit"
	"dshare/internal/security"
	"dshare/internal/store"
)

type App struct {
	cfg           config.Config
	store         *store.Store
	ds2           *ds2api.Client
	proxy         *proxy.Handler
	limiter       *ratelimit.Limiter
	discord       discordauth.DiscordClient
	oauthStatesMu sync.Mutex
	oauthStates   map[string]time.Time
}

func New(cfg config.Config) (*App, error) {
	s, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	settings, err := s.ServiceSettings(context.Background())
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	cfg = applyServiceSettings(cfg, settings)
	limiter := ratelimit.New()
	app := &App{
		cfg:         cfg,
		store:       s,
		ds2:         ds2api.New(cfg.DS2APIBaseURL, cfg.DS2APIAdminKey, cfg.HTTPClientTimeout, toDS2AutoProxy(cfg.DS2APIAutoProxy)),
		limiter:     limiter,
		oauthStates: map[string]time.Time{},
	}
	app.discord = discordauth.DiscordClient{
		ClientID:     cfg.DiscordClientID,
		ClientSecret: cfg.DiscordClientSecret,
		RedirectURL:  cfg.DiscordRedirectURL,
		HTTPClient:   &http.Client{Timeout: 15 * time.Second},
	}
	app.proxy = proxy.New(cfg, s, limiter)
	return app, nil
}

func (a *App) Close() {
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(a.cors)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(api chi.Router) {
		api.Get("/config", a.handlePublicConfig)
		api.Get("/rank", a.handlePublicRank)
		api.Get("/auth/discord/start", a.handleDiscordStart)
		api.Get("/auth/discord/callback", a.handleDiscordCallback)
		api.Post("/auth/admin/login", a.handleAdminLogin)
		api.Post("/auth/logout", a.handleLogout)

		api.Group(func(authed chi.Router) {
			authed.Use(a.requireSession)
			authed.Get("/me", a.handleMe)
			authed.Get("/keys", a.handleListKeys)
			authed.Post("/keys", a.handleCreateKey)
			authed.Delete("/keys/{id}", a.handleRevokeKey)
			authed.Post("/ds2api/import", a.handleDS2Import)
			authed.Get("/contributions", a.handleContributions)
			authed.Delete("/contributions/{id}", a.handleDeleteContribution)
		})

		api.Group(func(admin chi.Router) {
			admin.Use(a.requireSession)
			admin.Use(a.requireAdmin)
			admin.Get("/admin/stats", a.handleAdminStats)
			admin.Get("/admin/users", a.handleAdminUsers)
			admin.Put("/admin/users/{id}/limits", a.handleAdminUpdateLimits)
			admin.Get("/admin/service-config", a.handleGetServiceConfig)
			admin.Put("/admin/service-config", a.handleUpdateServiceConfig)
		})
	})

	a.mountProxyRoutes(r)

	a.mountStatic(r)
	return r
}

func (a *App) handlePublicConfig(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"discord_enabled": a.cfg.DiscordClientID != "" && a.cfg.DiscordClientSecret != "",
		"ds2api_enabled":  a.ds2.Enabled(),
		"new_api_enabled": a.cfg.NewAPIBaseURL != "" && a.cfg.NewAPIKey != "",
		"base_url":        a.cfg.AppBaseURL,
	})
}

func (a *App) handlePublicRank(w http.ResponseWriter, r *http.Request) {
	rank, err := a.store.PublicRank(r.Context(), store.RankQuery{
		Board:  r.URL.Query().Get("board"),
		Period: r.URL.Query().Get("period"),
		Search: r.URL.Query().Get("q"),
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "加载排行榜失败")
		return
	}
	httpx.JSON(w, http.StatusOK, rank)
}

func (a *App) handleDiscordStart(w http.ResponseWriter, r *http.Request) {
	if a.cfg.DiscordClientID == "" || a.cfg.DiscordClientSecret == "" {
		httpx.Error(w, http.StatusServiceUnavailable, "discord oauth is not configured")
		return
	}
	state, err := security.RandomToken(24)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.oauthStatesMu.Lock()
	a.oauthStates[state] = time.Now().Add(10 * time.Minute)
	a.oauthStatesMu.Unlock()
	http.Redirect(w, r, a.discord.AuthURL(state), http.StatusFound)
}

func (a *App) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if !a.consumeOAuthState(state) {
		httpx.Error(w, http.StatusForbidden, "invalid oauth state")
		return
	}
	code := r.URL.Query().Get("code")
	discordUser, err := a.discord.ExchangeUser(r.Context(), code)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	defaults := store.UserLimits{
		RequestsPerMinute:     a.cfg.DefaultRequestsPerMin,
		RequestsPerDay:        a.cfg.DefaultRequestsPerDay,
		MaxConcurrentRequests: a.cfg.DefaultMaxConcurrent,
	}
	user, err := a.store.UpsertDiscordUser(r.Context(), discordUser.ID, discordUser.Username, discordUser.GlobalName, discordUser.Avatar, defaults)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a.cfg.AdminDiscordIDs[user.DiscordID] {
		if err := a.store.EnsureAdminByDiscordID(r.Context(), user.DiscordID); err != nil {
			log.Printf("promote admin %s: %v", user.DiscordID, err)
		}
		user, _ = a.store.GetUserByID(r.Context(), user.ID)
	}
	token, expires, err := a.store.CreateSession(r.Context(), user.ID, time.Duration(a.cfg.SessionDays)*24*time.Hour)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setSessionCookie(w, token, expires)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	usernameOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(a.cfg.AdminUsername)) == 1
	passwordOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.cfg.AdminPassword)) == 1
	if !usernameOK || !passwordOK {
		httpx.Error(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}
	defaults := store.UserLimits{
		RequestsPerMinute:     a.cfg.DefaultRequestsPerMin,
		RequestsPerDay:        a.cfg.DefaultRequestsPerDay,
		MaxConcurrentRequests: a.cfg.DefaultMaxConcurrent,
	}
	user, err := a.store.UpsertDiscordUser(r.Context(), "admin-local", a.cfg.AdminUsername, "Local Admin", "", defaults)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.EnsureAdminByDiscordID(r.Context(), "admin-local"); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, expires, err := a.store.CreateSession(r.Context(), user.ID, time.Duration(a.cfg.SessionDays)*24*time.Hour)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setSessionCookie(w, token, expires)
	httpx.JSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		_ = a.store.DeleteSession(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{Name: "dshare_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.CookieSecure})
	httpx.JSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stats, _ := a.store.UserStatsWithLimits(r.Context(), user.ID, user.RequestsPerDay)
	keys, _ := a.store.ListAPIKeys(r.Context(), user.ID)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":           user,
		"stats":          stats,
		"keys":           keys,
		"proxy_base_url": strings.TrimRight(a.cfg.AppBaseURL, "/"),
	})
}

func (a *App) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListAPIKeys(r.Context(), currentUser(r).ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": keys})
}

func (a *App) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	key, err := a.store.CreateAPIKey(r.Context(), currentUser(r).ID, req.Name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, key)
}

func (a *App) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := a.store.RevokeAPIKey(r.Context(), currentUser(r).ID, id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpx.Error(w, status, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handleDS2Import(w http.ResponseWriter, r *http.Request) {
	if !a.ds2.Enabled() {
		httpx.Error(w, http.StatusServiceUnavailable, "ds2api is not configured")
		return
	}
	var req struct {
		Lines string `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	entries := parseAccountLines(req.Lines)
	if len(entries) == 0 {
		httpx.Error(w, http.StatusBadRequest, "未解析到任何有效账号，格式为 账号:密码 一行一个，中文冒号会自动兼容")
		return
	}
	user := currentUser(r)
	type job struct {
		Account  string
		Password string
	}
	jobs := make(chan job)
	results := make(chan ds2api.ImportResult)
	workerCount := a.cfg.DS2APIValidateWorkers
	if workerCount > len(entries) {
		workerCount = len(entries)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				results <- a.ds2.AddAndValidateAccount(r.Context(), entry.Account, entry.Password)
			}
		}()
	}
	go func() {
		for _, entry := range entries {
			jobs <- job{Account: entry.Account, Password: entry.Password}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	out := []ds2api.ImportResult{}
	valid := 0
	invalid := 0
	duplicate := 0
	audit := contributionAudit(r)
	for result := range results {
		out = append(out, result)
		switch result.Status {
		case "valid":
			inserted, err := a.store.RecordContributionWithLimitBonus(r.Context(), store.Contribution{
				UserID:       user.ID,
				Account:      result.Account,
				Status:       "valid",
				Message:      result.Message,
				ResponseTime: result.ResponseTimeMS,
				IP:           audit.IP,
				UserAgent:    audit.UserAgent,
				Referer:      audit.Referer,
				RequestID:    audit.RequestID,
			}, 1, 100)
			if err != nil {
				result.Status = "error"
				result.Message = "记录贡献失败: " + err.Error()
				invalid++
				break
			}
			if inserted {
				valid++
			} else {
				result.Status = "duplicate"
				result.Message = "该有效账号此前已被记录"
				duplicate++
			}
		case "duplicate":
			duplicate++
		default:
			invalid++
			_, _ = a.store.RecordContribution(r.Context(), store.Contribution{
				UserID:       user.ID,
				Account:      result.Account,
				Status:       result.Status,
				Message:      result.Message,
				ResponseTime: result.ResponseTimeMS,
				IP:           audit.IP,
				UserAgent:    audit.UserAgent,
				Referer:      audit.Referer,
				RequestID:    audit.RequestID,
			})
		}
		out[len(out)-1] = result
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Account < out[j].Account })
	httpx.JSON(w, http.StatusOK, map[string]any{
		"total":     len(entries),
		"valid":     valid,
		"invalid":   invalid,
		"duplicate": duplicate,
		"results":   out,
	})
}

func (a *App) handleContributions(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListContributions(r.Context(), currentUser(r).ID, 100)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) handleDeleteContribution(w http.ResponseWriter, r *http.Request) {
	if !a.ds2.Enabled() {
		httpx.Error(w, http.StatusServiceUnavailable, "ds2api is not configured")
		return
	}
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid contribution id")
		return
	}
	user := currentUser(r)
	contribution, err := a.store.GetContributionForUser(r.Context(), user.ID, id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpx.Error(w, status, err.Error())
		return
	}
	if contribution.Status != "valid" {
		httpx.Error(w, http.StatusBadRequest, "只有有效贡献可以删除")
		return
	}
	if err := a.ds2.DeleteAccount(r.Context(), contribution.Account); err != nil {
		httpx.Error(w, http.StatusBadGateway, "从 ds2api 删除账号失败: "+err.Error())
		return
	}
	if err := a.store.MarkContributionDeletedWithLimitPenalty(r.Context(), user.ID, id, "用户已删除 ds2api 账号", -1, -100); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpx.Error(w, status, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.DashboardStats(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, stats)
}

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	users, total, err := a.store.ListUsers(r.Context(), r.URL.Query().Get("q"), limit, (page-1)*limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": users, "total": total, "page": page, "limit": limit})
}

func (a *App) handleAdminUpdateLimits(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req store.UserLimits
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.RequestsPerMinute < 0 || req.RequestsPerDay < 0 || req.MaxConcurrentRequests < 0 {
		httpx.Error(w, http.StatusBadRequest, "limits must be non-negative")
		return
	}
	user, err := a.store.UpdateUserLimits(r.Context(), id, req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpx.Error(w, status, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

func (a *App) handleGetServiceConfig(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.publicServiceConfig())
}

func (a *App) handleUpdateServiceConfig(w http.ResponseWriter, r *http.Request) {
	var req store.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	current := a.cfg
	next := current
	if strings.TrimSpace(req.NewAPIBaseURL) != "" {
		next.NewAPIBaseURL = strings.TrimRight(strings.TrimSpace(req.NewAPIBaseURL), "/")
	}
	if strings.TrimSpace(req.NewAPIKey) != "" {
		next.NewAPIKey = strings.TrimSpace(req.NewAPIKey)
	}
	if strings.TrimSpace(req.DS2APIBaseURL) != "" {
		next.DS2APIBaseURL = strings.TrimRight(strings.TrimSpace(req.DS2APIBaseURL), "/")
	}
	if strings.TrimSpace(req.DS2APIAdminKey) != "" {
		next.DS2APIAdminKey = strings.TrimSpace(req.DS2APIAdminKey)
	}
	if serviceAutoProxyProvided(req) {
		next.DS2APIAutoProxy = config.NormalizeDS2APIAutoProxy(config.DS2APIAutoProxyConfig{
			Enabled:          req.DS2APIAutoProxy.Enabled,
			Type:             req.DS2APIAutoProxy.Type,
			Host:             req.DS2APIAutoProxy.Host,
			Port:             req.DS2APIAutoProxy.Port,
			UsernameTemplate: req.DS2APIAutoProxy.UsernameTemplate,
			Password:         current.DS2APIAutoProxy.Password,
			NameTemplate:     req.DS2APIAutoProxy.NameTemplate,
		})
		if strings.TrimSpace(req.DS2APIAutoProxy.Password) != "" {
			next.DS2APIAutoProxy.Password = strings.TrimSpace(req.DS2APIAutoProxy.Password)
		}
	}
	next.DiscordClientID = strings.TrimSpace(req.DiscordClientID)
	if strings.TrimSpace(req.DiscordClientSecret) != "" {
		next.DiscordClientSecret = strings.TrimSpace(req.DiscordClientSecret)
	}
	if strings.TrimSpace(req.AppBaseURL) != "" {
		next.AppBaseURL = strings.TrimRight(strings.TrimSpace(req.AppBaseURL), "/")
	}
	if strings.TrimSpace(req.DiscordRedirectURL) != "" {
		next.DiscordRedirectURL = strings.TrimSpace(req.DiscordRedirectURL)
	} else if next.AppBaseURL != "" {
		next.DiscordRedirectURL = next.AppBaseURL + "/api/auth/discord/callback"
	}

	if err := a.store.UpdateServiceSettings(r.Context(), map[string]string{
		"new_api_base_url":                    next.NewAPIBaseURL,
		"new_api_key":                         next.NewAPIKey,
		"ds2api_base_url":                     next.DS2APIBaseURL,
		"ds2api_admin_key":                    next.DS2APIAdminKey,
		"ds2api_auto_proxy_enabled":           boolSetting(next.DS2APIAutoProxy.Enabled),
		"ds2api_auto_proxy_type":              next.DS2APIAutoProxy.Type,
		"ds2api_auto_proxy_host":              next.DS2APIAutoProxy.Host,
		"ds2api_auto_proxy_port":              fmt.Sprintf("%d", next.DS2APIAutoProxy.Port),
		"ds2api_auto_proxy_username_template": next.DS2APIAutoProxy.UsernameTemplate,
		"ds2api_auto_proxy_password":          next.DS2APIAutoProxy.Password,
		"ds2api_auto_proxy_name_template":     next.DS2APIAutoProxy.NameTemplate,
		"discord_client_id":                   next.DiscordClientID,
		"discord_client_secret":               next.DiscordClientSecret,
		"discord_redirect_url":                next.DiscordRedirectURL,
		"app_base_url":                        next.AppBaseURL,
	}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.applyRuntimeConfig(next)
	httpx.JSON(w, http.StatusOK, a.publicServiceConfig())
}

func (a *App) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		if token == "" {
			httpx.Error(w, http.StatusUnauthorized, "login required")
			return
		}
		user, err := a.store.GetSessionUser(r.Context(), token)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "login required")
			return
		}
		if a.cfg.AdminDiscordIDs[user.DiscordID] && user.Role != "admin" {
			if err := a.store.EnsureAdminByDiscordID(r.Context(), user.DiscordID); err == nil {
				user, _ = a.store.GetUserByID(r.Context(), user.ID)
			}
		}
		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "admin" {
			httpx.Error(w, http.StatusForbidden, "admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) mountProxyRoutes(r chi.Router) {
	for _, pattern := range []string{
		"/v1/*",
		"/v1beta/*",
		"/mj/*",
		"/{mode}/mj/*",
		"/suno/*",
		"/pg/*",
	} {
		r.Handle(pattern, a.proxy)
	}
}

func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, x-api-key, x-goog-api-key, anthropic-version")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) mountStatic(r chi.Router) {
	staticDir := a.cfg.StaticDir
	if _, err := os.Stat(staticDir); err != nil {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			httpx.JSON(w, http.StatusOK, map[string]string{"service": "dshare"})
		})
		return
	}
	fileServer := http.FileServer(http.Dir(staticDir))
	r.Handle("/assets/*", fileServer)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(index); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
		}
		http.ServeFile(w, r, index)
	})
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "dshare_session",
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.cfg.CookieSecure,
	})
}

func (a *App) applyRuntimeConfig(cfg config.Config) {
	a.cfg = cfg
	a.ds2.SetConfig(cfg.DS2APIBaseURL, cfg.DS2APIAdminKey, toDS2AutoProxy(cfg.DS2APIAutoProxy))
	a.proxy.SetConfig(cfg)
	a.discord.ClientID = cfg.DiscordClientID
	a.discord.ClientSecret = cfg.DiscordClientSecret
	a.discord.RedirectURL = cfg.DiscordRedirectURL
}

func (a *App) publicServiceConfig() store.ServiceConfig {
	out := store.ServiceConfig{
		NewAPIBaseURL:         a.cfg.NewAPIBaseURL,
		NewAPIKeyPreview:      security.MaskSecret(a.cfg.NewAPIKey),
		DS2APIBaseURL:         a.cfg.DS2APIBaseURL,
		DS2APIAdminKeyPreview: security.MaskSecret(a.cfg.DS2APIAdminKey),
		DiscordClientID:       a.cfg.DiscordClientID,
		DiscordSecretPreview:  security.MaskSecret(a.cfg.DiscordClientSecret),
		DiscordRedirectURL:    a.cfg.DiscordRedirectURL,
		AppBaseURL:            a.cfg.AppBaseURL,
	}
	out.DS2APIAutoProxy.Enabled = a.cfg.DS2APIAutoProxy.Enabled
	out.DS2APIAutoProxy.Type = a.cfg.DS2APIAutoProxy.Type
	out.DS2APIAutoProxy.Host = a.cfg.DS2APIAutoProxy.Host
	out.DS2APIAutoProxy.Port = a.cfg.DS2APIAutoProxy.Port
	out.DS2APIAutoProxy.UsernameTemplate = a.cfg.DS2APIAutoProxy.UsernameTemplate
	out.DS2APIAutoProxy.PasswordPreview = security.MaskSecret(a.cfg.DS2APIAutoProxy.Password)
	out.DS2APIAutoProxy.NameTemplate = a.cfg.DS2APIAutoProxy.NameTemplate
	return out
}

func applyServiceSettings(cfg config.Config, values map[string]string) config.Config {
	if v := strings.TrimSpace(values["new_api_base_url"]); v != "" {
		cfg.NewAPIBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(values["new_api_key"]); v != "" {
		cfg.NewAPIKey = v
	}
	if v := strings.TrimSpace(values["ds2api_base_url"]); v != "" {
		cfg.DS2APIBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(values["ds2api_admin_key"]); v != "" {
		cfg.DS2APIAdminKey = v
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_enabled"]); v != "" {
		cfg.DS2APIAutoProxy.Enabled = settingBool(v, cfg.DS2APIAutoProxy.Enabled)
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_type"]); v != "" {
		cfg.DS2APIAutoProxy.Type = v
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_host"]); v != "" {
		cfg.DS2APIAutoProxy.Host = v
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_port"]); v != "" {
		cfg.DS2APIAutoProxy.Port = settingInt(v, cfg.DS2APIAutoProxy.Port)
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_username_template"]); v != "" {
		cfg.DS2APIAutoProxy.UsernameTemplate = v
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_password"]); v != "" {
		cfg.DS2APIAutoProxy.Password = v
	}
	if v := strings.TrimSpace(values["ds2api_auto_proxy_name_template"]); v != "" {
		cfg.DS2APIAutoProxy.NameTemplate = v
	}
	cfg.DS2APIAutoProxy = config.NormalizeDS2APIAutoProxy(cfg.DS2APIAutoProxy)
	if v := strings.TrimSpace(values["discord_client_id"]); v != "" {
		cfg.DiscordClientID = v
	}
	if v := strings.TrimSpace(values["discord_client_secret"]); v != "" {
		cfg.DiscordClientSecret = v
	}
	if v := strings.TrimSpace(values["app_base_url"]); v != "" {
		cfg.AppBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(values["discord_redirect_url"]); v != "" {
		cfg.DiscordRedirectURL = v
	} else if cfg.DiscordRedirectURL == "" && cfg.AppBaseURL != "" {
		cfg.DiscordRedirectURL = cfg.AppBaseURL + "/api/auth/discord/callback"
	}
	return cfg
}

func serviceAutoProxyProvided(req store.ServiceConfig) bool {
	p := req.DS2APIAutoProxy
	return p.Enabled ||
		strings.TrimSpace(p.Type) != "" ||
		strings.TrimSpace(p.Host) != "" ||
		p.Port != 0 ||
		strings.TrimSpace(p.UsernameTemplate) != "" ||
		strings.TrimSpace(p.Password) != "" ||
		strings.TrimSpace(p.NameTemplate) != ""
}

func toDS2AutoProxy(in config.DS2APIAutoProxyConfig) ds2api.AutoProxyConfig {
	in = config.NormalizeDS2APIAutoProxy(in)
	return ds2api.AutoProxyConfig{
		Enabled:          in.Enabled,
		Type:             in.Type,
		Host:             in.Host,
		Port:             in.Port,
		UsernameTemplate: in.UsernameTemplate,
		Password:         in.Password,
		NameTemplate:     in.NameTemplate,
	}
}

func boolSetting(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func settingBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func settingInt(raw string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &n); err != nil {
		return fallback
	}
	return n
}

func (a *App) consumeOAuthState(state string) bool {
	if state == "" {
		return false
	}
	now := time.Now()
	a.oauthStatesMu.Lock()
	defer a.oauthStatesMu.Unlock()
	for key, expires := range a.oauthStates {
		if now.After(expires) {
			delete(a.oauthStates, key)
		}
	}
	expires, ok := a.oauthStates[state]
	if !ok || now.After(expires) {
		return false
	}
	delete(a.oauthStates, state)
	return true
}

type userContextKey struct{}

func currentUser(r *http.Request) *store.User {
	user, _ := r.Context().Value(userContextKey{}).(*store.User)
	return user
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie("dshare_session")
	if err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

type accountEntry struct {
	Account  string
	Password string
}

func parseAccountLines(lines string) []accountEntry {
	out := []accountEntry{}
	seen := map[string]bool{}
	for _, raw := range strings.Split(lines, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.ReplaceAll(line, "：", ":")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var account, password string
		if idx := strings.Index(line, ":"); idx > 0 {
			account = strings.TrimSpace(line[:idx])
			password = strings.TrimSpace(line[idx+1:])
		} else if idx := strings.Index(line, "\t"); idx > 0 {
			account = strings.TrimSpace(line[:idx])
			password = strings.TrimSpace(line[idx+1:])
		} else {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				account = strings.TrimSpace(fields[0])
				password = strings.TrimSpace(strings.Join(fields[1:], " "))
			}
		}
		if account == "" || password == "" {
			continue
		}
		key := strings.ToLower(account)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, accountEntry{Account: account, Password: password})
	}
	return out
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		return fallback
	}
	return n
}

func parseID(raw string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &id); err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

type contributionAuditInfo struct {
	IP        string
	UserAgent string
	Referer   string
	RequestID string
}

func contributionAudit(r *http.Request) contributionAuditInfo {
	return contributionAuditInfo{
		IP:        limitString(clientIP(r), 120),
		UserAgent: limitString(r.UserAgent(), 500),
		Referer:   limitString(r.Referer(), 500),
		RequestID: limitString(r.Header.Get("X-Request-Id"), 120),
	}
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Real-IP", "X-Forwarded-For"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if value != "" {
			return value
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func limitString(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
