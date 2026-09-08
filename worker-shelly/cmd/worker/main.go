package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Role              string
	APIServerURL      string
	ShellyURL         string
	ShellyUsername    string
	ShellyPassword    string
	PollInterval      time.Duration
	ActiveThreshold   float64
	InactiveThreshold float64
	InactiveDuration  time.Duration
	HTTPTimeout       time.Duration
	ListenAddr        string
	ConfigPath        string
	SendSMSURL        string
	SMSUser           string
	SMSPassword       string
}

func loadConfig(getenv func(string) string) (Config, error) {
	config := Config{
		Role:              getenv("APPLIANCE_ROLE"),
		APIServerURL:      getenv("API_SERVER_URL"),
		ShellyURL:         getenv("SHELLY_URL"),
		ShellyUsername:    getenv("SHELLY_USERNAME"),
		ShellyPassword:    getenv("SHELLY_PASSWORD"),
		PollInterval:      durationEnv(getenv, "SHELLY_POLL_INTERVAL_SECONDS", 5*time.Second),
		ActiveThreshold:   floatEnv(getenv, "CURRENT_ACTIVE_THRESHOLD_A", 0.5),
		InactiveThreshold: floatEnv(getenv, "CURRENT_INACTIVE_THRESHOLD_A", 0.1),
		InactiveDuration:  durationEnv(getenv, "CURRENT_INACTIVE_DURATION_SECONDS", 5*time.Minute),
		HTTPTimeout:       durationEnv(getenv, "HTTP_TIMEOUT_SECONDS", 10*time.Second),
		ListenAddr:        getenv("LISTEN_ADDR"),
		ConfigPath:        getenv("CONFIG_PATH"),
		SendSMSURL:        getenv("SEND_SMS_URL"),
		SMSUser:           getenv("SMS_USER"),
		SMSPassword:       getenv("SMS_PASSWORD"),
	}
	if config.Role == "" {
		config.Role = "washer"
	}
	if config.ListenAddr == "" {
		config.ListenAddr = ":8005"
	}
	if config.ConfigPath == "" {
		config.ConfigPath = "/config/config.json"
	}
	if config.APIServerURL == "" || config.ShellyURL == "" {
		return Config{}, errors.New("API_SERVER_URL and SHELLY_URL are required")
	}
	if config.Role != "washer" && config.Role != "dryer" {
		return Config{}, fmt.Errorf("APPLIANCE_ROLE must be washer or dryer, got %q", config.Role)
	}
	if config.PollInterval <= 0 || config.HTTPTimeout <= 0 || config.InactiveDuration <= 0 {
		return Config{}, errors.New("poll interval, HTTP timeout, and inactive duration must be positive")
	}
	if config.InactiveThreshold < 0 || config.ActiveThreshold <= config.InactiveThreshold {
		return Config{}, errors.New("active threshold must be greater than or equal to inactive threshold, and inactive threshold cannot be negative")
	}
	return config, nil
}

func durationEnv(getenv func(string) string, key string, fallback time.Duration) time.Duration {
	value := getenv(key)
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		log.Printf("invalid %s=%q; using %s", key, value, fallback)
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func floatEnv(getenv func(string) string, key string, fallback float64) float64 {
	value := getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("invalid %s=%q; using %v", key, value, fallback)
		return fallback
	}
	return parsed
}

type ShellyClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

