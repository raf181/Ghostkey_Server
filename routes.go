// Package main declares the main package of the application
package main

// Import necessary packages
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Constants for file delivery status and retry settings
const (
	StatusPending    = "pending"       // Indicates a file is pending delivery
	StatusCompleted  = "completed"     // Indicates a file was delivered successfully
	StatusFailed     = "failed"        // Indicates a file delivery has failed
	MaxRetryAttempts = 5               // Maximum number of retry attempts for delivery
	RetryInterval    = 1 * time.Minute // Time interval between retry attempts
)

// authRequired checks for either session cookie or Basic Auth
func authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")
		authenticated := session.Get("authenticated")

		if userID != nil && authenticated != nil {
			c.Set("user_id", userID)
			c.Next()
			return
		}

		// If no valid session, check for Basic Auth
		username, password, hasAuth := c.Request.BasicAuth()
		if hasAuth {
			// Rate limiting check
			clientIP := c.ClientIP()
			if isRateLimited(clientIP, 10, time.Minute) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
				c.Abort()
				return
			}

			// Sanitize and validate input
			username = sanitizeUsername(username)
			if !validateInput(username) || !validateLength(username, 1, 50) {
				log.Printf("Security Warning: Invalid username format from IP %s: %s", clientIP, username)
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input format"})
				c.Abort()
				return
			}

			if !validateLength(password, 1, 100) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid password length"})
				c.Abort()
				return
			}

			// Verify credentials
			var user User
			if err := db.Where("username = ?", username).First(&user).Error; err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
				c.Abort()
				return
			}

			if !user.CheckPassword(password) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
				c.Abort()
				return
			}

			// Set user ID in context
			c.Set("user_id", user.ID)
			c.Next()
			return
		}

		// No valid authentication method found
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		c.Abort()
	}
}

// healthCheck responds with server status
func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"service": "Ghostkey_Server",
	})
}

// securityHeaders middleware adds security headers to all responses
func securityHeaders() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Prevent XSS attacks
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; font-src 'self'; object-src 'none'; media-src 'self'; form-action 'self'; base-uri 'self';")

		// Remove server information
		c.Header("Server", "")

		c.Next()
	})
}

// requestSizeLimit middleware limits request body size
func requestSizeLimit(maxSize int64) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		if c.Request.ContentLength > maxSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request too large"})
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	})
}

// registerRoutes sets up all the API endpoints for the server
func registerRoutes(r *gin.Engine) {
	// Add security middleware
	r.Use(securityHeaders())
	r.Use(requestSizeLimit(10 * 1024 * 1024)) // 10MB limit

	// Health check endpoint (no authentication required)
	r.GET("/", healthCheck) // Root path for basic connectivity check
	r.GET("/health", healthCheck)

	// Public routes (no authentication required)
	r.POST("/register_user", registerUser)
	r.POST("/login", login)
	r.GET("/get_command", getCommand)
	r.POST("/cargo_delivery", cargoDelivery)
	r.GET("/ws", handleWebSocket)              // WebSocket endpoint for real-time sync
	r.POST("/gossip", receiveGossip)           // Gossip endpoint for sync between nodes
	r.GET("/cluster/status", getClusterStatus) // Get cluster status

	// Authenticated routes (require valid session)
	authenticated := r.Group("/")
	authenticated.Use(authRequired())
	{
		authenticated.POST("/logout", logout)
		authenticated.POST("/register_device", registerDevice)
		authenticated.POST("/command", addCommand)
		authenticated.POST("/remove_command", removeCommand)
		authenticated.GET("/get_all_commands", getAllCommands)
		authenticated.GET("/active_boards", getActiveBoards)
		authenticated.POST("/register_mailer", registerMail)
		authenticated.POST("/loaded_command", updateLoadedCommands)
		authenticated.DELETE("/remove_device", removeDevice)
	}
}

// sanitizeInput cleans the input string to prevent injection attacks
func sanitizeInput(input string) string {
	input = strings.TrimSpace(input) // Remove leading/trailing whitespace

	// Limit input length to prevent buffer overflow attacks
	if len(input) > 1000 {
		input = input[:1000]
	}

	// Remove null bytes and control characters
	input = strings.ReplaceAll(input, "\x00", "")
	re := regexp.MustCompile(`[\x00-\x1f\x7f]`)
	input = re.ReplaceAllString(input, "")

	return input
}

