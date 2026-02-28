// Package main declares the main package of the application
package main

// Import necessary packages
import (
	"bytes"
	"fmt"
	"io"
	"log"
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
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	MaxRetryAttempts = 5
	RetryInterval   = 1 * time.Minute
)

// authRequired checks for either session cookie or Basic Auth
func authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// First check for session authentication
		session := sessions.Default(c)
		userID := session.Get("user_id")
		if userID != nil {
			c.Set("user_id", userID)
			c.Next()
			return
		}

		// If no session, check for Basic Auth
		username, password, hasAuth := c.Request.BasicAuth()
		if hasAuth {
			// Sanitize input
			username = sanitizeInput(username)
			
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

// registerRoutes sets up all the API endpoints for the server
func registerRoutes(r *gin.Engine) {
	// Public routes (no authentication required)
	r.POST("/register_user", registerUser)
	r.POST("/login", login)
	r.GET("/get_command", getCommand)
	r.POST("/cargo_delivery", cargoDelivery)

	// Protected routes (authentication required)
	authenticated := r.Group("/")
	authenticated.Use(authRequired())
	{
		authenticated.POST("/logout", logout)

		// Device routes
		authenticated.POST("/register_device", registerDevice)
		authenticated.DELETE("/remove_device", removeDevice)

		// Command routes
		authenticated.POST("/loaded_command", loadedCommand)
		authenticated.GET("/get_loaded_command", getLoadedCommand)
		authenticated.POST("/command", command)
		authenticated.POST("/remove_command", removeCommand)
		authenticated.GET("/get_all_commands", getAllCommands)

		// Active boards route
		authenticated.GET("/active_boards", getActiveBoards)

		// CARGO routes
		authenticated.POST("/register_mailer", registerMail)
	}
}

// sanitizeInput cleans the input string to prevent injection attacks
func sanitizeInput(input string) string {
	input = strings.TrimSpace(input) // Remove leading/trailing whitespace
	re := regexp.MustCompile(`[^\w@.-]`)
	return re.ReplaceAllString(input, "") // Remove unwanted characters
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
	secretKey := c.PostForm("secret_key")
	expectedSecretKey := os.Getenv("SECRET_KEY") // Expected secret key from environment variables

	// Validate secret key
	if secretKey != expectedSecretKey {
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

	// Sanitize username only
	username = sanitizeInput(username)

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

	c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
}

// login handles user login
func login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	// Validate input
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username and password are required"})
		return
	}

	// Sanitize username only
	username = sanitizeInput(username)

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

	// Create session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Save()

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
	espID := c.PostForm("esp_id")
	espSecretKey := c.PostForm("esp_secret_key")

	// Validate input
	if espID == "" || espSecretKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
		return
	}

	// Sanitize inputs
	espID = sanitizeInput(espID)
	espSecretKey = sanitizeInput(espSecretKey)

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

	c.JSON(http.StatusOK, gin.H{"message": "ESP32 registered successfully", "esp_id": espID})
}

// removeDevice handles removal of an ESP device
func removeDevice(c *gin.Context) {
	espID := c.Query("esp_id")
	espSecretKey := c.Query("secret_key")

	// Validate parameters
	if espID == "" || espSecretKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
		return
	}

	// Sanitize inputs
	espID = sanitizeInput(espID)
	espSecretKey = sanitizeInput(espSecretKey)

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

	c.JSON(http.StatusOK, gin.H{"message": "ESP32 removed successfully"})
}

// command adds a new command for an ESP device
func command(c *gin.Context) {
	espID := c.PostForm("esp_id")
	commandText := c.PostForm("command")

	// Validate input
	if espID == "" || commandText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and command are required"})
		return
	}

	// Sanitize inputs
	espID = sanitizeInput(espID)
	commandText = sanitizeInput(commandText)

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

	// Update last request time
	now := time.Now().UTC()
	device.LastRequestTime = &now

	// Retrieve the next command
	var command Command
	err := db.Where("esp_id = ?", espID).Order("id").First(&command).Error
	if err != nil {
		// If no command found, return a preset command
		presetCommand := "null"
		command = Command{EspID: espID, Command: presetCommand}
	} else {
		// Delete the retrieved command from the database
		if err := db.Delete(&command).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to delete command"})
			return
		}
	}

	// Save the updated LastRequestTime for the device
	if err := db.Save(&device).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update device"})
		return
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

	// Get devices with a last request time within the last 2 minutes
	XMinutesAgo := time.Now().UTC().Add(-2 * time.Minute)
	if err := db.Where("last_request_time > ?", XMinutesAgo).Find(&devices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve active boards"})
		return
	}

	// Build a list of active devices
	activeBoards := make([]map[string]interface{}, len(devices))
	for i, device := range devices {
		// Check if LastRequestTime is not nil
		if device.LastRequestTime != nil {
			durationSinceLastRequest := time.Since(*device.LastRequestTime)
			activeBoards[i] = map[string]interface{}{
				"esp_id":                device.EspID,
				"last_request_duration": durationSinceLastRequest.String(),
			}
		} else {
			// Handle case where LastRequestTime is nil
			activeBoards[i] = map[string]interface{}{
				"esp_id":                device.EspID,
				"last_request_duration": "No commands since last request",
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"active_boards": activeBoards})
}

