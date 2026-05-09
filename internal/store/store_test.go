package store

import (
	"context"
	"regexp"
	"testing"
	"time"
)

func TestCreateAPIKeyUsesDshareLetterFormat(t *testing.T) {
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	user, err := s.UpsertDiscordUser(context.Background(), "discord-1", "tester", "Tester", "", UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(context.Background(), user.ID, "default")
	if err != nil {
		t.Fatal(err)
	}
	if ok := regexp.MustCompile(`^dsh-[A-Za-z]+$`).MatchString(key.PlaintextKey); !ok {
		t.Fatalf("key format = %q, want dsh- plus letters only", key.PlaintextKey)
	}
}

func TestRecordContributionWithLimitBonusOnlyForNewValidAccount(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	user, err := s.UpsertDiscordUser(ctx, "discord-2", "tester", "Tester", "", UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	contribution := Contribution{
		UserID:    user.ID,
		Account:   "deepseek@example.com",
		Status:    "valid",
		Message:   "ok",
		IP:        "203.0.113.10",
		UserAgent: "test-agent",
	}
	inserted, err := s.RecordContributionWithLimitBonus(ctx, contribution, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first valid contribution was not inserted")
	}
	updated, err := s.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RequestsPerMinute != 6 || updated.RequestsPerDay != 2600 {
		t.Fatalf("limits = rpm %d, daily %d; want 6, 2600", updated.RequestsPerMinute, updated.RequestsPerDay)
	}

	inserted, err = s.RecordContributionWithLimitBonus(ctx, contribution, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("duplicate valid contribution was inserted")
	}
	updated, err = s.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RequestsPerMinute != 6 || updated.RequestsPerDay != 2600 {
		t.Fatalf("duplicate changed limits to rpm %d, daily %d", updated.RequestsPerMinute, updated.RequestsPerDay)
	}
}

func TestMarkContributionDeletedWithLimitPenalty(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	user, err := s.UpsertDiscordUser(ctx, "discord-delete", "tester", "Tester", "", UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := s.RecordContributionWithLimitBonus(ctx, Contribution{
		UserID:  user.ID,
		Account: "remove@example.com",
		Status:  "valid",
		Message: "ok",
	}, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("valid contribution was not inserted")
	}
	items, err := s.ListContributions(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if err := s.MarkContributionDeletedWithLimitPenalty(ctx, user.ID, items[0].ID, "deleted by user", -1, -100); err != nil {
		t.Fatal(err)
	}

	updated, err := s.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ValidUploads != 0 {
		t.Fatalf("valid uploads = %d, want 0", updated.ValidUploads)
	}
	if updated.RequestsPerMinute != 5 || updated.RequestsPerDay != 2500 {
		t.Fatalf("limits = rpm %d, daily %d; want 5, 2500", updated.RequestsPerMinute, updated.RequestsPerDay)
	}
	deleted, err := s.GetContributionForUser(ctx, user.ID, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != "deleted" || deleted.Message != "deleted by user" {
		t.Fatalf("deleted contribution = status %q message %q", deleted.Status, deleted.Message)
	}
}

func TestListAPIKeysIncludesUsageAndHidesRevokedKeys(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	user, err := s.UpsertDiscordUser(ctx, "discord-3", "tester", "Tester", "", UserLimits{
		RequestsPerMinute:     5,
		RequestsPerDay:        2500,
		MaxConcurrentRequests: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "active")
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := s.CreateAPIKey(ctx, user.ID, "revoked")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeAPIKey(ctx, user.ID, revoked.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRequest(ctx, RequestLog{
		UserID:     user.ID,
		APIKeyID:   key.ID,
		Method:     "POST",
		Path:       "/v1/chat/completions",
		Status:     200,
		DurationMS: 12,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRequest(ctx, RequestLog{
		UserID:       user.ID,
		APIKeyID:     key.ID,
		Method:       "POST",
		Path:         "/v1/chat/completions",
		Status:       429,
		DurationMS:   1,
		ErrorMessage: "rate limit exceeded",
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	keys, err := s.ListAPIKeys(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d, want 1 active key", len(keys))
	}
	if keys[0].ID != key.ID {
		t.Fatalf("key id = %d, want %d", keys[0].ID, key.ID)
	}
	if keys[0].TotalRequests != 2 || keys[0].RequestsToday != 1 {
		t.Fatalf("usage = total %d, today %d; want 2, 1", keys[0].TotalRequests, keys[0].RequestsToday)
	}
	if keys[0].PlaintextKey != key.PlaintextKey {
		t.Fatalf("plaintext key = %q, want original key", keys[0].PlaintextKey)
	}
}

func TestListAPIKeysExcludesNewRateLimitMessagesFromToday(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	user, err := s.UpsertDiscordUser(ctx, "discord-rate", "tester", "Tester", "", UserLimits{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(ctx, user.ID, "active")
	if err != nil {
		t.Fatal(err)
	}
	for _, errorType := range []string{"rate_limit", "daily_rate_limit", "concurrent_limit"} {
		if err := s.RecordRequest(ctx, RequestLog{
			UserID:     user.ID,
			APIKeyID:   key.ID,
			Method:     "POST",
			Path:       "/v1/chat/completions",
			Status:     429,
			ErrorType:  errorType,
			CreatedAt:  time.Now().UTC(),
			DurationMS: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := s.ListAPIKeys(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d", len(keys))
	}
	if keys[0].TotalRequests != 3 || keys[0].RequestsToday != 0 {
		t.Fatalf("usage = total %d today %d; want 3, 0", keys[0].TotalRequests, keys[0].RequestsToday)
	}
	accepted, err := s.CountAcceptedRequestsToday(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted != 0 {
		t.Fatalf("accepted today = %d, want 0", accepted)
	}
}

func TestPublicRankRequestsPeriodsAndSearch(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	alice, err := s.UpsertDiscordUser(ctx, "100001", "alice", "Alice", "", UserLimits{})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.UpsertDiscordUser(ctx, "100002", "bob", "Bob", "", UserLimits{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.RecordRequest(ctx, RequestLog{UserID: alice.ID, Method: "POST", Path: "/v1/chat/completions", Status: 200, CreatedAt: now.Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.RecordRequest(ctx, RequestLog{UserID: bob.ID, Method: "POST", Path: "/v1/chat/completions", Status: 200, CreatedAt: now.Add(-8 * 24 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.PublicRank(ctx, RankQuery{Board: "requests", Period: "all", Limit: 10, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 2 || all.Items[0].DisplayName != "Alice" || all.Items[0].Value != 3 {
		t.Fatalf("all rank = %#v", all.Items)
	}
	week, err := s.PublicRank(ctx, RankQuery{Board: "requests", Period: "week", Limit: 10, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(week.Items) != 1 || week.Items[0].DisplayName != "Alice" {
		t.Fatalf("week rank = %#v", week.Items)
	}
	search, err := s.PublicRank(ctx, RankQuery{Board: "requests", Period: "all", Search: "bo", Limit: 10, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Items) != 1 || search.Items[0].DisplayName != "Bob" {
		t.Fatalf("search rank = %#v", search.Items)
	}
}

func TestPublicRankContributions(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alice, err := s.UpsertDiscordUser(ctx, "200001", "alice", "Alice", "", UserLimits{})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.UpsertDiscordUser(ctx, "200002", "bob", "Bob", "", UserLimits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{"a1@example.com", "a2@example.com"} {
		if _, err := s.RecordContribution(ctx, Contribution{UserID: alice.ID, Account: account, Status: "valid"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.RecordContribution(ctx, Contribution{UserID: bob.ID, Account: "b1@example.com", Status: "valid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordContribution(ctx, Contribution{UserID: bob.ID, Account: "bad@example.com", Status: "invalid"}); err != nil {
		t.Fatal(err)
	}

	rank, err := s.PublicRank(ctx, RankQuery{Board: "contributions", Period: "day", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rank.Board != "contributions" || rank.Period != "all" {
		t.Fatalf("rank metadata = %#v", rank)
	}
	if len(rank.Items) != 2 || rank.Items[0].DisplayName != "Alice" || rank.Items[0].Value != 2 {
		t.Fatalf("contribution rank = %#v", rank.Items)
	}
}

func TestPublicRankBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/dshare.db"
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	user, err := s.UpsertDiscordUser(ctx, "300001", "legacy", "Legacy", "", UserLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO request_logs (user_id, method, path, status, duration_ms, created_at)
		VALUES (?, 'POST', '/v1/chat/completions', 200, 1, ?)
	`, user.ID, formatTime(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO contributions (user_id, account, account_hash, status, message, created_at, validated_at)
		VALUES (?, 'legacy@example.com', 'legacy-hash', 'valid', 'ok', ?, ?)
	`, user.ID, formatTime(now), formatTime(now)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	requests, err := s.PublicRank(ctx, RankQuery{Board: "requests", Period: "all", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests.Items) != 1 || requests.Items[0].DisplayName != "Legacy" || requests.Items[0].Value != 1 {
		t.Fatalf("request backfill rank = %#v", requests.Items)
	}
	contributions, err := s.PublicRank(ctx, RankQuery{Board: "contributions"})
	if err != nil {
		t.Fatal(err)
	}
	if len(contributions.Items) != 1 || contributions.Items[0].DisplayName != "Legacy" || contributions.Items[0].Value != 1 {
		t.Fatalf("contribution backfill rank = %#v", contributions.Items)
	}
	accepted, err := s.CountAcceptedRequestsToday(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted != 1 {
		t.Fatalf("accepted request backfill = %d, want 1", accepted)
	}
}

func TestPublicRankSubtractsDeletedContribution(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir() + "/dshare.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	user, err := s.UpsertDiscordUser(ctx, "400001", "delete", "Delete", "", UserLimits{
		RequestsPerMinute: 1,
		RequestsPerDay:    100,
	})
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := s.RecordContributionWithLimitBonus(ctx, Contribution{
		UserID:  user.ID,
		Account: "delete-rank@example.com",
		Status:  "valid",
		Message: "ok",
	}, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("contribution was not inserted")
	}
	rank, err := s.PublicRank(ctx, RankQuery{Board: "contributions"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rank.Items) != 1 || rank.Items[0].Value != 1 {
		t.Fatalf("rank before delete = %#v", rank.Items)
	}
	items, err := s.ListContributions(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkContributionDeletedWithLimitPenalty(ctx, user.ID, items[0].ID, "deleted", -1, -100); err != nil {
		t.Fatal(err)
	}
	rank, err = s.PublicRank(ctx, RankQuery{Board: "contributions"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rank.Items) != 0 {
		t.Fatalf("rank after delete = %#v, want empty", rank.Items)
	}
}