// sanitizeAlphanumeric allows only alphanumeric characters, underscores, hyphens, and dots
func sanitizeAlphanumeric(input string) string {
	input = sanitizeInput(input)
	re := regexp.MustCompile(`[^\w.-]`)
	return re.ReplaceAllString(input, "")
}

// sanitizeUsername allows alphanumeric characters, underscores, hyphens, dots, and @ symbol
func sanitizeUsername(input string) string {
	input = sanitizeInput(input)
	re := regexp.MustCompile(`[^\w@.-]`)
	return re.ReplaceAllString(input, "")
}

// validateInput checks for common SQL injection patterns
func validateInput(input string) bool {
	// Common SQL injection patterns
	sqlPatterns := []string{
		`(?i)(union\s+select)`,
		`(?i)(select\s+.*\s+from)`,
		`(?i)(drop\s+table)`,
		`(?i)(delete\s+from)`,
		`(?i)(insert\s+into)`,
		`(?i)(update\s+.*\s+set)`,
		`(?i)(exec\s*\()`,
		`(?i)(script\s*>)`,
		`(?i)(<\s*script)`,
		`--`,
		`;`,
		`'.*'`,
		`".*"`,
		`\*`,
		`%`,
	}

	for _, pattern := range sqlPatterns {
		matched, _ := regexp.MatchString(pattern, input)
		if matched {
			log.Printf("Security Warning: Potential SQL injection detected: %s", input)
			return false
		}
	}
	return true
}

// validateLength validates input length constraints
func validateLength(input string, minLen, maxLen int) bool {
	length := len(input)
	return length >= minLen && length <= maxLen
}

// Rate limiting map to track requests per IP
var rateLimitMap = make(map[string][]time.Time)
var rateLimitMutex sync.RWMutex

// isRateLimited checks if an IP has exceeded rate limits
func isRateLimited(ip string, maxRequests int, timeWindow time.Duration) bool {
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()

	// Clean old entries
	if requests, exists := rateLimitMap[ip]; exists {
		var validRequests []time.Time
		for _, reqTime := range requests {
			if now.Sub(reqTime) <= timeWindow {
				validRequests = append(validRequests, reqTime)
			}
		}
		rateLimitMap[ip] = validRequests
	}

	// Check if rate limit is exceeded
	if len(rateLimitMap[ip]) >= maxRequests {
		return true
	}

	// Add current request
	rateLimitMap[ip] = append(rateLimitMap[ip], now)
	return false
}

// loadedCommand replaces existing commands for an ESP device with a new list
func loadedCommand(c *gin.Context) {
	var payload LoadedCommandPayload

	// Bind JSON payload to the struct
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate input
	if payload.EspID == "" || len(payload.Commands) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ESP ID and commands are required"})
		return
	}

	// Begin a database transaction
	tx := db.Begin()

	// Delete existing commands for the given ESP ID
	if err := tx.Where("esp_id = ?", payload.EspID).Delete(&Command{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete existing commands"})
		return
	}

	// Save new commands associated with the ESP ID
	for _, cmd := range payload.Commands {
		newCommand := Command{EspID: payload.EspID, Command: cmd}
		if err := tx.Create(&newCommand).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save commands"})
			return
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Commands saved successfully"})
}

// getLoadedCommand retrieves all commands associated with an ESP device
func getLoadedCommand(c *gin.Context) {
	espID := c.Query("esp_id")

	// Validate input
	if espID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ESP ID is required"})
		return
	}

	// Fetch commands from database
	var commands []Command
	if err := db.Where("esp_id = ?", espID).Find(&commands).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve commands"})
		return
	}

	// Extract command strings
	commandList := make([]string, len(commands))
	for i, cmd := range commands {
		commandList[i] = cmd.Command
	}

	// Return commands in JSON response
	c.JSON(http.StatusOK, gin.H{"esp_id": espID, "commands": commandList})
}