// CARGO
// cargoDelivery handles file delivery to the server
var (
	idCounter = 1        // Counter for unique IDs
	idMutex   sync.Mutex // Mutex to protect the counter
)

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
		FileName:           fileName,
		OriginalFileName:   header.Filename,
		FilePath:           outputPath,
		EspID:              espID,
		DeliveryKey:        deliveryKey,
		EncryptionPassword: encryptionPassword,
		Status:            StatusPending,
		RetryCount:        0,
	}
	if err := db.Create(&fileMetadata).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file metadata", "details": err.Error()})
		return
	}

	// Try immediate delivery
	err = sendFileToStorage(outputPath, header.Filename, espID, deliveryKey, encryptionPassword)
	if err != nil {
		// Log the error but don't delete the file - the background service will retry
		log.Printf("Warning: Failed to deliver file to Storage server: %v. Will retry later.", err)
		c.JSON(http.StatusOK, gin.H{
			"message": "File received successfully, will attempt delivery to storage server",
			"status":  StatusPending,
		})
		return
	}

	// Success! Update status and delete local file
	fileMetadata.Status = StatusCompleted
	db.Save(&fileMetadata)
	
	err = os.Remove(outputPath)
	if err != nil {
		log.Printf("Warning: Failed to delete local file %s: %v", outputPath, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "File delivered successfully",
		"status":  StatusCompleted,
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
	encryptionPassword = sanitizeInput(encryptionPassword)

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
	// Open the file to send
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file to the form
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	// Add other form fields
	_ = writer.WriteField("esp_id", espID)
	_ = writer.WriteField("delivery_key", deliveryKey)
	_ = writer.WriteField("encryption_password", encryptionPassword)

	err = writer.Close()
	if err != nil {
		return err
	}

	// Create POST request to Storage server
	req, err := http.NewRequest("POST", "http://localhost:6000/upload_file", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload file to Storage server: %s", string(respBody))
	}

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

	c.JSON(http.StatusOK, gin.H{"status": "success"})
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

// isStorageServerOnline checks if the storage server is responding
func isStorageServerOnline() bool {
	// Try a simple HEAD request to check if server is up
	resp, err := http.Head("http://localhost:6000/health")
	if err != nil {
		log.Printf("Storage server appears to be offline: %v", err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// retryPendingFiles attempts to deliver any pending files to the storage server
func retryPendingFiles() {
	// First check if storage server is online
	if !isStorageServerOnline() {
		log.Printf("Storage server is offline, skipping file delivery attempts")
		return
	}

	var pendingFiles []FileMetadata
	
	// Find all pending files
	if err := db.Where("status = ?", StatusPending).Find(&pendingFiles).Error; err != nil {
		log.Printf("Error fetching pending files: %v", err)
		return
	}

	for _, file := range pendingFiles {
		// Check if file still exists
		if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
			log.Printf("File %s no longer exists, marking as failed", file.FilePath)
			file.Status = StatusFailed
			db.Save(&file)
			continue
		}

		// Attempt to send file
		err := sendFileToStorage(file.FilePath, file.OriginalFileName, file.EspID, file.DeliveryKey, file.EncryptionPassword)
		if err != nil {
			log.Printf("Failed to deliver file %s: %v", file.FilePath, err)
			// Don't increment retry count if server becomes unreachable
			if isStorageServerOnline() {
				file.RetryCount++
				if file.RetryCount >= MaxRetryAttempts {
					file.Status = StatusFailed
					log.Printf("File %s failed to deliver after %d attempts", file.FilePath, MaxRetryAttempts)
				}
				db.Save(&file)
			}
			continue
		}

		// Success! Delete the file and update the database
		os.Remove(file.FilePath)
		file.Status = StatusCompleted
		db.Save(&file)
		log.Printf("Successfully delivered pending file: %s", file.FilePath)
	}
}
