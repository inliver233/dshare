package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"dshare/internal/security"
)

type Store struct {
	db                  *sql.DB
	apiKeyEncryptSecret string
}

type User struct {
	ID                    int64     `json:"id"`
	DiscordID             string    `json:"discord_id"`
	DiscordUsername       string    `json:"discord_username"`
	DiscordGlobalName     string    `json:"discord_global_name"`
	DiscordAvatar         string    `json:"discord_avatar"`
	Role                  string    `json:"role"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	ValidUploads          int64     `json:"valid_uploads"`
	TotalRequests         int64     `json:"total_requests"`
	RequestsPerMinute     int       `json:"requests_per_minute"`
	RequestsPerDay        int       `json:"requests_per_day"`
	MaxConcurrentRequests int       `json:"max_concurrent_requests"`
}

type UserLimits struct {
	RequestsPerMinute     int `json:"requests_per_minute"`
	RequestsPerDay        int `json:"requests_per_day"`
	MaxConcurrentRequests int `json:"max_concurrent_requests"`
}

type APIKey struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	Name          string    `json:"name"`
	Prefix        string    `json:"prefix"`
	MaskedKey     string    `json:"masked_key"`
	TotalRequests int64     `json:"total_requests"`
	RequestsToday int64     `json:"requests_today"`
	LastUsedAt    time.Time `json:"last_used_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	RevokedAt     time.Time `json:"revoked_at,omitempty"`
	PlaintextKey  string    `json:"key,omitempty"`
}

type Contribution struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Account      string    `json:"account"`
	AccountHash  string    `json:"account_hash"`
	Status       string    `json:"status"`
	Message      string    `json:"message"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	Referer      string    `json:"referer,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ValidatedAt  time.Time `json:"validated_at,omitempty"`
	ResponseTime int       `json:"response_time_ms,omitempty"`
}

