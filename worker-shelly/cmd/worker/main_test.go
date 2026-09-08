package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestShellyClientReadCurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/rpc/PM1.GetStatus" || request.URL.Query().Get("id") != "0" {
			t.Fatalf("unexpected request %s", request.URL.String())
		}
		_, _ = response.Write([]byte(`{"id":0,"voltage":121.6,"current":1.37,"apower":166.7,"aenergy":{"total":487.164}}`))
	}))
	defer server.Close()

	client := ShellyClient{
		baseURL: server.URL,
		client:  server.Client(),
	}
	amps, err := client.ReadCurrent(context.Background())
	if err != nil {
		t.Fatalf("ReadCurrent returned error: %v", err)
	}
	if amps != 1.37 {
		t.Fatalf("ReadCurrent returned %v amps, want 1.37", amps)
	}
}

func TestShellyClientReadCurrentRejectsMissingCurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte(`{"id":0,"voltage":121.6,"apower":6.7}`))
	}))
	defer server.Close()

	client := ShellyClient{baseURL: server.URL, client: server.Client()}
	if _, err := client.ReadCurrent(context.Background()); err == nil {
		t.Fatal("ReadCurrent returned nil error for missing current")
	}
}

func TestCurrentMonitorCompletesWhenStartedInactive(t *testing.T) {
	monitor := NewCurrentMonitor(0.5, 0.1, 10*time.Second)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	if monitor.Observe(start, 0, nil) {
		t.Fatal("monitor completed on the first inactive reading")
	}
	if !monitor.Observe(start.Add(10*time.Second), 0, nil) {
		t.Fatal("monitor did not complete after inactive duration")
	}
	if monitor.State() != inactive {
		t.Fatalf("state = %v, want inactive", monitor.State())
	}
}

func TestCurrentMonitorCompletesAfterContinuousInactiveCurrent(t *testing.T) {
	monitor := NewCurrentMonitor(0.5, 0.1, 10*time.Second)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	monitor.Observe(start, 1.5, nil)
	if monitor.Observe(start.Add(5*time.Second), 0.05, nil) {
		t.Fatal("monitor completed too early")
	}
	if !monitor.Observe(start.Add(15*time.Second), 0.05, nil) {
		t.Fatal("monitor did not complete after inactive duration")
	}
}

func TestCurrentMonitorHysteresisAndErrorPause(t *testing.T) {
	monitor := NewCurrentMonitor(0.5, 0.1, 10*time.Second)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	monitor.Observe(start, 1.5, nil)
	monitor.Observe(start.Add(1*time.Second), 0.05, nil)
	monitor.Observe(start.Add(5*time.Second), 0.2, nil)
	if monitor.State() != inactive {
		t.Fatalf("state in hysteresis band = %v, want inactive", monitor.State())
	}
	monitor.Observe(start.Add(6*time.Second), 0, context.Canceled)
	if monitor.Observe(start.Add(7*time.Second), 0, nil) {
		t.Fatal("monitor counted the failed sample toward completion")
	}
	if monitor.Observe(start.Add(11*time.Second), 0, nil) {
		t.Fatal("monitor counted the error gap toward completion")
	}
	if !monitor.Observe(start.Add(13*time.Second), 0, nil) {
		t.Fatal("monitor did not complete after remaining valid inactive time")
	}
}

func TestLoadConfigRoleAndDefaults(t *testing.T) {
	values := map[string]string{
		"APPLIANCE_ROLE": "dryer",
		"API_SERVER_URL": "http://api:8001",
		"SHELLY_URL":     "http://shelly.local",
	}
	config, err := loadConfig(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	if config.Role != "dryer" {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestLoadUsers(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`[{"id":7,"phone":"+10000000000"}]`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	users, err := loadUsers(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if users[7] != "+10000000000" {
		t.Fatalf("unexpected users: %#v", users)
	}
}