func (client *ShellyClient) ReadCurrent(ctx context.Context) (float64, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return 0, fmt.Errorf("parse Shelly URL: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, "/rpc/PM1.GetStatus")
	query := endpoint.Query()
	query.Set("id", "0")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("create Shelly request: %w", err)
	}
	if client.username != "" {
		request.SetBasicAuth(client.username, client.password)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("request Shelly status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("Shelly status returned %s", response.Status)
	}

	var status struct {
		Current *float64 `json:"current"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return 0, fmt.Errorf("decode Shelly status: %w", err)
	}
	if status.Current == nil {
		return 0, errors.New("Shelly PM response missing current")
	}
	return *status.Current, nil
}

type CurrentState int

const (
	unknown CurrentState = iota
	active
	inactive
)

type CurrentMonitor struct {
	activeThreshold   float64
	inactiveThreshold float64
	inactiveDuration  time.Duration
	state             CurrentState
	activeObserved    bool
	inactiveElapsed   time.Duration
	lastGoodAt        time.Time
	lastObservationOK bool
	lastProgress      int
}

func NewCurrentMonitor(activeThreshold, inactiveThreshold float64, inactiveDuration time.Duration) *CurrentMonitor {
	return &CurrentMonitor{
		activeThreshold:   activeThreshold,
		inactiveThreshold: inactiveThreshold,
		inactiveDuration:  inactiveDuration,
	}
}

func (monitor *CurrentMonitor) Reset() {
	monitor.state = unknown
	monitor.activeObserved = false
	monitor.inactiveElapsed = 0
	monitor.lastGoodAt = time.Time{}
	monitor.lastObservationOK = false
	monitor.lastProgress = 0
}

func (monitor *CurrentMonitor) Observe(now time.Time, amps float64, err error) bool {
	if err != nil {
		monitor.lastObservationOK = false
		return false
	}

	if monitor.lastObservationOK && !monitor.lastGoodAt.IsZero() {
		elapsed := now.Sub(monitor.lastGoodAt)
		if elapsed > 0 && monitor.state == inactive {
			monitor.inactiveElapsed += elapsed
		}
	}
	monitor.lastGoodAt = now
	monitor.lastObservationOK = true

	switch {
	case amps >= monitor.activeThreshold:
		monitor.state = active
		monitor.activeObserved = true
		monitor.inactiveElapsed = 0
	case amps <= monitor.inactiveThreshold:
		monitor.state = inactive
	case monitor.state == unknown:
		monitor.state = inactive
	}

	return monitor.activeObserved && monitor.state == inactive && monitor.inactiveElapsed >= monitor.inactiveDuration
}

func (monitor *CurrentMonitor) State() CurrentState {
	return monitor.state
}

func currentStateName(state CurrentState) string {
	switch state {
	case active:
		return "active"
	case inactive:
		return "inactive"
	default:
		return "unknown"
	}
}

type agentStatus struct {
	Status string `json:"status"`
	User   string `json:"user"`
}

type User struct {
	ID    int    `json:"id"`
	Phone string `json:"phone"`
}

func loadUsers(configPath string) (map[int]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	phones := make(map[int]string, len(users))
	for _, user := range users {
		phones[user.ID] = user.Phone
	}
	return phones, nil
}

type Worker struct {
	config      Config
	httpClient  *http.Client
	shelly      *ShellyClient
	users       map[int]string
	monitor     *CurrentMonitor
	lastStatus  string
	statusMutex sync.Mutex
}

func (worker *Worker) route(suffix string) string {
	return strings.TrimRight(worker.config.APIServerURL, "/") + "/" + worker.config.Role + "/" + suffix
}

func (worker *Worker) getStatus(ctx context.Context) (agentStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, worker.route("getAgentStatus"), nil)
	if err != nil {
		return agentStatus{}, err
	}
	response, err := worker.httpClient.Do(request)
	if err != nil {
		return agentStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return agentStatus{}, fmt.Errorf("agent status returned %s", response.Status)
	}
	var status agentStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return agentStatus{}, err
	}
	return status, nil
}

func (worker *Worker) checkIn(ctx context.Context, sensorOK bool) error {
	payload, _ := json.Marshal(map[string]bool{"sensorOk": sensorOK})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, worker.route("checkin"), strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := worker.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("check-in returned %s", response.Status)
	}
	return nil
}

func (worker *Worker) setIdle(ctx context.Context) error {
	payload := strings.NewReader(`{"status":"idle"}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, worker.route("setAgentStatus"), payload)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := worker.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("set idle returned %s", response.Status)
	}
	return nil
}

func (worker *Worker) notify(ctx context.Context, user string) error {
	if worker.config.SendSMSURL == "" {
		log.Printf("SEND_SMS_URL is not configured; skipping %s notification", worker.config.Role)
		return nil
	}
	userID, err := strconv.Atoi(user)
	if err != nil {
		return fmt.Errorf("parse monitoring user %q: %w", user, err)
	}
	phone := worker.users[userID]
	if phone == "" {
		return fmt.Errorf("no phone configured for user %d", userID)
	}
	message := strings.ToUpper(worker.config.Role[:1]) + worker.config.Role[1:] + " has finished running"
	payload, _ := json.Marshal(map[string]interface{}{
		"message":      message,
		"phoneNumbers": []string{phone},
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, worker.config.SendSMSURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(worker.config.SMSUser, worker.config.SMSPassword)
	response, err := worker.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("SMS request returned %s", response.Status)
	}
	return nil
}

func (worker *Worker) run(ctx context.Context) {
	ticker := time.NewTicker(worker.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			worker.tick(ctx, now)
		}
	}
}

