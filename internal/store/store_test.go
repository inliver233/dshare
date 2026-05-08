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
