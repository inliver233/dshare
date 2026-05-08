package ds2api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAddAndValidateEmailUsesBulkImportWithAutoProxy(t *testing.T) {
	var sawBulkImport bool
	var sawTest bool
	var gotAutoProxy map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token","expires_in":3600}`))
		case "/admin/accounts/bulk-import":
			sawBulkImport = true
			if auth := r.Header.Get("Authorization"); auth != "Bearer admin-token" {
				t.Fatalf("Authorization = %q", auth)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode bulk payload: %v", err)
			}
			lines, _ := payload["lines"].(string)
			if lines != "u@example.com:secret" {
				t.Fatalf("lines = %q", lines)
			}
			gotAutoProxy, _ = payload["auto_proxy"].(map[string]any)
			_, _ = w.Write([]byte(`{"success":true,"imported_accounts":1,"imported_proxies":1,"skipped":[],"errors":[]}`))
		case "/admin/accounts/test":
			sawTest = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode test payload: %v", err)
			}
			if payload["identifier"] != "u@example.com" {
				t.Fatalf("identifier = %#v", payload["identifier"])
			}
			_, _ = w.Write([]byte(`{"account":"u@example.com","success":true,"message":"ok","response_time":123}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL, "admin-key", time.Second, AutoProxyConfig{
		Enabled:          true,
		Type:             "socks5",
		Host:             "172.20.0.1",
		Port:             21345,
		UsernameTemplate: "Default.{local}",
		Password:         "proxy-pass",
		NameTemplate:     "resin-{local}",
	})
	result := c.AddAndValidateAccount(context.Background(), "u@example.com", "secret")
	if result.Status != "valid" {
		t.Fatalf("status = %q, message = %q", result.Status, result.Message)
	}
	if !sawBulkImport || !sawTest {
		t.Fatalf("sawBulkImport=%t sawTest=%t", sawBulkImport, sawTest)
	}
	if gotAutoProxy["enabled"] != true {
		t.Fatalf("auto_proxy.enabled = %#v", gotAutoProxy["enabled"])
	}
	if gotAutoProxy["host"] != "172.20.0.1" || gotAutoProxy["username_template"] != "Default.{local}" {
		t.Fatalf("auto_proxy = %#v", gotAutoProxy)
	}
}

func TestAddAndValidateMobileCreatesProxyAndAssignsProxyID(t *testing.T) {
	var sawProxy bool
	var sawAccount bool
	var gotProxyID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token","expires_in":3600}`))
		case "/admin/proxies":
			sawProxy = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode proxy payload: %v", err)
			}
			gotProxyID, _ = payload["id"].(string)
			if !strings.HasPrefix(gotProxyID, "proxy_") {
				t.Fatalf("proxy id = %q", gotProxyID)
			}
			if payload["username"] != "Default.13800138000" {
				t.Fatalf("proxy username = %#v", payload["username"])
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/admin/accounts":
			sawAccount = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode account payload: %v", err)
			}
			if payload["mobile"] != "13800138000" {
				t.Fatalf("mobile = %#v", payload["mobile"])
			}
			if payload["proxy_id"] != gotProxyID {
				t.Fatalf("proxy_id = %#v, want %q", payload["proxy_id"], gotProxyID)
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/admin/accounts/test":
			_, _ = w.Write([]byte(`{"account":"13800138000","success":true,"message":"ok","response_time":50}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL, "admin-key", time.Second, AutoProxyConfig{
		Enabled:          true,
		Type:             "socks5",
		Host:             "172.20.0.1",
		Port:             21345,
		UsernameTemplate: "Default.{local}",
		Password:         "proxy-pass",
		NameTemplate:     "resin-{local}",
	})
	result := c.AddAndValidateAccount(context.Background(), "13800138000", "secret")
	if result.Status != "valid" {
		t.Fatalf("status = %q, message = %q", result.Status, result.Message)
	}
	if !sawProxy || !sawAccount {
		t.Fatalf("sawProxy=%t sawAccount=%t", sawProxy, sawAccount)
	}
}

func TestDeleteAccountUsesAdminAccountDeleteEndpoint(t *testing.T) {
	var sawDelete bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token","expires_in":3600}`))
		case "/admin/accounts/u@example.com":
			if r.Method != http.MethodDelete {
				t.Fatalf("method = %s, want DELETE", r.Method)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer admin-token" {
				t.Fatalf("Authorization = %q", auth)
			}
			sawDelete = true
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL, "admin-key", time.Second, AutoProxyConfig{})
	if err := c.DeleteAccount(context.Background(), "u@example.com"); err != nil {
		t.Fatal(err)
	}
	if !sawDelete {
		t.Fatal("delete endpoint was not called")
	}
}