func (worker *Worker) tick(ctx context.Context, now time.Time) {
	status, err := worker.getStatus(ctx)
	if err != nil {
		log.Printf("get %s status: %v", worker.config.Role, err)
		return
	}
	if worker.lastStatus != "" && worker.lastStatus != status.Status {
		log.Printf("status transition role=%s from=%s to=%s user=%s", worker.config.Role, worker.lastStatus, status.Status, status.User)
	}
	amps, readErr := worker.shelly.ReadCurrent(ctx)
	if err := worker.checkIn(ctx, readErr == nil); err != nil {
		log.Printf("check in %s worker: %v", worker.config.Role, err)
	}
	if readErr != nil {
		log.Printf("read Shelly current: %v", readErr)
		if status.Status != "monitor" {
			worker.monitor.Reset()
		}
		return
	}
	if status.Status != "monitor" {
		if worker.lastStatus == "monitor" {
			worker.monitor.Reset()
			log.Printf("monitor reset role=%s reason=status_%s", worker.config.Role, status.Status)
		}
		worker.lastStatus = status.Status
		return
	}
	if worker.lastStatus != "monitor" {
		worker.monitor.Reset()
		worker.lastStatus = "monitor"
		log.Printf("monitor started role=%s user=%s", worker.config.Role, status.User)
	}
	previousState := worker.monitor.State()
	previousElapsed := worker.monitor.inactiveElapsed
	completed := worker.monitor.Observe(now, amps, nil)
	currentState := worker.monitor.State()
	if previousState != currentState {
		log.Printf("current state transition role=%s from=%s to=%s amps=%.3f", worker.config.Role, currentStateName(previousState), currentStateName(currentState), amps)
		if previousState == active && currentState == inactive {
			log.Printf("inactive timer started role=%s amps=%.3f required=%s", worker.config.Role, amps, worker.monitor.inactiveDuration)
		}
	}
	if currentState == active && previousElapsed > 0 {
		log.Printf("inactive timer reset role=%s reason=active_current amps=%.3f previous_elapsed=%s", worker.config.Role, amps, previousElapsed)
	}
	if !completed && worker.monitor.activeObserved && currentState == inactive {
		progress := int(worker.monitor.inactiveElapsed * 100 / worker.monitor.inactiveDuration)
		milestone := (progress / 25) * 25
		if milestone > 75 {
			milestone = 75
		}
		if milestone >= 25 && milestone > worker.monitor.lastProgress {
			remaining := worker.monitor.inactiveDuration - worker.monitor.inactiveElapsed
			log.Printf("inactive progress role=%s elapsed=%s remaining=%s percent=%d", worker.config.Role, worker.monitor.inactiveElapsed, remaining, milestone)
			worker.monitor.lastProgress = milestone
		}
	}
	if completed {
		log.Printf("inactive threshold reached role=%s inactive_for=%s threshold=%s", worker.config.Role, worker.monitor.inactiveElapsed, worker.monitor.inactiveDuration)
		if err := worker.setIdle(ctx); err != nil {
			log.Printf("set %s idle: %v", worker.config.Role, err)
			return
		}
		log.Printf("status transition role=%s from=monitor to=idle reason=inactive_timeout inactive_for=%s", worker.config.Role, worker.monitor.inactiveElapsed)
		if err := worker.notify(ctx, status.User); err != nil {
			log.Printf("completion notification failed role=%s user=%s err=%v", worker.config.Role, status.User, err)
		} else if worker.config.SendSMSURL != "" {
			log.Printf("completion notification sent role=%s user=%s", worker.config.Role, status.User)
		}
		worker.monitor.Reset()
		worker.lastStatus = "idle"
	}
}

func main() {
	config, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	users, err := loadUsers(config.ConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	httpClient := &http.Client{Timeout: config.HTTPTimeout}
	worker := &Worker{
		config:     config,
		httpClient: httpClient,
		users:      users,
		monitor:    NewCurrentMonitor(config.ActiveThreshold, config.InactiveThreshold, config.InactiveDuration),
		shelly: &ShellyClient{
			baseURL:  config.ShellyURL,
			username: config.ShellyUsername,
			password: config.ShellyPassword,
			client:   httpClient,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":"ok"}`))
	})
	server := &http.Server{Addr: config.ListenAddr, Handler: mux}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.run(ctx)
	log.Printf("starting Shelly worker for %s on %s", config.Role, config.ListenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
