package userinfo

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
)

type UserInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Phone string `json:"phone"`
}

var users []UserInfo

// LoadConfig reads and parses the config.json file
func LoadConfig(configPath string) error {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file at %s: %w", configPath, err)
	}

	err = json.Unmarshal(data, &users)
	if err != nil {
		return fmt.Errorf("failed to parse config.json: %w", err)
	}

	if len(users) == 0 {
		return fmt.Errorf("config.json must contain at least 1 user")
	}

	if len(users) > 6 {
		return fmt.Errorf("config.json cannot contain more than 6 users")
	}

	// Validate each user has required fields
	for i, user := range users {
		if user.ID == 0 || user.Name == "" || user.Color == "" || user.Phone == "" {
			return fmt.Errorf("user at index %d is missing required fields (id, name, color, phone)", i)
		}
	}

	log.Printf("Successfully loaded %d users from config.json", len(users))
	return nil
}

// GetUserInfo retrieves user info by numeric ID (1-6)
func GetUserInfo(userID int) (UserInfo, error) {
	for _, user := range users {
		if user.ID == userID {
			return user, nil
		}
	}
	return UserInfo{}, fmt.Errorf("user with ID %d not found", userID)
}

// GetAllUsers returns all configured users
func GetAllUsers() []UserInfo {
	return users
}

// GetUserCount returns the number of configured users
func GetUserCount() int {
	return len(users)
}