// registerUser handles the registration of a new user
func registerUser(c *gin.Context) {
	// Rate limiting check
	clientIP := c.ClientIP()
	if isRateLimited(clientIP, 5, time.Minute) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "Rate limit exceeded"})
		return
	}

	secretKey := c.PostForm("secret_key")
	expectedSecretKey := os.Getenv("SECRET_KEY") // Expected secret key from environment variables
	if expectedSecretKey == "" {
		expectedSecretKey = loadSecretFromFile()
	}

	// Validate secret key
	if secretKey != expectedSecretKey {
		log.Printf("Security Warning: Invalid secret key attempt from IP %s", clientIP)
		c.JSON(http.StatusForbidden, gin.H{"message": "Invalid secret key"})
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")

	// Validate input
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username and password are required"})
		return
	}

	// Enhanced input validation
	username = sanitizeUsername(username)
	if !validateInput(username) || !validateLength(username, 3, 50) {
		log.Printf("Security Warning: Invalid username format from IP %s: %s", clientIP, username)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid username format"})
		return
	}

	if !validateLength(password, 8, 100) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Password must be between 8 and 100 characters"})
		return
	}

	// Check if username already exists
	var user User
	if err := db.Where("username = ?", username).First(&user).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username already exists"})
		return
	}

	// Create new user
	newUser := User{Username: username}
	if err := newUser.SetPassword(password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to set password"})
		return
	}
	// Save user to database
	if err := db.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to register user"})
		return
	}

	// Publish the user change to the cluster
	publishUserChange(newUser, "create")

	c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
}

// login handles user login
func login(c *gin.Context) {
	// Rate limiting check
	clientIP := c.ClientIP()
	if isRateLimited(clientIP, 10, time.Minute) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "Rate limit exceeded"})
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")

	// Validate input
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username and password are required"})
		return
	}

	// Enhanced input validation
	username = sanitizeUsername(username)
	if !validateInput(username) || !validateLength(username, 1, 50) {
		log.Printf("Security Warning: Invalid username format from IP %s: %s", clientIP, username)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid input format"})
		return
	}

	if !validateLength(password, 1, 100) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid password length"})
		return
	}

	// Fetch user from database
	var user User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid username or password"})
		return
	}

	// Check password
	if !user.CheckPassword(password) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid username or password"})
		return
	}

	// Create session with maximum age and path settings
	session := sessions.Default(c)
	session.Options(sessions.Options{
		MaxAge:   3600 * 24, // 24 hours
		Path:     "/",
		HttpOnly: true,
	})
	session.Set("user_id", user.ID)
	session.Set("authenticated", true) // Add explicit authentication flag
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}

// logout handles user logout
func logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// registerDevice handles registration of a new ESP device
func registerDevice(c *gin.Context) {
	// Rate limiting check
	clientIP := c.ClientIP()
	if isRateLimited(clientIP, 20, time.Minute) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "Rate limit exceeded"})
		return
	}

	espID := c.PostForm("esp_id")
	espSecretKey := c.PostForm("esp_secret_key")

	// Validate input
	if espID == "" || espSecretKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
		return
	}

	// Enhanced input validation
	espID = sanitizeAlphanumeric(espID)
	espSecretKey = sanitizeAlphanumeric(espSecretKey)

	if !validateInput(espID) || !validateLength(espID, 3, 50) {
		log.Printf("Security Warning: Invalid ESP ID format from IP %s: %s", clientIP, espID)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID format"})
		return
	}

	if !validateInput(espSecretKey) || !validateLength(espSecretKey, 8, 100) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid secret key format"})
		return
	}

	// Check if device already exists
	var device ESPDevice
	if err := db.Where("esp_id = ?", espID).First(&device).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID already exists"})
		return
	}
	// Create new device
	newDevice := ESPDevice{EspID: espID, EspSecretKey: espSecretKey}
	if err := db.Create(&newDevice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to register ESP32"})
		return
	}

	// Publish the device change to the cluster
	publishDeviceChange(newDevice, "create")

	c.JSON(http.StatusOK, gin.H{"message": "ESP32 registered successfully", "esp_id": espID})
}