type RequestLog struct {
	ID                  int64     `json:"id"`
	UserID              int64     `json:"user_id"`
	APIKeyID            int64     `json:"api_key_id"`
	Method              string    `json:"method"`
	Path                string    `json:"path"`
	Query               string    `json:"query,omitempty"`
	UpstreamPath        string    `json:"upstream_path,omitempty"`
	Status              int       `json:"status"`
	UpstreamStatus      int       `json:"upstream_status,omitempty"`
	DurationMS          int64     `json:"duration_ms"`
	FirstByteMS         int64     `json:"first_byte_ms,omitempty"`
	BytesIn             int64     `json:"bytes_in"`
	BytesOut            int64     `json:"bytes_out"`
	Stream              bool      `json:"stream,omitempty"`
	IP                  string    `json:"ip,omitempty"`
	UserAgent           string    `json:"user_agent,omitempty"`
	Referer             string    `json:"referer,omitempty"`
	RequestID           string    `json:"request_id,omitempty"`
	UpstreamRequestID   string    `json:"upstream_request_id,omitempty"`
	RequestContentType  string    `json:"request_content_type,omitempty"`
	ResponseContentType string    `json:"response_content_type,omitempty"`
	ErrorType           string    `json:"error_type,omitempty"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type UserStats struct {
	ValidUploads      int64 `json:"valid_uploads"`
	TotalRequests     int64 `json:"total_requests"`
	RequestsToday     int64 `json:"requests_today"`
	RequestsRemaining int64 `json:"requests_remaining"`
}

type DashboardStats struct {
	Users         int64 `json:"users"`
	ValidUploads  int64 `json:"valid_uploads"`
	TotalRequests int64 `json:"total_requests"`
	ActiveAPIKeys int64 `json:"active_api_keys"`
}

type RankQuery struct {
	Board  string
	Period string
	Search string
	Limit  int
	Offset int
	Now    time.Time
}

type RankItem struct {
	Rank             int    `json:"rank"`
	UserID           int64  `json:"user_id"`
	DisplayName      string `json:"display_name"`
	DiscordUsername  string `json:"discord_username"`
	DiscordIDPreview string `json:"discord_id_preview"`
	Value            int64  `json:"value"`
}

type RankResult struct {
	Board       string     `json:"board"`
	Period      string     `json:"period"`
	Items       []RankItem `json:"items"`
	Limit       int        `json:"limit"`
	Offset      int        `json:"offset"`
	NextOffset  int        `json:"next_offset,omitempty"`
	HasMore     bool       `json:"has_more"`
	GeneratedAt time.Time  `json:"generated_at"`
}

type ServiceConfig struct {
	NewAPIBaseURL         string `json:"new_api_base_url"`
	NewAPIKey             string `json:"new_api_key,omitempty"`
	NewAPIKeyPreview      string `json:"new_api_key_preview"`
	DS2APIBaseURL         string `json:"ds2api_base_url"`
	DS2APIAdminKey        string `json:"ds2api_admin_key,omitempty"`
	DS2APIAdminKeyPreview string `json:"ds2api_admin_key_preview"`
	DS2APIAutoProxy       struct {
		Enabled          bool   `json:"enabled"`
		Type             string `json:"type"`
		Host             string `json:"host"`
		Port             int    `json:"port"`
		UsernameTemplate string `json:"username_template"`
		Password         string `json:"password,omitempty"`
		PasswordPreview  string `json:"password_preview"`
		NameTemplate     string `json:"name_template"`
	} `json:"ds2api_auto_proxy"`
	DiscordClientID      string `json:"discord_client_id"`
	DiscordClientSecret  string `json:"discord_client_secret,omitempty"`
	DiscordSecretPreview string `json:"discord_secret_preview"`
	DiscordRedirectURL   string `json:"discord_redirect_url"`
	AppBaseURL           string `json:"app_base_url"`
}

var ErrNotFound = errors.New("not found")

func Open(path string) (*Store, error) {
	return OpenWithAPIKeySecret(path, "")
}

func OpenWithAPIKeySecret(path, apiKeyEncryptSecret string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	secret, err := resolveAPIKeyEncryptSecret(path, apiKeyEncryptSecret)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, apiKeyEncryptSecret: secret}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func resolveAPIKeyEncryptSecret(dbPath, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured, nil
	}
	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "dshare_key_secret")
	raw, err := os.ReadFile(path)
	if err == nil {
		secret := strings.TrimSpace(string(raw))
		if secret != "" {
			return secret, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	secret, err := security.RandomToken(48)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			discord_id TEXT NOT NULL UNIQUE,
			discord_username TEXT NOT NULL DEFAULT '',
			discord_global_name TEXT NOT NULL DEFAULT '',
			discord_avatar TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'user',
			requests_per_minute INTEGER NOT NULL,
			requests_per_day INTEGER NOT NULL,
			max_concurrent_requests INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_search ON users(discord_username, discord_global_name, discord_id)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL DEFAULT '',
			key_hash TEXT NOT NULL UNIQUE,
			key_ciphertext TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_used_at TEXT,
			revoked_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`,
		`CREATE TABLE IF NOT EXISTS contributions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			account TEXT NOT NULL,
			account_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			response_time_ms INTEGER NOT NULL DEFAULT 0,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			referer TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			validated_at TEXT
			)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_contributions_valid_account ON contributions(account_hash) WHERE status = 'valid'`,
		`CREATE INDEX IF NOT EXISTS idx_contributions_user ON contributions(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_contributions_status_created_user ON contributions(status, created_at DESC, user_id)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			api_key_id INTEGER NOT NULL DEFAULT 0,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			query TEXT NOT NULL DEFAULT '',
			upstream_path TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL,
			upstream_status INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL,
			first_byte_ms INTEGER NOT NULL DEFAULT 0,
			bytes_in INTEGER NOT NULL DEFAULT 0,
			bytes_out INTEGER NOT NULL DEFAULT 0,
			stream INTEGER NOT NULL DEFAULT 0,
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			referer TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			upstream_request_id TEXT NOT NULL DEFAULT '',
			request_content_type TEXT NOT NULL DEFAULT '',
			response_content_type TEXT NOT NULL DEFAULT '',
			error_type TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_user_time ON request_logs(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_time ON request_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_user ON request_logs(created_at DESC, user_id)`,
		`CREATE TABLE IF NOT EXISTS rank_counters (
			metric TEXT NOT NULL,
			period TEXT NOT NULL,
			period_start TEXT NOT NULL,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			value INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (metric, period, period_start, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rank_counters_lookup ON rank_counters(metric, period, period_start, value DESC, user_id)`,
		`CREATE TABLE IF NOT EXISTS service_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.ensureRequestLogColumns(ctx); err != nil {
		return err
	}
	if err := s.ensureContributionColumns(ctx); err != nil {
		return err
	}
	if err := s.ensureAPIKeyColumns(ctx); err != nil {
		return err
	}
	if err := s.ensureRankCounters(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureAPIKeyColumns(ctx context.Context) error {
	return s.ensureColumns(ctx, "api_keys", map[string]string{
		"key_ciphertext": "TEXT NOT NULL DEFAULT ''",
	})
}

func (s *Store) ensureContributionColumns(ctx context.Context) error {
	return s.ensureColumns(ctx, "contributions", map[string]string{
		"ip":         "TEXT NOT NULL DEFAULT ''",
		"user_agent": "TEXT NOT NULL DEFAULT ''",
		"referer":    "TEXT NOT NULL DEFAULT ''",
		"request_id": "TEXT NOT NULL DEFAULT ''",
	})
}

func (s *Store) ensureRequestLogColumns(ctx context.Context) error {
	return s.ensureColumns(ctx, "request_logs", map[string]string{
		"query":                 "TEXT NOT NULL DEFAULT ''",
		"upstream_path":         "TEXT NOT NULL DEFAULT ''",
		"upstream_status":       "INTEGER NOT NULL DEFAULT 0",
		"first_byte_ms":         "INTEGER NOT NULL DEFAULT 0",
		"stream":                "INTEGER NOT NULL DEFAULT 0",
		"ip":                    "TEXT NOT NULL DEFAULT ''",
		"user_agent":            "TEXT NOT NULL DEFAULT ''",
		"referer":               "TEXT NOT NULL DEFAULT ''",
		"request_id":            "TEXT NOT NULL DEFAULT ''",
		"upstream_request_id":   "TEXT NOT NULL DEFAULT ''",
		"request_content_type":  "TEXT NOT NULL DEFAULT ''",
		"response_content_type": "TEXT NOT NULL DEFAULT ''",
		"error_type":            "TEXT NOT NULL DEFAULT ''",
	})
}

func (s *Store) ensureColumns(ctx context.Context, table string, columns map[string]string) error {
	allowedTables := map[string]bool{
		"api_keys":      true,
		"contributions": true,
		"request_logs":  true,
	}
	if !allowedTables[table] {
		return fmt.Errorf("unsafe migration table %q", table)
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for name, ddl := range columns {
		if existing[name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+name+` `+ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureRankCounters(ctx context.Context) error {
	var existing int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rank_counters`).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	stmts := []string{
		`INSERT INTO rank_counters (metric, period, period_start, user_id, value)
			SELECT 'requests', 'all', '', user_id, COUNT(*)
			FROM request_logs
			GROUP BY user_id`,
		`INSERT INTO rank_counters (metric, period, period_start, user_id, value)
			SELECT 'requests', 'day', substr(created_at, 1, 10), user_id, COUNT(*)
			FROM request_logs
			GROUP BY substr(created_at, 1, 10), user_id`,
		`INSERT INTO rank_counters (metric, period, period_start, user_id, value)
			SELECT 'requests', 'week', date(substr(created_at, 1, 10), '-' || ((strftime('%w', substr(created_at, 1, 10)) + 6) % 7) || ' days'), user_id, COUNT(*)
			FROM request_logs
			GROUP BY date(substr(created_at, 1, 10), '-' || ((strftime('%w', substr(created_at, 1, 10)) + 6) % 7) || ' days'), user_id`,
		`INSERT INTO rank_counters (metric, period, period_start, user_id, value)
			SELECT 'accepted_requests', 'day', substr(created_at, 1, 10), user_id, COUNT(*)
			FROM request_logs
			WHERE NOT (
					status = 429 AND (
						error_type IN ('rate_limit', 'daily_rate_limit', 'concurrent_limit') OR
						error_message IN ('daily rate limit exceeded', 'rate limit exceeded', 'concurrent request limit exceeded')
					)
				)
				AND NOT (method = 'GET' AND path IN ('/v1/models', '/v1beta/models'))
			GROUP BY substr(created_at, 1, 10), user_id`,
		`INSERT INTO rank_counters (metric, period, period_start, user_id, value)
			SELECT 'contributions', 'all', '', user_id, COUNT(*)
			FROM contributions
			WHERE status = 'valid'
			GROUP BY user_id`,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ServiceSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM service_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

func (s *Store) UpdateServiceSettings(ctx context.Context, values map[string]string) error {
	now := formatTime(time.Now().UTC())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO service_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, strings.TrimSpace(value), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertDiscordUser(ctx context.Context, discordID, username, globalName, avatar string, defaults UserLimits) (*User, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			discord_id, discord_username, discord_global_name, discord_avatar, role,
			requests_per_minute, requests_per_day, max_concurrent_requests, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'user', ?, ?, ?, ?, ?)
		ON CONFLICT(discord_id) DO UPDATE SET
			discord_username = excluded.discord_username,
			discord_global_name = excluded.discord_global_name,
			discord_avatar = excluded.discord_avatar,
			updated_at = excluded.updated_at
	`, discordID, username, globalName, avatar, defaults.RequestsPerMinute, defaults.RequestsPerDay, defaults.MaxConcurrentRequests, formatTime(now), formatTime(now))
	if err != nil {
		return nil, err
	}
	return s.GetUserByDiscordID(ctx, discordID)
}

func (s *Store) GetUserByDiscordID(ctx context.Context, discordID string) (*User, error) {
	return s.getUser(ctx, `WHERE u.discord_id = ?`, discordID)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return s.getUser(ctx, `WHERE u.id = ?`, id)
}

func (s *Store) getUser(ctx context.Context, where string, args ...any) (*User, error) {
	query := userSelect() + " " + where + " GROUP BY u.id"
	row := s.db.QueryRowContext(ctx, query, args...)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return user, nil
}

func (s *Store) ListUsers(ctx context.Context, q string, limit, offset int) ([]User, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{}
	where := ""
	q = strings.TrimSpace(strings.ToLower(q))
	if q != "" {
		where = `WHERE lower(u.discord_username) LIKE ? OR lower(u.discord_global_name) LIKE ? OR lower(u.discord_id) LIKE ?`
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	var total int64
	countQuery := `SELECT COUNT(*) FROM users u ` + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := userSelect() + " " + where + " GROUP BY u.id ORDER BY u.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (s *Store) UpdateUserLimits(ctx context.Context, userID int64, limits UserLimits) (*User, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET requests_per_minute = ?, requests_per_day = ?, max_concurrent_requests = ?, updated_at = ?
		WHERE id = ?
	`, limits.RequestsPerMinute, limits.RequestsPerDay, limits.MaxConcurrentRequests, formatTime(now), userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.GetUserByID(ctx, userID)
}

func (s *Store) EnsureAdminByDiscordID(ctx context.Context, discordID string) error {
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET role = 'admin', updated_at = ? WHERE discord_id = ?`, formatTime(time.Now().UTC()), discordID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, time.Time, error) {
	token, err := security.RandomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(ttl)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, userID, security.SHA256Hex(token), formatTime(expires), formatTime(time.Now().UTC()))
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) GetSessionUser(ctx context.Context, token string) (*User, error) {
	hash := security.SHA256Hex(token)
	row := s.db.QueryRowContext(ctx, `
		SELECT user_id, expires_at FROM sessions WHERE token_hash = ?
	`, hash)
	var userID int64
	var expiresRaw string
	if err := row.Scan(&userID, &expiresRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	expires, err := parseTime(expiresRaw)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(expires) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hash)
		return nil, ErrNotFound
	}
	return s.GetUserByID(ctx, userID)
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, security.SHA256Hex(token))
	return err
}

func (s *Store) CreateAPIKey(ctx context.Context, userID int64, name string) (*APIKey, error) {
	token, err := security.RandomLetters(48)
	if err != nil {
		return nil, err
	}
	fullKey := "dsh-" + token
	prefix := fullKey
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	ciphertext, err := security.EncryptString(fullKey, s.apiKeyEncryptSecret)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, key_ciphertext, prefix, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, strings.TrimSpace(name), security.SHA256Hex(fullKey), ciphertext, prefix, formatTime(now))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &APIKey{
		ID:           id,
		UserID:       userID,
		Name:         strings.TrimSpace(name),
		Prefix:       prefix,
		MaskedKey:    security.MaskSecret(fullKey),
		CreatedAt:    now,
		PlaintextKey: fullKey,
	}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, userID int64) ([]APIKey, error) {
	start, end := todayUTCWindow()
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			k.id, k.user_id, k.name, k.prefix, k.created_at,
			COALESCE(k.last_used_at, ''), COALESCE(k.revoked_at, ''), k.key_ciphertext,
			(SELECT COUNT(*) FROM request_logs rl WHERE rl.api_key_id = k.id),
			(
					SELECT COUNT(*) FROM request_logs rl
					WHERE rl.api_key_id = k.id
						AND rl.created_at >= ? AND rl.created_at < ?
						AND NOT (
							rl.status = 429 AND (
								rl.error_type IN ('rate_limit', 'daily_rate_limit', 'concurrent_limit') OR
								rl.error_message IN ('daily rate limit exceeded', 'rate limit exceeded', 'concurrent request limit exceeded')
							)
						)
				)
		FROM api_keys k
		WHERE k.user_id = ? AND k.revoked_at IS NULL
		ORDER BY k.created_at DESC
	`, formatTime(start), formatTime(end), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []APIKey{}
	for rows.Next() {
		key, err := scanAPIKeyWithStats(rows)
		if err != nil {
			return nil, err
		}
		if key.PlaintextKey != "" {
			plain, err := security.DecryptString(key.PlaintextKey, s.apiKeyEncryptSecret)
			if err == nil {
				key.PlaintextKey = plain
			} else {
				key.PlaintextKey = ""
			}
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID, keyID int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE api_keys SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL
	`, formatTime(time.Now().UTC()), keyID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAPIKeyUser(ctx context.Context, key string) (*User, *APIKey, error) {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(strings.ToLower(key), "bearer ") {
		key = strings.TrimSpace(key[7:])
	}
	if key == "" {
		return nil, nil, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, prefix, created_at, COALESCE(last_used_at, ''), COALESCE(revoked_at, '')
		FROM api_keys
		WHERE key_hash = ? AND revoked_at IS NULL
	`, security.SHA256Hex(key))
	apiKey, err := scanAPIKey(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, formatTime(time.Now().UTC()), apiKey.ID)
	user, err := s.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, nil, err
	}
	return user, apiKey, nil
}

func (s *Store) RecordContribution(ctx context.Context, c Contribution) (bool, error) {
	c = prepareContribution(c)
	inserted, err := insertContribution(ctx, s.db, c)
	if err != nil || !inserted || c.Status != "valid" {
		return inserted, err
	}
	return inserted, addRankCount(ctx, s.db, "contributions", "all", "", c.UserID, 1)
}

func (s *Store) RecordContributionWithLimitBonus(ctx context.Context, c Contribution, rpmDelta, dailyDelta int) (bool, error) {
	c = prepareContribution(c)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	inserted, err := insertContribution(ctx, tx, c)
	if err != nil {
		return false, err
	}
	if inserted && (rpmDelta != 0 || dailyDelta != 0) {
		res, err := tx.ExecContext(ctx, `
			UPDATE users
			SET requests_per_minute = requests_per_minute + ?,
				requests_per_day = requests_per_day + ?,
				updated_at = ?
			WHERE id = ?
		`, rpmDelta, dailyDelta, formatTime(time.Now().UTC()), c.UserID)
		if err != nil {
			return false, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return false, ErrNotFound
		}
	}
	if inserted && c.Status == "valid" {
		if err := addRankCount(ctx, tx, "contributions", "all", "", c.UserID, 1); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return inserted, nil
}

func prepareContribution(c Contribution) Contribution {
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.ValidatedAt.IsZero() && c.Status == "valid" {
		c.ValidatedAt = now
	}
	if c.AccountHash == "" {
		c.AccountHash = security.SHA256Hex(strings.ToLower(strings.TrimSpace(c.Account)))
	}
	return c
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertContribution(ctx context.Context, db execer, c Contribution) (bool, error) {
	validated := nullableTime(c.ValidatedAt)
	res, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO contributions (
			user_id, account, account_hash, status, message, response_time_ms,
			ip, user_agent, referer, request_id, created_at, validated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.UserID, c.Account, c.AccountHash, c.Status, c.Message, c.ResponseTime, c.IP, c.UserAgent, c.Referer, c.RequestID, formatTime(c.CreatedAt), validated)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) ListContributions(ctx context.Context, userID int64, limit int) ([]Contribution, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, account, account_hash, status, message, response_time_ms,
			ip, user_agent, referer, request_id, created_at, COALESCE(validated_at, '')
		FROM contributions
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Contribution{}
	for rows.Next() {
		item, err := scanContribution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (s *Store) GetContributionForUser(ctx context.Context, userID, contributionID int64) (*Contribution, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, account, account_hash, status, message, response_time_ms,
			ip, user_agent, referer, request_id, created_at, COALESCE(validated_at, '')
		FROM contributions
		WHERE id = ? AND user_id = ?
	`, contributionID, userID)
	item, err := scanContribution(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

func (s *Store) MarkContributionDeletedWithLimitPenalty(ctx context.Context, userID, contributionID int64, message string, rpmDelta, dailyDelta int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := formatTime(time.Now().UTC())
	res, err := tx.ExecContext(ctx, `
		UPDATE contributions
		SET status = 'deleted', message = ?, validated_at = ?
		WHERE id = ? AND user_id = ? AND status = 'valid'
	`, strings.TrimSpace(message), now, contributionID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if rpmDelta != 0 || dailyDelta != 0 {
		res, err = tx.ExecContext(ctx, `
			UPDATE users
			SET requests_per_minute = max(0, requests_per_minute + ?),
				requests_per_day = max(0, requests_per_day + ?),
				updated_at = ?
			WHERE id = ?
		`, rpmDelta, dailyDelta, now, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	if err := addRankCount(ctx, tx, "contributions", "all", "", userID, -1); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordRequest(ctx context.Context, r RequestLog) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	stream := 0
	if r.Stream {
		stream = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO request_logs (
			user_id, api_key_id, method, path, query, upstream_path, status, upstream_status,
			duration_ms, first_byte_ms, bytes_in, bytes_out, stream,
			ip, user_agent, referer, request_id, upstream_request_id,
			request_content_type, response_content_type, error_type, error_message, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.UserID, r.APIKeyID, r.Method, r.Path, r.Query, r.UpstreamPath, r.Status, r.UpstreamStatus,
		r.DurationMS, r.FirstByteMS, r.BytesIn, r.BytesOut, stream,
		r.IP, r.UserAgent, r.Referer, r.RequestID, r.UpstreamRequestID,
		r.RequestContentType, r.ResponseContentType, r.ErrorType, r.ErrorMessage, formatTime(r.CreatedAt))
	if err != nil {
		return err
	}
	if err := addRequestRankCounts(ctx, tx, r.UserID, r.CreatedAt); err != nil {
		return err
	}
	if isAcceptedDailyRequestLog(r) {
		if err := addRankCount(ctx, tx, "accepted_requests", "day", dayPeriodStart(r.CreatedAt), r.UserID, 1); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CountRequestsSince(ctx context.Context, userID int64, since time.Time) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM request_logs WHERE user_id = ? AND created_at >= ?
	`, userID, formatTime(since.UTC())).Scan(&count)
	return count, err
}

func (s *Store) UserStats(ctx context.Context, userID int64) (UserStats, error) {
	var stats UserStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM contributions WHERE user_id = ? AND status = 'valid'),
			(SELECT COUNT(*) FROM request_logs WHERE user_id = ?)
	`, userID, userID).Scan(&stats.ValidUploads, &stats.TotalRequests)
	return stats, err
}

func (s *Store) UserStatsWithLimits(ctx context.Context, userID int64, dailyLimit int) (UserStats, error) {
	stats, err := s.UserStats(ctx, userID)
	if err != nil {
		return stats, err
	}
	stats.RequestsToday, err = s.CountAcceptedRequestsToday(ctx, userID)
	if err != nil {
		return stats, err
	}
	if dailyLimit > 0 {
		stats.RequestsRemaining = int64(dailyLimit) - stats.RequestsToday
		if stats.RequestsRemaining < 0 {
			stats.RequestsRemaining = 0
		}
	}
	return stats, nil
}

func (s *Store) CountAcceptedRequestsToday(ctx context.Context, userID int64) (int64, error) {
	var count int64
	start, end := todayUTCWindow()
	err := s.db.QueryRowContext(ctx, `
		SELECT value
		FROM rank_counters
		WHERE metric = 'accepted_requests' AND period = 'day' AND period_start = ? AND user_id = ?
	`, dayPeriodStart(start), userID).Scan(&count)
	if err == nil {
		return count, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM request_logs
			WHERE user_id = ? AND created_at >= ? AND created_at < ?
				AND NOT (
					status = 429 AND (
						error_type IN ('rate_limit', 'daily_rate_limit', 'concurrent_limit') OR
						error_message IN ('daily rate limit exceeded', 'rate limit exceeded', 'concurrent request limit exceeded')
					)
				)
				AND NOT (method = 'GET' AND path IN ('/v1/models', '/v1beta/models'))
	`, userID, formatTime(start), formatTime(end)).Scan(&count)
	return count, err
}

func (s *Store) IncrementUserLimits(ctx context.Context, userID int64, rpmDelta, dailyDelta int) (*User, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET requests_per_minute = requests_per_minute + ?,
			requests_per_day = requests_per_day + ?,
			updated_at = ?
		WHERE id = ?
	`, rpmDelta, dailyDelta, formatTime(now), userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetUserByID(ctx, userID)
}

func (s *Store) DashboardStats(ctx context.Context) (DashboardStats, error) {
	var stats DashboardStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM contributions WHERE status = 'valid'),
			(SELECT COUNT(*) FROM request_logs),
			(SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL)
	`).Scan(&stats.Users, &stats.ValidUploads, &stats.TotalRequests, &stats.ActiveAPIKeys)
	return stats, err
}

func (s *Store) PublicRank(ctx context.Context, q RankQuery) (RankResult, error) {
	q = normalizeRankQuery(q)
	result := RankResult{
		Board:       q.Board,
		Period:      q.Period,
		Limit:       q.Limit,
		Offset:      q.Offset,
		GeneratedAt: nowOrUTC(q.Now),
	}
	periodStart := rankPeriodStart(q)
	searchSQL, searchArgs := rankSearchSQL(q.Search)
	args := []any{q.Board, q.Period, periodStart}
	args = append(args, searchArgs...)
	args = append(args, q.Limit+1, q.Offset)

	query := buildRankSQL(searchSQL, q.Search != "")
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	items := make([]RankItem, 0, q.Limit)
	for rows.Next() {
		var item RankItem
		var discordID string
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.DiscordUsername, &discordID, &item.Value); err != nil {
			return result, err
		}
		if item.DisplayName == "" {
			item.DisplayName = displayName(item.DiscordUsername, "", discordID)
		}
		item.DiscordIDPreview = maskDiscordID(discordID)
		if len(items) < q.Limit {
			item.Rank = q.Offset + len(items) + 1
			items = append(items, item)
		} else {
			result.HasMore = true
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Items = items
	if result.HasMore {
		result.NextOffset = q.Offset + len(items)
	}
	return result, nil
}

func userSelect() string {
	return `
		SELECT
			u.id, u.discord_id, u.discord_username, u.discord_global_name, u.discord_avatar, u.role,
			u.requests_per_minute, u.requests_per_day, u.max_concurrent_requests,
			u.created_at, u.updated_at,
			COALESCE(SUM(CASE WHEN c.status = 'valid' THEN 1 ELSE 0 END), 0) AS valid_uploads,
			(SELECT COUNT(*) FROM request_logs rl WHERE rl.user_id = u.id) AS total_requests
		FROM users u
		LEFT JOIN contributions c ON c.user_id = u.id
	`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*User, error) {
	var u User
	var created, updated string
	if err := row.Scan(
		&u.ID, &u.DiscordID, &u.DiscordUsername, &u.DiscordGlobalName, &u.DiscordAvatar, &u.Role,
		&u.RequestsPerMinute, &u.RequestsPerDay, &u.MaxConcurrentRequests,
		&created, &updated, &u.ValidUploads, &u.TotalRequests,
	); err != nil {
		return nil, err
	}
	var err error
	u.CreatedAt, err = parseTime(created)
	if err != nil {
		return nil, err
	}
	u.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func scanAPIKey(row scanner) (*APIKey, error) {
	var key APIKey
	var created, lastUsed, revoked string
	if err := row.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &created, &lastUsed, &revoked); err != nil {
		return nil, err
	}
	key.CreatedAt, _ = parseTime(created)
	key.LastUsedAt, _ = parseOptionalTime(lastUsed)
	key.RevokedAt, _ = parseOptionalTime(revoked)
	key.MaskedKey = key.Prefix + "..."
	return &key, nil
}

func scanAPIKeyWithStats(row scanner) (*APIKey, error) {
	var key APIKey
	var created, lastUsed, revoked, ciphertext string
	if err := row.Scan(
		&key.ID, &key.UserID, &key.Name, &key.Prefix, &created, &lastUsed, &revoked,
		&ciphertext, &key.TotalRequests, &key.RequestsToday,
	); err != nil {
		return nil, err
	}
	key.CreatedAt, _ = parseTime(created)
	key.LastUsedAt, _ = parseOptionalTime(lastUsed)
	key.RevokedAt, _ = parseOptionalTime(revoked)
	key.MaskedKey = key.Prefix + "..."
	key.PlaintextKey = ciphertext
	return &key, nil
}

func scanContribution(row scanner) (*Contribution, error) {
	var c Contribution
	var created, validated string
	if err := row.Scan(
		&c.ID, &c.UserID, &c.Account, &c.AccountHash, &c.Status, &c.Message, &c.ResponseTime,
		&c.IP, &c.UserAgent, &c.Referer, &c.RequestID, &created, &validated,
	); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = parseTime(created)
	c.ValidatedAt, _ = parseOptionalTime(validated)
	return &c, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func todayUTCWindow() (time.Time, time.Time) {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.Add(24 * time.Hour)
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

func normalizeRankQuery(q RankQuery) RankQuery {
	q.Board = strings.ToLower(strings.TrimSpace(q.Board))
	if q.Board != "contributions" {
		q.Board = "requests"
	}
	q.Period = strings.ToLower(strings.TrimSpace(q.Period))
	if q.Board == "contributions" {
		q.Period = "all"
	} else {
		switch q.Period {
		case "day", "week", "all":
		default:
			q.Period = "all"
		}
	}
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 80 {
		q.Search = q.Search[:80]
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return q
}

func buildRankSQL(searchSQL string, hasSearch bool) string {
	where := ""
	if hasSearch {
		where = `AND ` + searchSQL
	}
	return `
		SELECT
			u.id,
			COALESCE(NULLIF(u.discord_global_name, ''), NULLIF(u.discord_username, ''), u.discord_id) AS display_name,
			u.discord_username,
			u.discord_id,
			rc.value
		FROM rank_counters rc
		JOIN users u ON u.id = rc.user_id
		WHERE rc.metric = ? AND rc.period = ? AND rc.period_start = ? AND rc.value > 0
		` + where + `
		ORDER BY rc.value DESC, u.id ASC
		LIMIT ? OFFSET ?`
}

func addRequestRankCounts(ctx context.Context, db execer, userID int64, createdAt time.Time) error {
	createdAt = createdAt.UTC()
	for _, item := range []struct {
		period string
		start  string
	}{
		{period: "all", start: ""},
		{period: "day", start: dayPeriodStart(createdAt)},
		{period: "week", start: weekPeriodStart(createdAt)},
	} {
		if err := addRankCount(ctx, db, "requests", item.period, item.start, userID, 1); err != nil {
			return err
		}
	}
	return nil
}

func isAcceptedDailyRequestLog(r RequestLog) bool {
	if r.Method == "GET" && (r.Path == "/v1/models" || r.Path == "/v1beta/models") {
		return false
	}
	if r.Status == 429 {
		switch r.ErrorType {
		case "rate_limit", "daily_rate_limit", "concurrent_limit":
			return false
		}
		switch r.ErrorMessage {
		case "daily rate limit exceeded", "rate limit exceeded", "concurrent request limit exceeded":
			return false
		}
	}
	return true
}

func addRankCount(ctx context.Context, db execer, metric, period, periodStart string, userID int64, delta int64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO rank_counters (metric, period, period_start, user_id, value)
		VALUES (?, ?, ?, ?, max(0, ?))
		ON CONFLICT(metric, period, period_start, user_id) DO UPDATE SET
			value = max(0, value + ?)
	`, metric, period, periodStart, userID, delta, delta)
	return err
}

func rankPeriodStart(q RankQuery) string {
	if q.Board == "contributions" || q.Period == "all" {
		return ""
	}
	now := nowOrUTC(q.Now)
	if q.Period == "day" {
		return dayPeriodStart(now)
	}
	return weekPeriodStart(now)
}

func dayPeriodStart(t time.Time) string {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func weekPeriodStart(t time.Time) string {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return dayStart.AddDate(0, 0, -(weekday - 1)).Format("2006-01-02")
}

func rankSearchSQL(search string) (string, []any) {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return "", nil
	}
	like := "%" + search + "%"
	return `(lower(u.discord_username) LIKE ? OR lower(u.discord_global_name) LIKE ? OR lower(u.discord_id) LIKE ?)`, []any{like, like, like}
}

func nowOrUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func displayName(username, globalName, discordID string) string {
	if strings.TrimSpace(globalName) != "" {
		return strings.TrimSpace(globalName)
	}
	if strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}
	return strings.TrimSpace(discordID)
}

func maskDiscordID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 6 {
		return id
	}
	return id[:3] + "..." + id[len(id)-3:]
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", value, err)
	}
	return t, nil
}

func parseOptionalTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return parseTime(value)
}
