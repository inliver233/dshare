package ds2api

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	baseURL   string
	adminKey  string
	autoProxy AutoProxyConfig
	http      *http.Client

	mu        sync.Mutex
	token     string
	tokenExp  time.Time
	loginOnce singleflight
}

type AutoProxyConfig struct {
	Enabled          bool
	Type             string
	Host             string
	Port             int
	UsernameTemplate string
	Password         string
	NameTemplate     string
}

type proxyConfig struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type TestResult struct {
	Account        string `json:"account"`
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	ResponseTimeMS int    `json:"response_time"`
}

type ImportResult struct {
	Account        string `json:"account"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	ResponseTimeMS int    `json:"response_time_ms,omitempty"`
}

func New(baseURL, adminKey string, timeout time.Duration, autoProxy AutoProxyConfig) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		adminKey:  adminKey,
		autoProxy: normalizeAutoProxy(autoProxy),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) SetConfig(baseURL, adminKey string, autoProxy AutoProxyConfig) {
	c.mu.Lock()
	c.baseURL = strings.TrimRight(baseURL, "/")
	c.adminKey = adminKey
	c.autoProxy = normalizeAutoProxy(autoProxy)
	c.token = ""
	c.tokenExp = time.Time{}
	c.mu.Unlock()
}

func (c *Client) Enabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL != "" && c.adminKey != ""
}

func (c *Client) AddAndValidateAccount(ctx context.Context, account, password string) ImportResult {
	account = strings.TrimSpace(account)
	password = strings.TrimSpace(password)
	if account == "" || password == "" {
		return ImportResult{Account: account, Status: "invalid", Message: "账号或密码为空"}
	}
	if !c.Enabled() {
		return ImportResult{Account: account, Status: "error", Message: "ds2api 未配置"}
	}
	if err := c.importAccount(ctx, account, password); err != nil {
		if isAlreadyExists(err.Error()) {
			return ImportResult{Account: account, Status: "duplicate", Message: "ds2api 中已存在该账号"}
		}
		return ImportResult{Account: account, Status: "error", Message: "添加到 ds2api 失败: " + err.Error()}
	}
	test, err := c.testAccount(ctx, account)
	if err != nil {
		_ = c.deleteAccount(context.WithoutCancel(ctx), account)
		return ImportResult{Account: account, Status: "invalid", Message: "测试失败: " + err.Error()}
	}
	if !test.Success {
		_ = c.deleteAccount(context.WithoutCancel(ctx), account)
		message := strings.TrimSpace(test.Message)
		if message == "" {
			message = "ds2api 账号测试未通过"
		}
		return ImportResult{
			Account:        account,
			Status:         "invalid",
			Message:        message,
			ResponseTimeMS: test.ResponseTimeMS,
		}
	}
	message := strings.TrimSpace(test.Message)
	if message == "" {
		message = "验证通过"
	}
	return ImportResult{
		Account:        account,
		Status:         "valid",
		Message:        message,
		ResponseTimeMS: test.ResponseTimeMS,
	}
}

func (c *Client) DeleteAccount(ctx context.Context, account string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return errors.New("账号为空")
	}
	if !c.Enabled() {
		return errors.New("ds2api 未配置")
	}
	return c.deleteAccount(ctx, account)
}

func (c *Client) importAccount(ctx context.Context, account, password string) error {
	if strings.Contains(account, "@") {
		return c.bulkImportAccount(ctx, account, password)
	}
	return c.addAccount(ctx, account, password)
}

func (c *Client) login(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-time.Minute)) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	return c.loginOnce.Do(func() (string, error) {
		c.mu.Lock()
		if c.token != "" && time.Now().Before(c.tokenExp.Add(-time.Minute)) {
			token := c.token
			c.mu.Unlock()
			return token, nil
		}
		c.mu.Unlock()

		var out struct {
			Success   bool   `json:"success"`
			Token     string `json:"token"`
			ExpiresIn int    `json:"expires_in"`
			Detail    string `json:"detail"`
		}
		err := c.doJSON(ctx, http.MethodPost, "/admin/login", "", map[string]any{
			"admin_key":    c.adminKey,
			"expire_hours": 24,
		}, &out)
		if err != nil {
			return "", err
		}
		if out.Token == "" {
			if out.Detail == "" {
				out.Detail = "ds2api 未返回管理 token"
			}
			return "", errors.New(out.Detail)
		}
		expiresIn := out.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 24 * 3600
		}
		c.mu.Lock()
		c.token = out.Token
		c.tokenExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
		c.mu.Unlock()
		return out.Token, nil
	})
}

func (c *Client) bulkImportAccount(ctx context.Context, account, password string) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"lines":      account + ":" + password,
		"auto_proxy": c.bulkAutoProxyBody(),
	}
	var out struct {
		Success          bool                `json:"success"`
		ImportedAccounts int                 `json:"imported_accounts"`
		ImportedProxies  int                 `json:"imported_proxies"`
		Skipped          []map[string]string `json:"skipped"`
		Errors           []map[string]string `json:"errors"`
		Detail           string              `json:"detail"`
	}
	err = c.doJSON(ctx, http.MethodPost, "/admin/accounts/bulk-import", token, body, &out)
	if unauthorized(err) {
		c.invalidateToken()
		token, err = c.login(ctx)
		if err != nil {
			return err
		}
		err = c.doJSON(ctx, http.MethodPost, "/admin/accounts/bulk-import", token, body, &out)
	}
	if err != nil {
		return err
	}
	if out.ImportedAccounts > 0 {
		return nil
	}
	if len(out.Skipped) > 0 {
		reason := strings.TrimSpace(out.Skipped[0]["reason"])
		if reason == "" {
			reason = "ds2api bulk import skipped account"
		}
		return errors.New(reason)
	}
	if len(out.Errors) > 0 {
		message := strings.TrimSpace(out.Errors[0]["error"])
		if message == "" {
			message = "ds2api bulk import failed"
		}
		return errors.New(message)
	}
	if out.Detail != "" {
		return errors.New(out.Detail)
	}
	return errors.New("ds2api bulk import did not import account")
}

func (c *Client) addAccount(ctx context.Context, account, password string) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"password": password,
	}
	if proxyCfg, ok, err := c.proxyForAccount(account); err != nil {
		return err
	} else if ok {
		if err := c.ensureProxy(ctx, token, proxyCfg); unauthorized(err) {
			c.invalidateToken()
			token, err = c.login(ctx)
			if err != nil {
				return err
			}
			if err := c.ensureProxy(ctx, token, proxyCfg); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		body["proxy_id"] = proxyCfg.ID
	}
	if strings.Contains(account, "@") {
		body["email"] = account
	} else {
		body["mobile"] = account
	}
	var out map[string]any
	err = c.doJSON(ctx, http.MethodPost, "/admin/accounts", token, body, &out)
	if unauthorized(err) {
		c.invalidateToken()
		token, err = c.login(ctx)
		if err != nil {
			return err
		}
		err = c.doJSON(ctx, http.MethodPost, "/admin/accounts", token, body, &out)
	}
	return err
}

func (c *Client) ensureProxy(ctx context.Context, token string, proxyCfg proxyConfig) error {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodPost, "/admin/proxies", token, proxyCfg, &out)
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "duplicate proxy id") || isAlreadyExists(message) {
		return nil
	}
	return err
}

func (c *Client) testAccount(ctx context.Context, account string) (TestResult, error) {
	token, err := c.login(ctx)
	if err != nil {
		return TestResult{}, err
	}
	var out TestResult
	err = c.doJSON(ctx, http.MethodPost, "/admin/accounts/test", token, map[string]any{
		"identifier": account,
	}, &out)
	if unauthorized(err) {
		c.invalidateToken()
		token, err = c.login(ctx)
		if err != nil {
			return TestResult{}, err
		}
		err = c.doJSON(ctx, http.MethodPost, "/admin/accounts/test", token, map[string]any{
			"identifier": account,
		}, &out)
	}
	return out, err
}

func (c *Client) deleteAccount(ctx context.Context, account string) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}
	path := "/admin/accounts/" + url.PathEscape(account)
	var out map[string]any
	err = c.doJSON(ctx, http.MethodDelete, path, token, nil, &out)
	if unauthorized(err) {
		c.invalidateToken()
		token, err = c.login(ctx)
		if err != nil {
			return err
		}
		err = c.doJSON(ctx, http.MethodDelete, path, token, nil, &out)
	}
	if notFound(err) {
		return nil
	}
	return err
}

func (c *Client) doJSON(ctx context.Context, method, path, bearer string, in any, out any) error {
	c.mu.Lock()
	baseURL := c.baseURL
	c.mu.Unlock()
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpError{Status: resp.StatusCode, Body: string(data)}
	}
	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode ds2api response: %w", err)
	}
	return nil
}

func (c *Client) bulkAutoProxyBody() map[string]any {
	c.mu.Lock()
	autoProxy := c.autoProxy
	c.mu.Unlock()
	autoProxy = normalizeAutoProxy(autoProxy)
	if !autoProxy.Enabled {
		return map[string]any{"enabled": false}
	}
	return map[string]any{
		"enabled":           true,
		"type":              autoProxy.Type,
		"host":              autoProxy.Host,
		"port":              autoProxy.Port,
		"username_template": autoProxy.UsernameTemplate,
		"password":          autoProxy.Password,
		"name_template":     autoProxy.NameTemplate,
	}
}

func (c *Client) proxyForAccount(account string) (proxyConfig, bool, error) {
	c.mu.Lock()
	autoProxy := c.autoProxy
	c.mu.Unlock()
	autoProxy = normalizeAutoProxy(autoProxy)
	if !autoProxy.Enabled {
		return proxyConfig{}, false, nil
	}
	if autoProxy.Host == "" || autoProxy.Port <= 0 {
		return proxyConfig{}, false, errors.New("ds2api 自动代理已启用但 host/port 未配置")
	}
	local := accountLocalPart(account)
	if local == "" {
		return proxyConfig{}, false, errors.New("无法为该账号生成代理用户名")
	}
	proxyCfg := proxyConfig{
		Name:     strings.ReplaceAll(autoProxy.NameTemplate, "{local}", local),
		Type:     autoProxy.Type,
		Host:     autoProxy.Host,
		Port:     autoProxy.Port,
		Username: strings.ReplaceAll(autoProxy.UsernameTemplate, "{local}", local),
		Password: autoProxy.Password,
	}
	proxyCfg.ID = stableProxyID(proxyCfg)
	return proxyCfg, true, nil
}

func normalizeAutoProxy(in AutoProxyConfig) AutoProxyConfig {
	out := AutoProxyConfig{
		Enabled:          in.Enabled,
		Type:             strings.ToLower(strings.TrimSpace(in.Type)),
		Host:             strings.TrimSpace(in.Host),
		Port:             in.Port,
		UsernameTemplate: strings.TrimSpace(in.UsernameTemplate),
		Password:         strings.TrimSpace(in.Password),
		NameTemplate:     strings.TrimSpace(in.NameTemplate),
	}
	if out.Type == "" {
		out.Type = "socks5"
	}
	if out.UsernameTemplate == "" {
		out.UsernameTemplate = "Default.{local}"
	}
	if out.NameTemplate == "" {
		out.NameTemplate = "resin-{local}"
	}
	return out
}

func accountLocalPart(account string) string {
	account = strings.TrimSpace(account)
	if account == "" {
		return ""
	}
	if at := strings.Index(account, "@"); at > 0 {
		return strings.TrimSpace(account[:at])
	}
	var b strings.Builder
	for _, r := range account {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func stableProxyID(proxyCfg proxyConfig) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(proxyCfg.Type)) + "|" + strings.ToLower(strings.TrimSpace(proxyCfg.Host)) + "|" + fmt.Sprintf("%d", proxyCfg.Port) + "|" + strings.TrimSpace(proxyCfg.Username)))
	return "proxy_" + hex.EncodeToString(sum[:6])
}

func (c *Client) invalidateToken() {
	c.mu.Lock()
	c.token = ""
	c.tokenExp = time.Time{}
	c.mu.Unlock()
}

type httpError struct {
	Status int
	Body   string
}

func (e httpError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(body), &parsed) == nil {
		for _, key := range []string{"detail", "message", "error"} {
			if v, ok := parsed[key]; ok {
				return fmt.Sprintf("HTTP %d: %v", e.Status, v)
			}
		}
	}
	if len(body) > 400 {
		body = body[:400]
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, body)
}

func unauthorized(err error) bool {
	var he httpError
	return errors.As(err, &he) && he.Status == http.StatusUnauthorized
}

func notFound(err error) bool {
	var he httpError
	return errors.As(err, &he) && he.Status == http.StatusNotFound
}

func isAlreadyExists(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "已存在") ||
		strings.Contains(message, "already exists") ||
		strings.Contains(message, "exists")
}

type singleflight struct {
	mu      sync.Mutex
	running bool
	done    chan struct{}
	value   string
	err     error
}

func (s *singleflight) Do(fn func() (string, error)) (string, error) {
	s.mu.Lock()
	if s.running {
		done := s.done
		s.mu.Unlock()
		<-done
		return s.value, s.err
	}
	s.running = true
	s.done = make(chan struct{})
	s.mu.Unlock()

	value, err := fn()

	s.mu.Lock()
	s.value = value
	s.err = err
	s.running = false
	close(s.done)
	s.mu.Unlock()
	return value, err
}
