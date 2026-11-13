package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiberv3"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/joho/godotenv"
	"github.com/masoncfrancis/washmonitor-agent/api/internal/userinfo"
	logrus "github.com/sirupsen/logrus"
)

type AgentState struct {
	Status string `json:"status"`
	User   string `json:"user"`
}

var washerAgentState = AgentState{
	Status: "idle",
	User:   "",
}

var dryerAgentState = AgentState{
	Status: "idle",
	User:   "",
}
var (
	washerLastSeen        time.Time
	dryerLastSeen         time.Time
	washerSensorOk        bool
	washerSensorLastSeen  time.Time
	dryerSensorOk         bool
	dryerSensorLastSeen   time.Time
	agentMutex            sync.Mutex
	offlineThreshold      = 30 * time.Second
)

func main() {
	// Load .env file if present, but do not overwrite already-set env vars
	err := godotenv.Overload(".env")
	if err != nil {
		// Only log if the .env file is missing for info, not as an error
		log.Println("No .env file found or error loading .env (this is fine if env vars are set elsewhere):", err)
	}

	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		err = sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Debug:            false,
			SendDefaultPII:   true,
			EnableTracing:    true,
			TracesSampleRate: 1.0,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
		logger := logrus.New()
		logger.SetFormatter(&logrus.JSONFormatter{})
		logger.SetOutput(os.Stdout)
		logger.SetLevel(logrus.InfoLevel)
		hook, err := sentrylogrus.NewLogHook([]logrus.Level{logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel}, sentry.ClientOptions{})
		if err != nil {
			log.Printf("sentry log hook init failed: %v", err)
		} else {
			logger.AddHook(hook)
		}
		log.SetOutput(logger.Writer())
		log.SetFlags(0)
		defer sentry.Flush(2 * time.Second)
		logger.Info("Sentry initialized using SENTRY_DSN")
	} else {
		log.Println("SENTRY_DSN not set; Sentry disabled")
	}

	// Load config.json with user data
	err = userinfo.LoadConfig("/config/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Allow overriding the agent offline threshold via env (seconds)
	if secondsStr := os.Getenv("OFFLINE_THRESHOLD_SECONDS"); secondsStr != "" {
		if seconds, err := strconv.Atoi(secondsStr); err == nil && seconds > 0 {
			offlineThreshold = time.Duration(seconds) * time.Second
		} else {
			log.Printf("Invalid OFFLINE_THRESHOLD_SECONDS %q, using default", secondsStr)
		}
	}
	log.Printf("Agent offline threshold: %v", offlineThreshold)

	app := newApp()
	if err := app.Listen(":8001"); err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Failed to start API server: %v", err)
	}
}

