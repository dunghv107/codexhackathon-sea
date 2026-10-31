package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAuthSessionsAcrossDevices(t *testing.T) {
	f := newFixture(t)
	login := func() map[string]any {
		return f.request("POST", "/api/auth/login", `{"username":"admin","password":"test-password"}`, 200)
	}
	refresh := func(device map[string]any, status int) map[string]any {
		return f.request("POST", "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, device["refresh_token"]), status)
	}
	a, b := login(), login()
	if a["access_token"] == b["access_token"] || a["refresh_token"] == b["refresh_token"] {
		t.Fatal("each login must issue independent credentials")
	}
	for _, device := range []map[string]any{a, b} {
		f.token = device["access_token"].(string)
		f.request("GET", "/api/auth/me", "", 200)
	}

	// Separate devices can refresh at the same time.
	responses := make(chan *httptest.ResponseRecorder, 2)
	parallelRefresh := func(device map[string]any) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/auth/refresh", strings.NewReader(fmt.Sprintf(`{"refresh_token":%q}`, device["refresh_token"])))
		r.Header.Set("Content-Type", "application/json")
		f.engine.ServeHTTP(w, r)
		responses <- w
	}
	go parallelRefresh(a)
	go parallelRefresh(b)
	devices := make([]map[string]any, 0, 2)
	for range 2 {
		response := <-responses
		if response.Code != 200 {
			t.Fatalf("parallel device refresh: %d %s", response.Code, response.Body.String())
		}
		var body struct{ Data map[string]any }
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		devices = append(devices, body.Data)
	}
	refresh(a, 401)
	refresh(b, 401)
	a, b = devices[0], devices[1]
	f.token = a["access_token"].(string)
	f.request("POST", "/api/auth/logout", "", 200)
	f.request("GET", "/api/auth/me", "", 401)
	refresh(a, 401)
	f.token = b["access_token"].(string)
	f.request("GET", "/api/auth/me", "", 200)
	b = refresh(b, 200)
	login()
	f.token = a["access_token"].(string)
	f.request("GET", "/api/auth/me", "", 401)

	// A replay race on one device still has exactly one winner.
	go parallelRefresh(b)
	go parallelRefresh(b)
	first, second := <-responses, <-responses
	if !(first.Code == 200 && second.Code == 401 || first.Code == 401 && second.Code == 200) {
		t.Fatalf("same-token race got %d and %d, want one success and one rejection", first.Code, second.Code)
	}
}

func TestAuthSessionExpiryOwnershipAndMigration(t *testing.T) {
	f := newFixture(t)
	login := f.request("POST", "/api/auth/login", `{"username":"admin","password":"test-password"}`, 200)
	f.token = login["access_token"].(string)
	var session struct {
		ID        string
		UserID    uint
		ExpiresAt time.Time
	}
	if err := f.db.Table("user_sessions").Order("created_at DESC").First(&session).Error; err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE user_sessions SET expires_at=now()-interval '1 second' WHERE id=?", session.ID)
	f.request("GET", "/api/auth/me", "", 401)
	f.request("POST", "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, login["refresh_token"]), 401)
	login = f.request("POST", "/api/auth/login", `{"username":"admin","password":"test-password"}`, 200)
	var expired int64
	if err := f.db.Table("user_sessions").Where("id=?", session.ID).Count(&expired).Error; err != nil || expired != 0 {
		t.Fatal("login should clean up expired sessions", expired, err)
	}
	f.token = login["access_token"].(string)
	f.exec("UPDATE users SET refresh_token_hash=repeat('a',64), refresh_token_expires_at=now()+interval '1 day'")
	up, err := os.ReadFile("../migrations/000009_user_sessions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(string(up))
	f.request("GET", "/api/auth/me", "", 200)
	var legacy int64
	if err := f.db.Table("users").Where("refresh_token_hash IS NOT NULL OR refresh_token_expires_at IS NOT NULL").Count(&legacy).Error; err != nil || legacy != 0 {
		t.Fatal("migration must clear legacy session state", legacy, err)
	}
	other := f.request("POST", "/api/users", `{"username":"other","email":"other@example.test","password":"test-password"}`, 201)
	// Even a valid signed access token must match its session's current user.
	f.exec("UPDATE user_sessions SET user_id=? WHERE user_id=?", other["id"], session.UserID)
	f.request("GET", "/api/auth/me", "", 401)
	down, err := os.ReadFile("../migrations/000009_user_sessions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(string(down))
	f.exec(string(up))
	login = f.request("POST", "/api/auth/login", `{"username":"admin","password":"test-password"}`, 200)
	f.token = login["access_token"].(string)
	f.request("GET", "/api/auth/me", "", 200)
}
