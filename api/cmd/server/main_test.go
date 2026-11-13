package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestMigratedRoutes(t *testing.T) {
	washerAgentState = AgentState{Status: "idle"}
	dryerAgentState = AgentState{Status: "idle"}
	app := newApp()

	t.Run("health endpoint and CORS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("Origin", "https://dashboard.example")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
		}
	})

	t.Run("JSON binding", func(t *testing.T) {
		resp := postJSON(t, app, "/washer/setAgentStatus", `{"status":"idle"}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("valid request status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		resp = postJSON(t, app, "/washer/setAgentStatus", `{"status":`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("malformed request status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("check-in JSON binding", func(t *testing.T) {
		resp := postJSON(t, app, "/washer/checkin", `{"sensorOk":true}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		agentMutex.Lock()
		got := washerSensorOk
		agentMutex.Unlock()
		if !got {
			t.Error("washerSensorOk = false, want true")
		}
	})
}

func postJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