func newApp() *fiber.App {
	app := fiber.New()
	app.Use(sentryfiber.New(sentryfiber.Options{Repanic: true}))

	// Permitir CORS para todos los orígenes
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
	}))

	// Endpoint de health check
	app.Get("/health", func(c fiber.Ctx) error {
		// Determine online status: offline if last seen > offlineThreshold
		agentMutex.Lock()
		ws := washerLastSeen
		ds := dryerLastSeen
		wso := washerSensorOk
		wsl := washerSensorLastSeen
		dso := dryerSensorOk
		dsl := dryerSensorLastSeen
		agentMutex.Unlock()

		washerOnline := false
		dryerOnline := false
		var washerLast string
		var dryerLast string
		if !ws.IsZero() {
			washerLast = ws.UTC().Format(time.RFC3339)
			if time.Since(ws) <= offlineThreshold {
				washerOnline = true
			}
		}
		if !ds.IsZero() {
			dryerLast = ds.UTC().Format(time.RFC3339)
			if time.Since(ds) <= offlineThreshold {
				dryerOnline = true
			}
		}

		washerSensorOnline := !wsl.IsZero() && time.Since(wsl) <= offlineThreshold
		dryerSensorOnline := !dsl.IsZero() && time.Since(dsl) <= offlineThreshold
		var washerSensorLast string
		var dryerSensorLast string
		if !wsl.IsZero() {
			washerSensorLast = wsl.UTC().Format(time.RFC3339)
		}
		if !dsl.IsZero() {
			dryerSensorLast = dsl.UTC().Format(time.RFC3339)
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"api": fiber.Map{
				"status": "ok",
			},
			"washer": fiber.Map{
				"online":   washerOnline,
				"lastSeen": washerLast,
				"sensor": fiber.Map{
					"online":   washerSensorOnline,
					"ok":       wso,
					"lastSeen": washerSensorLast,
				},
			},
			"dryer": fiber.Map{
				"online":   dryerOnline,
				"lastSeen": dryerLast,
				"sensor": fiber.Map{
					"online":   dryerSensorOnline,
					"ok":       dso,
					"lastSeen": dryerSensorLast,
				},
			},
		})
	})

	app.Post("/washer/setAgentStatus", func(c fiber.Ctx) error {
		var body AgentState
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Malformed request",
			})
		}
		if body.Status != "monitor" && body.Status != "idle" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Status must be 'monitor' or 'idle'",
			})
		}
		if body.Status == "monitor" && body.User == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "User is required when status is 'monitor'",
			})
		}
		if body.Status == "monitor" {
			userID, err := strconv.Atoi(body.User)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "User must be a numeric ID",
				})
			}
			if _, err := userinfo.GetUserInfo(userID); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "User ID is not configured",
				})
			}
		}
		if body.Status == "idle" {
			washerAgentState.Status = "idle"
			washerAgentState.User = ""
		} else {
			washerAgentState.Status = "monitor"
			washerAgentState.User = body.User
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Agent status set successfully",
			"status":  washerAgentState.Status,
			"user":    washerAgentState.User,
		})
	})

	app.Get("/washer/getAgentStatus", func(c fiber.Ctx) error {
		user := washerAgentState.User
		if washerAgentState.Status == "idle" {
			user = ""
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": washerAgentState.Status,
			"user":   user,
		})
	})

	app.Post("/dryer/setAgentStatus", func(c fiber.Ctx) error {
		var body AgentState
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Malformed request",
			})
		}
		if body.Status != "monitor" && body.Status != "idle" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Status must be 'monitor' or 'idle'",
			})
		}
		if body.Status == "monitor" && body.User == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "User is required when status is 'monitor'",
			})
		}
		if body.Status == "monitor" {
			userID, err := strconv.Atoi(body.User)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "User must be a numeric ID",
				})
			}
			if _, err := userinfo.GetUserInfo(userID); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "User ID is not configured",
				})
			}
		}
		if body.Status == "idle" {
			dryerAgentState.Status = "idle"
			dryerAgentState.User = ""
		} else {
			dryerAgentState.Status = "monitor"
			dryerAgentState.User = body.User
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Agent status set successfully",
			"status":  dryerAgentState.Status,
			"user":    dryerAgentState.User,
		})
	})

	// Top-level check-in endpoints for agents (heartbeats)
	app.Post("/washer/checkin", func(c fiber.Ctx) error {
		var body struct {
			SensorOk bool `json:"sensorOk"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "sensorOk required",
			})
		}
		agentMutex.Lock()
		washerLastSeen = time.Now()
		washerSensorOk = body.SensorOk
		washerSensorLastSeen = time.Now()
		agentMutex.Unlock()
		return c.SendStatus(fiber.StatusOK)
	})

	app.Post("/dryer/checkin", func(c fiber.Ctx) error {
		var body struct {
			SensorOk bool `json:"sensorOk"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "sensorOk required",
			})
		}
		agentMutex.Lock()
		dryerLastSeen = time.Now()
		dryerSensorOk = body.SensorOk
		dryerSensorLastSeen = time.Now()
		agentMutex.Unlock()
		return c.SendStatus(fiber.StatusOK)
	})

	app.Get("/dryer/getAgentStatus", func(c fiber.Ctx) error {
		user := dryerAgentState.User
		if dryerAgentState.Status == "idle" {
			user = ""
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": dryerAgentState.Status,
			"user":   user,
		})
	})

	// Endpoint for user names and colors
	app.Get("/users/names", func(c fiber.Ctx) error {
		users := userinfo.GetAllUsers()
		response := make(map[string]fiber.Map)
		for _, user := range users {
			response[fmt.Sprintf("%d", user.ID)] = fiber.Map{
				"name":  user.Name,
				"color": user.Color,
			}
		}
		return c.Status(fiber.StatusOK).JSON(response)
	})

	return app
}
