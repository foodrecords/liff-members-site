package kiosk

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLineProfileVerifiesChannelBeforeUsingProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/v2.1/verify":
			if r.URL.Query().Get("access_token") != "valid-token" {
				http.Error(w, "invalid", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"client_id":"test-channel","expires_in":3600}`)
		case "/v2/profile":
			if r.Header.Get("Authorization") != "Bearer valid-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"userId":"Utest"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LINE_API_BASE_URL", server.URL)
	t.Setenv("LINE_LOGIN_CHANNEL_ID", "test-channel")

	uid, err := lineProfile("valid-token")
	if err != nil || uid != "Utest" {
		t.Fatalf("uid=%q err=%v", uid, err)
	}
}

func TestLineProfileRejectsAnotherChannel(t *testing.T) {
	profileCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/profile" {
			profileCalled = true
		}
		fmt.Fprint(w, `{"client_id":"another-channel","expires_in":3600}`)
	}))
	defer server.Close()
	t.Setenv("LINE_API_BASE_URL", server.URL)
	t.Setenv("LINE_LOGIN_CHANNEL_ID", "expected-channel")

	if _, err := lineProfile("other-token"); err == nil {
		t.Fatal("expected channel mismatch")
	}
	if profileCalled {
		t.Fatal("profile must not be requested after channel mismatch")
	}
}

func TestLineProfileAcceptsConfiguredChannelList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/v2.1/verify":
			fmt.Fprint(w, `{"client_id":"mobile-channel","expires_in":3600}`)
		case "/v2/profile":
			fmt.Fprint(w, `{"userId":"Umobile"}`)
		}
	}))
	defer server.Close()
	t.Setenv("LINE_API_BASE_URL", server.URL)
	t.Setenv("LINE_LOGIN_CHANNEL_IDS", "members-channel, mobile-channel")
	t.Setenv("LINE_LOGIN_CHANNEL_ID", "")

	uid, err := lineProfile("valid-token")
	if err != nil || uid != "Umobile" {
		t.Fatalf("uid=%q err=%v", uid, err)
	}
}

func TestLineProfileRequiresConfiguredChannel(t *testing.T) {
	t.Setenv("LINE_LOGIN_CHANNEL_ID", "")
	t.Setenv("LINE_LOGIN_CHANNEL_IDS", "")
	if _, err := lineProfile("token"); err == nil {
		t.Fatal("expected missing channel configuration error")
	}
}
