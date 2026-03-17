package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

// StateSubmission holds a state and its timestamp

var (
	stateHistory           []StateSubmission
	stateMutex             sync.Mutex
	monitorActive          bool
	monitorCancel          chan struct{}
	stationaryTimer        time.Duration
	lastStationaryState    bool
	lastFailedCheckinPrint time.Time
	lastFailedCheckinMutex sync.Mutex
)

type StateSubmission struct {
	State     string // 'vibrating' or 'stationary'
	Timestamp time.Time
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, proceeding with environment variables.")
	}

	API_SERVER_URL := os.Getenv("API_SERVER_URL")
	if API_SERVER_URL == "" {
		panic("API_SERVER_URL environment variable is not set")
	}

	app := fiber.New()
	app.Use(cors.New())

	app.Get("/status", func(c *fiber.Ctx) error {
		log.Printf("Received %s request at %s", c.Method(), c.Path())
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ok",
		})
	})

	app.Post("/submitState", func(c *fiber.Ctx) error {
		log.Printf("Received %s request at %s", c.Method(), c.Path())
		var req struct {
			State string `json:"state"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}
		if req.State != "vibrating" && req.State != "stationary" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "State must be 'vibrating' or 'stationary'"})
		}
		stateMutex.Lock()
		stateHistory = append(stateHistory, StateSubmission{State: req.State, Timestamp: time.Now()})
		stateMutex.Unlock()
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "State submitted"})
	})

	c := cron.New()
	c.AddFunc("@every 5s", func() {
		// Get agent status from API server
		resp, err := http.Get(API_SERVER_URL + "/dryer/getAgentStatus")
		if err != nil {
			log.Printf("Failed to get agent status: %v", err)
			return
		}
		defer resp.Body.Close()

		var agentStatus struct {
			Status string `json:"status"`
			User   string `json:"user"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&agentStatus); err != nil {
			log.Printf("Failed to decode agent status response: %v", err)
			return
		}
		log.Printf("Agent status: %s", agentStatus.Status)

		// Send a heartbeat/check-in to the API so server records last-seen
		go func() {
			resp, err := http.Post(API_SERVER_URL+"/dryer/checkin", "application/json", bytes.NewBuffer([]byte("{}")))
			if err != nil {
				// print at most every 30s to avoid flooding logs
				lastFailedCheckinMutex.Lock()
				if time.Since(lastFailedCheckinPrint) >= 30*time.Second {
					log.Printf("Check-in request failed: %v", err)
					lastFailedCheckinPrint = time.Now()
				}
				lastFailedCheckinMutex.Unlock()
				return
			}
			resp.Body.Close()
		}()

		if agentStatus.Status == "monitor" {
			if !monitorActive {
				monitorActive = true
				if monitorCancel != nil {
					close(monitorCancel)
				}
				monitorCancel = make(chan struct{})
				go func(user string, cancelChan chan struct{}) {
					log.Println("Starting stationary timer goroutine (reset on state change)...")
					ticker := time.NewTicker(1 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-cancelChan:
							log.Println("Monitor observation cancelled (status changed or new monitor started)")
							monitorActive = false
							return
						case <-ticker.C:
							stateMutex.Lock()
							n := len(stateHistory)
							lastState := ""
							if n > 0 {
								lastState = stateHistory[n-1].State
							}
							if lastState == "stationary" {
								if !lastStationaryState {
									// Just became stationary, reset timer
									stationaryTimer = 0
									lastStationaryState = true
									log.Println("State became stationary, timer started/reset.")
								} else {
									stationaryTimer += time.Second
								}
								if stationaryTimer >= 5*time.Minute {
									log.Println("Stationary for 5 uninterrupted minutes. Notifying user and setting status to idle.")
									// Send POST request to API server to update status to 'idle'
									payload := map[string]string{"status": "idle"}
									payloadBytes, _ := json.Marshal(payload)
									resp, err := http.Post(API_SERVER_URL+"/dryer/setAgentStatus", "application/json",
										bytes.NewBuffer(payloadBytes))
									if err != nil {
										log.Printf("Failed to update status to 'idle': %v", err)
									} else {
										resp.Body.Close()
										if resp.StatusCode == http.StatusOK {
											log.Println("Successfully updated status to 'idle'")
										} else {
											log.Printf("Failed to update status to 'idle', server responded with status: %s", resp.Status)
										}
									}
									// Notify only the user who started monitoring
									var destinationNumber string
									switch user {
									case "user1":
										destinationNumber = os.Getenv("USER1_PHONE_NUMBER")
									case "user2":
										destinationNumber = os.Getenv("USER2_PHONE_NUMBER")
									default:
										log.Printf("Unknown user '%s', skipping SMS notification", user)
									}
									if destinationNumber != "" {
										smsURL := os.Getenv("SEND_SMS_URL")
										smsUser := os.Getenv("SMS_USER")
										smsPassword := os.Getenv("SMS_PASSWORD")
										smsPayload := map[string]interface{}{
											"message":      "✅ Dryer has finished running",
											"phoneNumbers": []string{destinationNumber},
										}
										smsPayloadBytes, _ := json.Marshal(smsPayload)
										req, err := http.NewRequest("POST", smsURL, bytes.NewBuffer(smsPayloadBytes))
										if err != nil {
											log.Printf("Failed to create SMS request: %v", err)
										} else {
											req.Header.Set("Content-Type", "application/json")
											req.SetBasicAuth(smsUser, smsPassword)
											client := &http.Client{}
											resp, err := client.Do(req)
											if err != nil {
												log.Printf("Failed to send SMS: %v", err)
											} else {
												defer resp.Body.Close()
												if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
													log.Println("SMS sent successfully")
												} else {
													log.Printf("Failed to send SMS: %d - %s", resp.StatusCode, resp.Status)
												}
											}
										}
									}
									// Reset timer and state for next monitoring session
									stationaryTimer = 0
									lastStationaryState = false
									monitorActive = false
									stateMutex.Unlock()
									return
								}
							} else {
								if lastStationaryState {
									// State changed away from stationary, reset timer
									stationaryTimer = 0
									lastStationaryState = false
									log.Println("State changed from stationary, timer reset.")
								}
							}
							stateMutex.Unlock()
						}
					}
				}(agentStatus.User, monitorCancel)
			}
		} else {
			if monitorActive {
				// Cancel any running monitor goroutine
				if monitorCancel != nil {
					close(monitorCancel)
					monitorCancel = nil
				}
				monitorActive = false
				log.Println("Timer goroutine cancelled due to status change to idle or non-monitor.")
			}
			// Reset timer and state when not monitoring
			stateMutex.Lock()
			stationaryTimer = 0
			lastStationaryState = false
			stateMutex.Unlock()
			log.Printf("Agent status is '%s', timer is not running.", agentStatus.Status)
		}
	})

	// Prune old state submissions every 10 minutes
	c.AddFunc("@every 10m", func() {
		stateMutex.Lock()
		cutoff := time.Now().Add(-5 * time.Minute)
		var pruned []StateSubmission
		for _, s := range stateHistory {
			if s.Timestamp.After(cutoff) {
				pruned = append(pruned, s)
			}
		}
		stateHistory = pruned
		stateMutex.Unlock()
		log.Println("Pruned old state submissions, kept:", len(stateHistory))
	})
	c.Start()
	app.Listen(":8005")
}

