package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestMigratedRoutes(t *testing.T) {
	stateMutex.Lock()
	stateHistory = nil
	lastStateSubmission = time.Time{}
	stateMutex.Unlock()
	app := newApp()

	t.Run("status endpoint", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/status", nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("JSON binding", func(t *testing.T) {
		resp := postJSON(t, app, `{"state":"vibrating"}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("valid request status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		stateMutex.Lock()
		gotCount := len(stateHistory)
		gotState := ""
		if gotCount > 0 {
			gotState = stateHistory[0].State
		}
		stateMutex.Unlock()
		if gotCount != 1 || gotState != "vibrating" {
			t.Errorf("stateHistory = (%d entries, %q), want (1 entry, %q)", gotCount, gotState, "vibrating")
		}

		resp = postJSON(t, app, `{"state":`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("malformed request status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})
}

func postJSON(t *testing.T, app *fiber.App, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/submitState", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
