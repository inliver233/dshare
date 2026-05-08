package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DiscordClient struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	HTTPClient   *http.Client
}

type DiscordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
}

func (c DiscordClient) AuthURL(state string) string {
	values := url.Values{}
	values.Set("client_id", c.ClientID)
	values.Set("redirect_uri", c.RedirectURL)
	values.Set("response_type", "code")
	values.Set("scope", "identify")
	values.Set("state", state)
	return "https://discord.com/oauth2/authorize?" + values.Encode()
}

func (c DiscordClient) ExchangeUser(ctx context.Context, code string) (*DiscordUser, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("missing discord code")
	}
	if c.ClientID == "" || c.ClientSecret == "" || c.RedirectURL == "" {
		return nil, errors.New("discord oauth is not configured")
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	values := url.Values{}
	values.Set("client_id", c.ClientID)
	values.Set("client_secret", c.ClientSecret)
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", c.RedirectURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://discord.com/api/v10/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discord token exchange failed: HTTP %d %s", resp.StatusCode, trimBody(data))
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("discord did not return an access token")
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discord user lookup failed: HTTP %d %s", resp.StatusCode, trimBody(data))
	}
	var user DiscordUser
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, errors.New("discord user response is missing id")
	}
	return &user, nil
}

func trimBody(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) > 300 {
		data = data[:300]
	}
	return string(data)
}