// isStateConsistent checks if the state has been consistent for the last 5 minutes
// with at least one record every 15 seconds and no more than 15 seconds between records.
// If the service has not been running for 5 minutes, it returns early.
func isStateConsistent(history []StateSubmission, now time.Time, serviceStartTime time.Time) (bool, string, string) {
	const (
		window     = 5 * time.Minute
		maxGap     = 15 * time.Second
		minRecords = int(window / maxGap)
	)
	// If service hasn't been running for 5 minutes, skip check
	if now.Sub(serviceStartTime) < window {
		return false, "", "Service has not been running for 5 minutes yet"
	}

	if len(history) == 0 {
		return false, "", "No state submissions available"
	}

	// Filter for last 5 minutes
	cutoff := now.Add(-window)
	var recent []StateSubmission
	for _, s := range history {
		if !s.Timestamp.Before(cutoff) {
			recent = append(recent, s)
		}
	}
	if len(recent) == 0 {
		return false, "", "No state submissions in the last 5 minutes"
	}

	// Check for gaps and consistency
	last := recent[0]
	state := last.State
	for i := 1; i < len(recent); i++ {
		if recent[i].State != state {
			return false, "", "State changed within the last 5 minutes"
		}
		if recent[i].Timestamp.Sub(last.Timestamp) > maxGap {
			return false, "", "Gap between submissions exceeds 15 seconds"
		}
		last = recent[i]
	}

	// Check if the first record covers the full window
	if now.Sub(recent[0].Timestamp) > window {
		return false, "", "Not enough data to cover the last 5 minutes"
	}

	// Optionally, check if there are enough records (not strictly required if gaps are checked)
	if len(recent) < minRecords {
		return false, "", "Not enough records for 5 minutes (should be ~20+)"
	}

	return true, state, ""
}