// removeDevice handles removal of an ESP device
func removeDevice(c *gin.Context) {
	// Rate limiting check
	clientIP := c.ClientIP()
	if isRateLimited(clientIP, 10, time.Minute) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "Rate limit exceeded"})
		return
	}

	espID := c.Query("esp_id")
	espSecretKey := c.Query("secret_key") // Changed from esp_secret_key to match test expectations

	// Validate parameters
	if espID == "" || espSecretKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
		return
	}

	// Enhanced input validation
	espID = sanitizeAlphanumeric(espID)
	espSecretKey = sanitizeAlphanumeric(espSecretKey)

	if !validateInput(espID) || !validateLength(espID, 3, 50) {
		log.Printf("Security Warning: Invalid ESP ID format from IP %s: %s", clientIP, espID)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID format"})
		return
	}

	if !validateInput(espSecretKey) || !validateLength(espSecretKey, 8, 100) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid secret key format"})
		return
	}

	// Find device in database
	var device ESPDevice
	if err := db.Where("esp_id = ? AND esp_secret_key = ?", espID, espSecretKey).First(&device).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID or secret key"})
		return
	}
	// Delete the device
	if err := db.Delete(&device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to remove ESP32"})
		return
	}

	// Publish the device deletion to the cluster
	publishDeviceChange(device, "delete")

	c.JSON(http.StatusOK, gin.H{"message": "ESP32 removed successfully"})
}

// command adds a new command for an ESP device
func command(c *gin.Context) {
	// Rate limiting check
	clientIP := c.ClientIP()
	if isRateLimited(clientIP, 30, time.Minute) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "Rate limit exceeded"})
		return
	}

	espID := c.PostForm("esp_id")
	commandText := c.PostForm("command")

	// Validate input
	if espID == "" || commandText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and command are required"})
		return
	}

	// Enhanced input validation
	espID = sanitizeAlphanumeric(espID)
	commandText = sanitizeInput(commandText) // Allow more characters for commands

	if !validateInput(espID) || !validateLength(espID, 3, 50) {
		log.Printf("Security Warning: Invalid ESP ID format from IP %s: %s", clientIP, espID)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID format"})
		return
	}

	if !validateInput(commandText) || !validateLength(commandText, 1, 500) {
		log.Printf("Security Warning: Invalid command format from IP %s: %s", clientIP, commandText)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid command format"})
		return
	}

	// Check if device exists
	var device ESPDevice
	if err := db.Where("esp_id = ?", espID).First(&device).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID"})
		return
	}
	// Create new command
	newCommand := Command{EspID: espID, Command: commandText}
	if err := db.Create(&newCommand).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to add command"})
		return
	}

	// Publish the command change to the cluster
	publishCommandChange(newCommand, "create")

	c.JSON(http.StatusOK, gin.H{"message": "Command added successfully"})
}

