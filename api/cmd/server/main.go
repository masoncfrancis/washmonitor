package main

import (
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	"github.com/masoncfrancis/washmonitor-agent/api/internal/config"
	"github.com/masoncfrancis/washmonitor-agent/api/internal/userinfo"
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
	washerLastSeen time.Time
	dryerLastSeen  time.Time
	agentMutex     sync.Mutex
)

func main() {
	// Load .env file if present, but do not overwrite already-set env vars
	err := godotenv.Overload(".env")
	if err != nil {
		// Only log if the .env file is missing for info, not as an error
		log.Println("No .env file found or error loading .env (this is fine if env vars are set elsewhere):", err)
	}

	USER1_NAME_DEFAULT := "User1"
	USER1_COLOR_DEFAULT := "#3b82f6" // blue-500 as hex
	USER2_NAME_DEFAULT := "User2"
	USER2_COLOR_DEFAULT := "#22c55e" // green-500 as hex

	// Set default values for user env vars if not set
	config.SetDefaultEnv("USER1_NAME", USER1_NAME_DEFAULT)
	config.SetDefaultEnv("USER1_COLOR", USER1_COLOR_DEFAULT)
	config.SetDefaultEnv("USER2_NAME", USER2_NAME_DEFAULT)
	config.SetDefaultEnv("USER2_COLOR", USER2_COLOR_DEFAULT)

	// Warn if any default values are being used
	config.WarnIfDefaultUsed("USER1_NAME", USER1_NAME_DEFAULT)
	config.WarnIfDefaultUsed("USER1_COLOR", USER1_COLOR_DEFAULT)
	config.WarnIfDefaultUsed("USER2_NAME", USER2_NAME_DEFAULT)
	config.WarnIfDefaultUsed("USER2_COLOR", USER2_COLOR_DEFAULT)

	app := fiber.New()

	// Permitir CORS para todos los orígenes
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
	}))

	// Endpoint de health check
	app.Get("/health", func(c *fiber.Ctx) error {
		// Determine online status: offline if last seen > 7 minutes
		agentMutex.Lock()
		ws := washerLastSeen
		ds := dryerLastSeen
		agentMutex.Unlock()

		sevenMin := 7 * time.Minute
		washerOnline := false
		dryerOnline := false
		var washerLast string
		var dryerLast string
		if !ws.IsZero() {
			washerLast = ws.UTC().Format(time.RFC3339)
			if time.Since(ws) <= sevenMin {
				washerOnline = true
			}
		}
		if !ds.IsZero() {
			dryerLast = ds.UTC().Format(time.RFC3339)
			if time.Since(ds) <= sevenMin {
				dryerOnline = true
			}
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"api": fiber.Map{
				"status": "ok",
			},
			"washer": fiber.Map{
				"online":   washerOnline,
				"lastSeen": washerLast,
			},
			"dryer": fiber.Map{
				"online":   dryerOnline,
				"lastSeen": dryerLast,
			},
		})
	})

	app.Post("/washer/setAgentStatus", func(c *fiber.Ctx) error {
		var body AgentState
		if err := c.BodyParser(&body); err != nil {
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
		if body.Status == "idle" {
			washerAgentState.Status = "idle"
			washerAgentState.User = ""
		} else {
			washerAgentState.Status = "monitor"
			washerAgentState.User = body.User
		}
		// Update last-seen timestamp when agent submits status
		agentMutex.Lock()
		washerLastSeen = time.Now()
		agentMutex.Unlock()
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Agent status set successfully",
			"status":  washerAgentState.Status,
			"user":    washerAgentState.User,
		})
	})

	app.Get("/washer/getAgentStatus", func(c *fiber.Ctx) error {
		user := washerAgentState.User
		if washerAgentState.Status == "idle" {
			user = ""
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": washerAgentState.Status,
			"user":   user,
		})
	})

	app.Post("/dryer/setAgentStatus", func(c *fiber.Ctx) error {
		var body AgentState
		if err := c.BodyParser(&body); err != nil {
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
		if body.Status == "idle" {
			dryerAgentState.Status = "idle"
			dryerAgentState.User = ""
		} else {
			dryerAgentState.Status = "monitor"
			dryerAgentState.User = body.User
		}
		// Update last-seen timestamp when agent submits status
		agentMutex.Lock()
		dryerLastSeen = time.Now()
		agentMutex.Unlock()
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Agent status set successfully",
			"status":  dryerAgentState.Status,
			"user":    dryerAgentState.User,
		})
	})

	app.Get("/dryer/getAgentStatus", func(c *fiber.Ctx) error {
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
	app.Get("/users/names", func(c *fiber.Ctx) error {
		user1 := userinfo.GetUserInfo(1)
		user2 := userinfo.GetUserInfo(2)
		// Ensure color is hex (if not, fallback to default)
		if len(user1.Color) == 0 || user1.Color[0] != '#' {
			user1.Color = USER1_COLOR_DEFAULT
		}
		if len(user2.Color) == 0 || user2.Color[0] != '#' {
			user2.Color = USER2_COLOR_DEFAULT
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"user1": user1,
			"user2": user2,
		})
	})

	app.Listen(":8001")
}
