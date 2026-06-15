package main

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
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

	// Load config.json with user data
	err = userinfo.LoadConfig("/config/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

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

		sevenMin := 4 * time.Minute
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

	// Top-level check-in endpoints for agents (heartbeats)
	app.Post("/washer/checkin", func(c *fiber.Ctx) error {
		agentMutex.Lock()
		washerLastSeen = time.Now()
		agentMutex.Unlock()
		return c.SendStatus(fiber.StatusOK)
	})

	app.Post("/dryer/checkin", func(c *fiber.Ctx) error {
		agentMutex.Lock()
		dryerLastSeen = time.Now()
		agentMutex.Unlock()
		return c.SendStatus(fiber.StatusOK)
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

	app.Listen(":8001")
}