// getCommand allows a device to retrieve the next command
func getCommand(c *gin.Context) {
	espID := c.Query("esp_id")
	espSecretKey := c.Query("esp_secret_key")

	// Validate input
	if espID == "" || espSecretKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
		return
	}

	// Sanitize inputs
	espID = sanitizeInput(espID)
	espSecretKey = sanitizeInput(espSecretKey)

	// Verify the device
	var device ESPDevice
	if err := db.Where("esp_id = ? AND esp_secret_key = ?", espID, espSecretKey).First(&device).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID or secret key"})
		return
	}

	// Update last request time - store current time
	now := time.Now().UTC()
	device.LastRequestTime = &now

	// Update the database with a separate goroutine to avoid blocking the response
	// This ensures the ESP gets a command quickly even if the database is locked
	go func(deviceID uint, timestamp time.Time) {
		// Retry logic for database updates with exponential backoff
		maxRetries := 5
		baseDelay := 10 * time.Millisecond

		for attempt := 0; attempt < maxRetries; attempt++ {
			// Create a new DB session for each attempt to avoid transaction conflicts
			updateErr := db.Exec("UPDATE esp_devices SET last_request_time = ? WHERE id = ?", timestamp, deviceID).Error

			if updateErr == nil {
				// Success - log and exit retry loop
				log.Printf("Successfully updated LastRequestTime for device %s (ID: %d) after %d attempt(s)", espID, deviceID, attempt+1)
				break
			}

			// If it's the last attempt, just log the failure
			if attempt == maxRetries-1 {
				log.Printf("Failed to update LastRequestTime for device %s after %d attempts: %v", espID, maxRetries, updateErr)
				break
			}

			// Calculate backoff with jitter for next attempt
			delay := baseDelay * time.Duration(1<<uint(attempt)) // Exponential backoff
			jitter := time.Duration(rand.Int63n(int64(delay / 2)))
			delay = delay + jitter

			log.Printf("Database error updating device %s timestamp (attempt %d/%d): %v - retrying in %v",
				espID, attempt+1, maxRetries, updateErr, delay)
			time.Sleep(delay)
		}
	}(device.ID, now)

	// Retrieve the next command - we do this after updating the last_request_time to ensure
	// the timestamp is updated even if there's a problem retrieving or deleting the command
	var commandErr error
	var command Command
	commandErr = db.Where("esp_id = ?", espID).Order("id").First(&command).Error

	if commandErr != nil {
		// If no command found, return a default "null" command
		presetCommand := "null"
		command = Command{EspID: espID, Command: presetCommand}
	} else {
		// A command was found - attempt to delete it from the database
		// If deletion fails due to database locking, we'll still return the command
		// but it might be retrieved again on the next poll
		if err := db.Delete(&command).Error; err != nil {
			log.Printf("Warning: Failed to delete command for device %s: %v", espID, err)
			// Continue processing - don't return an error to the client
		}
	}

	c.JSON(http.StatusOK, gin.H{"command": command.Command})
}

// removeCommand removes a specific command by ID
func removeCommand(c *gin.Context) {
	commandID := c.PostForm("command_id")

	// Validate input
	if commandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Command ID is required"})
		return
	}

	// Find the command
	var command Command
	if err := db.First(&command, commandID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Command not found"})
		return
	}
	// Delete the command
	if err := db.Delete(&command).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to remove command"})
		return
	}

	// Publish the command deletion to the cluster
	publishCommandChange(command, "delete")

	c.JSON(http.StatusOK, gin.H{"message": "Command removed successfully"})
}

// getAllCommands retrieves all commands for an ESP device
func getAllCommands(c *gin.Context) {
	espID := c.Query("esp_id")

	// Validate input
	if espID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID is required"})
		return
	}

	// Fetch commands from database
	var commands []Command
	db.Where("esp_id = ?", espID).Order("id").Find(&commands)

	// Build a list of commands with IDs
	commandList := make([]map[string]interface{}, len(commands))
	for i, cmd := range commands {
		commandList[i] = map[string]interface{}{
			"id":      cmd.ID,
			"command": cmd.Command,
		}
	}

	c.JSON(http.StatusOK, gin.H{"commands": commandList})
}

// getActiveBoards returns a list of devices that have been active within the last 2 minutes
func getActiveBoards(c *gin.Context) {
	var devices []ESPDevice

	// Get devices with a last request time within the last 5 minutes (increased window for better visibility)
	fiveMinutesAgo := time.Now().UTC().Add(-5 * time.Minute)

	// Query to get all devices that have communicated recently
	if err := db.Where("last_request_time > ?", fiveMinutesAgo).Find(&devices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve active boards", "details": err.Error()})
		return
	}

	// Debug information
	log.Printf("Found %d active devices in the last 5 minutes", len(devices))

	// Build a list of active devices with proper time handling
	activeBoards := make([]map[string]interface{}, 0, len(devices))
	for _, device := range devices {
		// Only include devices with valid LastRequestTime
		if device.LastRequestTime != nil {
			// Calculate how long ago the device was active
			now := time.Now().UTC()
			lastRequestTime := device.LastRequestTime.UTC() // Ensure UTC comparison
			durationSinceLastRequest := now.Sub(lastRequestTime)

			// Format duration in a human-readable way
			var durationStr string
			if durationSinceLastRequest.Minutes() < 1 {
				durationStr = fmt.Sprintf("%.0f seconds ago", durationSinceLastRequest.Seconds())
			} else if durationSinceLastRequest.Hours() < 1 {
				durationStr = fmt.Sprintf("%.1f minutes ago", durationSinceLastRequest.Minutes())
			} else {
				durationStr = fmt.Sprintf("%.1f hours ago", durationSinceLastRequest.Hours())
			}
			// Add to the list of active boards
			boardInfo := map[string]interface{}{
				"esp_id":                device.EspID,
				"last_request_time":     device.LastRequestTime.Format(time.RFC3339),
				"last_request_duration": durationStr,
			}
			activeBoards = append(activeBoards, boardInfo)

			log.Printf("Active device: %s, last active: %s", device.EspID, durationStr)
		}
	}

	c.JSON(http.StatusOK, gin.H{"active_boards": activeBoards, "count": len(activeBoards)})
}

// CARGO
// cargoDelivery handles file delivery to the server
var idMutex sync.Mutex // Mutex to protect the counter

func cargoDelivery(c *gin.Context) {
	espID := c.PostForm("esp_id")
	deliveryKey := c.PostForm("delivery_key")
	encryptionPassword := c.PostForm("encryption_password")

	// Validate input
	if espID == "" || deliveryKey == "" || encryptionPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ESP ID, delivery key, and encryption password are required"})
		return
	}

	// Sanitize inputs
	espID = sanitizeInput(espID)
	deliveryKey = sanitizeInput(deliveryKey)
	encryptionPassword = sanitizeInput(encryptionPassword)

	// Retrieve the file from the form data
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload failed", "details": err.Error()})
		return
	}
	defer file.Close()

	// Generate a unique ID for the file
	uniqueID := getNextID()
	nodeIdentifier := "node1"

	// Generate a safe filename
	fileName := fmt.Sprintf("%s-%d", nodeIdentifier, uniqueID)
	outputDir := "cargo_files"

	// Ensure the output directory exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		err := os.Mkdir(outputDir, 0755)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory", "details": err.Error()})
			return
		}
	}

	// Create the output file
	outputPath := filepath.Join(outputDir, fileName)
	out, err := os.Create(outputPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file", "details": err.Error()})
		return
	}
	defer out.Close()

	// Copy the uploaded file to the output file
	if _, err := io.Copy(out, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write file", "details": err.Error()})
		return
	}

	// Update file metadata and save to database
	fileMetadata := FileMetadata{
		FileName:         fileName,
		OriginalFileName: header.Filename,
		FilePath:         outputPath,
		EspID:            espID,
		DeliveryKey:      deliveryKey, EncryptionPassword: encryptionPassword,
		Status:     StatusPending,
		RetryCount: 0,
	}
	if err := db.Create(&fileMetadata).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file metadata", "details": err.Error()})
		return
	}

	// Publish the file metadata to the cluster
	publishFileChange(fileMetadata, "create")

	// Try immediate delivery
	err = sendFileToStorage(outputPath, header.Filename, espID, deliveryKey, encryptionPassword)
	if err != nil {
		// Log the error but don't delete the file - the background service will retry
		log.Printf("Warning: Failed to deliver file to Storage server: %v. Will retry later.", err)
		c.JSON(http.StatusOK, gin.H{
			"message": "received successfully",
			"status":  StatusPending,
		})
		return
	}

	// Create a client to get the file ID from storage
	client := &http.Client{}
	resp, err := client.Get("http://localhost:6000/list_files")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "received successfully",
			"status":  StatusCompleted,
		})
		return
	}
	defer resp.Body.Close()

	var filesResp struct {
		Files []struct {
			ID       uint   `json:"id"`
			FileName string `json:"file_name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&filesResp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "received successfully",
			"status":  StatusCompleted,
		})
		return
	}

	// Find our file in the list - it should be the most recent one
	var fileID uint
	if len(filesResp.Files) > 0 {
		fileID = filesResp.Files[len(filesResp.Files)-1].ID
	} else {
	}

	// Success! Update status and delete local file
	fileMetadata.Status = StatusCompleted
	db.Save(&fileMetadata)

	err = os.Remove(outputPath)
	if err != nil {
		log.Printf("Warning: Failed to delete local file %s: %v", outputPath, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "delivered successfully",
		"status":  StatusCompleted,
		"file_id": fileID,
	})
}

// getNextID safely increments and returns the next unique ID
func getNextID() int {
	idMutex.Lock()
	defer idMutex.Unlock()

	// Persist idCounter in the database
	var counter Counter
	if err := db.First(&counter).Error; err != nil {
		// If counter not found, initialize it
		counter.Value = 1
		db.Create(&counter)
	} else {
		// Increment and save the counter
		counter.Value++
		db.Save(&counter)
	}
	return counter.Value
}

// saveFileMetadataToDatabase saves file metadata to the database
func saveFileMetadataToDatabase(fileName, originalFileName, filePath, espID, deliveryKey, encryptionPassword string) error {
	// Create a FileMetadata struct
	fileMetadata := FileMetadata{
		FileName:           fileName,
		OriginalFileName:   originalFileName,
		FilePath:           filePath,
		EspID:              espID,
		DeliveryKey:        deliveryKey,
		EncryptionPassword: encryptionPassword,
	}
	// Save to database
	if err := db.Create(&fileMetadata).Error; err != nil {
		return err
	}
	return nil
}

// registerMail registers a new mailer device
func registerMail(c *gin.Context) {
	espID := c.PostForm("esp_id")
	deliveryKey := c.PostForm("delivery_key")
	encryptionPassword := c.PostForm("encryption_password")

	// Validate input
	if espID == "" || deliveryKey == "" || encryptionPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID, delivery key, and encryption password are required"})
		return
	}

	// Sanitize inputs
	espID = sanitizeInput(espID)
	deliveryKey = sanitizeInput(deliveryKey)
	// Since we're not using encryptionPassword in this function, we can remove its sanitization
	
	// Check if device already exists
	var device ESPDevice
	if err := db.Where("esp_id = ?", espID).First(&device).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID already exists"})
		return
	}

	// Create new device
	newDevice := ESPDevice{
		EspID:           espID,
		EspSecretKey:    deliveryKey,
		LastRequestTime: nil,
	}

	if err := db.Create(&newDevice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to register device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Device registered successfully"})
}

// sendFileToStorage sends the file to the Storage server
func sendFileToStorage(filePath, fileName, espID, deliveryKey, encryptionPassword string) error {
	// Setup context with timeout for the request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Open the file to send
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file to the form
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %v", err)
	}

	// Add other form fields
	if err := writer.WriteField("esp_id", espID); err != nil {
		return fmt.Errorf("failed to add esp_id field: %v", err)
	}
	if err := writer.WriteField("delivery_key", deliveryKey); err != nil {
		return fmt.Errorf("failed to add delivery_key field: %v", err)
	}
	if err := writer.WriteField("encryption_password", encryptionPassword); err != nil {
		return fmt.Errorf("failed to add encryption_password field: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// Create POST request to Storage server
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:6000/upload_file", body)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request to storage server failed: %v", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload file to Storage server (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Log successful file transfer
	log.Printf("File successfully sent to Storage server: %s", fileName)
	return nil
}

// Gossip

// uploadFile handles file uploads to the server
func uploadFile(c *gin.Context) {
	// Retrieve the file from the form data
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get file"})
		return
	}
	defer file.Close()

	// Get other form fields
	espID := c.PostForm("esp_id")
	deliveryKey := c.PostForm("delivery_key")
	encryptionPassword := c.PostForm("encryption_password")

	// Create the file path
	filePath := "./uploads/" + header.Filename

	// Create the output file
	out, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create file"})
		return
	}
	defer out.Close()

	// Copy the uploaded file to the output file
	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Save file metadata to the database
	fileMetadata := FileMetadata{
		FileName:           header.Filename,
		OriginalFileName:   header.Filename,
		FilePath:           filePath,
		EspID:              espID,
		DeliveryKey:        deliveryKey,
		EncryptionPassword: encryptionPassword,
	}
	db.Create(&fileMetadata)

	c.JSON(http.StatusOK, gin.H{"status": "file uploaded"})
}

// authenticate handles user authentication
func authenticate(c *gin.Context) {
	var login struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	// Bind JSON payload to struct
	if err := c.ShouldBindJSON(&login); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Sanitize inputs
	login.Username = sanitizeInput(login.Username)
	login.Password = sanitizeInput(login.Password)

	// Fetch user from database
	var user User
	if err := db.Where("username = ?", login.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// Check password
	if !user.CheckPassword(login.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// Create session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Save()

	c.JSON(http.StatusOK, gin.H{"status": "authenticated"})
}

// receiveGossip handles incoming gossip data and merges it with local data
func receiveGossip(c *gin.Context) {
	var payload GossipPayload

	// Bind JSON payload to struct
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Merge remote commands
	for _, remoteCommand := range payload.Commands {
		var localCommand Command
		if err := db.Where("id = ?", remoteCommand.ID).First(&localCommand).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// If command doesn't exist locally, create it
				db.Create(&remoteCommand)
			} else {
				log.Printf("Failed to check existing command: %v", err)
			}
		} else {
			// If remote command is newer, update local command
			if remoteCommand.UpdatedAt.After(localCommand.UpdatedAt) {
				db.Save(&remoteCommand)
			}
		}
	}

	// Merge remote devices
	for _, remoteDevice := range payload.ESPDevices {
		var localDevice ESPDevice
		if err := db.Where("esp_id = ?", remoteDevice.EspID).First(&localDevice).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// If device doesn't exist locally, create it
				db.Create(&remoteDevice)
			} else {
				log.Printf("Failed to check existing device: %v", err)
			}
		} else {
			// If remote device is newer, update local device
			if remoteDevice.UpdatedAt.After(localDevice.UpdatedAt) {
				db.Save(&remoteDevice)
			}
		}
	}

	// Merge remote users
	for _, remoteUser := range payload.Users {
		var localUser User
		if err := db.Where("id = ?", remoteUser.ID).First(&localUser).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// If user doesn't exist locally, create it
				db.Create(&remoteUser)
			} else {
				log.Printf("Failed to check existing user: %v", err)
			}
		} else {
			// If remote user is newer, update local user
			if remoteUser.UpdatedAt.After(localUser.UpdatedAt) {
				db.Save(&remoteUser)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "gossip received"})
}

// updateLoadedCommands updates the commands loaded on an ESP device
func updateLoadedCommands(c *gin.Context) {
	espID := c.PostForm("esp_id")
	commandText := c.PostForm("command")
	if espID == "" || commandText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and command are required"})
		return
	}
	command(c) // Reuse the command function
}

// addCommand is an alias for the command function to maintain API compatibility
func addCommand(c *gin.Context) {
	command(c)
}

// isStorageServerOnline checks if the storage server is responding
func isStorageServerOnline() bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get("http://localhost:6000/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// startFileDeliveryService starts the background service that retries pending file deliveries
func startFileDeliveryService() {
	go func() {
		for {
			time.Sleep(RetryInterval)
			retryPendingFiles()
		}
	}()
}

// retryPendingFiles attempts to deliver any pending files to storage
func retryPendingFiles() {
	// Find all pending files that haven't exceeded max retries
	var pendingFiles []FileMetadata
	if err := db.Where("status = ? AND retry_count < ?", StatusPending, MaxRetryAttempts).Find(&pendingFiles).Error; err != nil {
		log.Printf("Failed to fetch pending files: %v", err)
		return
	}

	for _, file := range pendingFiles {
		// Try to deliver the file
		err := sendFileToStorage(file.FilePath, file.OriginalFileName, file.EspID, file.DeliveryKey, file.EncryptionPassword)

		if err != nil {
			// Update retry count
			file.RetryCount++
			if file.RetryCount >= MaxRetryAttempts {
				file.Status = StatusFailed
				log.Printf("File delivery failed after %d attempts: %s", MaxRetryAttempts, file.FileName)
			}
		} else {
			// Delivery successful
			file.Status = StatusCompleted
			// Try to delete the local file
			if err := os.Remove(file.FilePath); err != nil {
				log.Printf("Warning: Failed to delete local file %s: %v", file.FilePath, err)
			}
		}

		// Save the updated file status
		if err := db.Save(&file).Error; err != nil {
			log.Printf("Failed to update file status: %v", err)
		}
	}
}
