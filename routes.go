// routes.go

package main

import (
    "net/http"
    "time"
    "os"

    "github.com/gin-contrib/sessions"
    "github.com/gin-gonic/gin"
)

func registerRoutes(r *gin.Engine) {
    // User routes
    r.POST("/register_user", registerUser)
    r.POST("/login", login)
    r.POST("/logout", logout)

    // Device routes
    r.POST("/register_device", registerDevice) 
    r.DELETE("/remove_device", removeDevice)

    // Command routes
    r.POST("/loaded_command", loadedCommand)
    r.GET("/get_loaded_command", getLoadedCommand)
    r.POST("/command", command)
    r.GET("/get_command", getCommand)
    r.POST("/remove_command", removeCommand)
    r.GET("/get_all_commands", getAllCommands)

    // Active boards route
// [1]:Remuve r.GET("/last_request_time", lastRequestTime)
    r.GET("/active_boards", getActiveBoards)// [1]:Remuve 
}

func loadedCommand(c *gin.Context) {
    var payload LoadedCommandPayload

    if err := c.ShouldBindJSON(&payload); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if payload.EspID == "" || len(payload.Commands) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ESP ID and commands are required"})
        return
    }

    // Save commands associated with ESP ID in your database
    for _, cmd := range payload.Commands {
        newCommand := Command{EspID: payload.EspID, Command: cmd}
        if err := db.Create(&newCommand).Error; err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save commands"})
            return
        }
    }

    c.JSON(http.StatusOK, gin.H{"message": "Commands saved successfully"})
}


func getLoadedCommand(c *gin.Context) {
    espID := c.Query("esp_id")

    if espID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ESP ID is required"})
        return
    }

    var commands []Command
    if err := db.Where("esp_id = ?", espID).Find(&commands).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve commands"})
        return
    }

    commandList := make([]string, len(commands))
    for i, cmd := range commands {
        commandList[i] = cmd.Command
    }

    c.JSON(http.StatusOK, gin.H{"esp_id": espID, "commands": commandList})
}


func registerUser(c *gin.Context) {
    secretKey := c.PostForm("secret_key")
    expectedSecretKey := os.Getenv("SECRET_KEY")

    if secretKey != expectedSecretKey {
        c.JSON(http.StatusForbidden, gin.H{"message": "Invalid secret key"})
        return
    }

    username := c.PostForm("username")
    password := c.PostForm("password")
    if username == "" || password == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Username and password are required"})
        return
    }

    var user User
    if err := db.Where("username = ?", username).First(&user).Error; err == nil {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Username already exists"})
        return
    }

    newUser := User{Username: username}
    if err := newUser.SetPassword(password); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to set password"})
        return
    }

    if err := db.Create(&newUser).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to register user"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
}

func login(c *gin.Context) {
    username := c.PostForm("username")
    password := c.PostForm("password")
    if username == "" || password == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Username and password are required"})
        return
    }

    var user User
    if err := db.Where("username = ?", username).First(&user).Error; err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid username or password"})
        return
    }

    if !user.CheckPassword(password) {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid username or password"})
        return
    }

    session := sessions.Default(c)
    session.Set("user_id", user.ID)
    session.Save()

    c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}

func logout(c *gin.Context) {
    session := sessions.Default(c)
    session.Clear()
    session.Save()

    c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func registerDevice(c *gin.Context) {
    espID := c.PostForm("esp_id")
    espSecretKey := c.PostForm("esp_secret_key")

    if espID == "" || espSecretKey == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
        return
    }

    var device ESPDevice
    if err := db.Where("esp_id = ?", espID).First(&device).Error; err == nil {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID already exists"})
        return
    }

    newDevice := ESPDevice{EspID: espID, EspSecretKey: espSecretKey}
    if err := db.Create(&newDevice).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to register ESP32"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "ESP32 registered successfully", "esp_id": espID})
}

func removeDevice(c *gin.Context) {
    espID := c.Query("esp_id")
    espSecretKey := c.Query("secret_key")

    // Validate parameters
    if espID == "" || espSecretKey == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
        return
    }

    // Your logic to find and delete the device
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

func command(c *gin.Context) {
    espID := c.PostForm("esp_id")
    commandText := c.PostForm("command")

    if espID == "" || commandText == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and command are required"})
        return
    }

    var device ESPDevice
    if err := db.Where("esp_id = ?", espID).First(&device).Error; err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID"})
        return
    }

    newCommand := Command{EspID: espID, Command: commandText}
    if err := db.Create(&newCommand).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to add command"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "Command added successfully"})
}

func getCommand(c *gin.Context) {
    espID := c.Query("esp_id")
    espSecretKey := c.Query("esp_secret_key")

    if espID == "" || espSecretKey == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID and secret key are required"})
        return
    }

    var device ESPDevice
    if err := db.Where("esp_id = ? AND esp_secret_key = ?", espID, espSecretKey).First(&device).Error; err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid ESP ID or secret key"})
        return
    }
    
    now := time.Now().UTC()
    device.LastRequestTime = &now

    var command Command
    err := db.Where("esp_id = ?", espID).Order("id").First(&command).Error
    if err != nil {
        // If no command found, create a preset command
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


func removeCommand(c *gin.Context) {
    commandID := c.PostForm("command_id")

    if commandID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "Command ID is required"})
        return
    }

    var command Command
    if err := db.First(&command, commandID).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{"message": "Command not found"})
        return
    }

    if err := db.Delete(&command).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to remove command"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "Command removed successfully"})
}

func getAllCommands(c *gin.Context) {
    espID := c.Query("esp_id")

    if espID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID is required"})
        return
    }

    var commands []Command
    db.Where("esp_id = ?", espID).Order("id").Find(&commands)

    commandList := make([]map[string]interface{}, len(commands))
    for i, cmd := range commands {
        commandList[i] = map[string]interface{}{
            "id":      cmd.ID,
            "command": cmd.Command,
        }
    }

    c.JSON(http.StatusOK, gin.H{"commands": commandList})
}

// [1]:Remuve func lastRequestTime(c *gin.Context) {
// [1]:Remuve     espID := c.Query("esp_id")
// [1]:Remuve 
// [1]:Remuve     if espID == "" {
// [1]:Remuve         c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID is required"})
// [1]:Remuve         return
// [1]:Remuve     }
// [1]:Remuve 
// [1]:Remuve     var device ESPDevice
// [1]:Remuve     if err := db.Where("esp_id = ?", espID).First(&device).Error; err != nil {
// [1]:Remuve         c.JSON(http.StatusNotFound, gin.H{"message": "ESP ID not found"})
// [1]:Remuve         return
// [1]:Remuve     }
// [1]:Remuve 
// [1]:Remuve     c.JSON(http.StatusOK, gin.H{"last_request_time": device.LastRequestTime})
// [1]:Remuve }

func getActiveBoards(c *gin.Context) {
    var devices []ESPDevice

    // Get devices with a last request time within the last 2 minutes
    XMinutesAgo := time.Now().UTC().Add(-2 * time.Minute)
    if err := db.Where("last_request_time > ?", XMinutesAgo).Find(&devices).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve active boards"})
        return
    }

    activeBoards := make([]map[string]interface{}, len(devices))
    for i, device := range devices {
        // Check if device.LastRequestTime is not nil to avoid nil pointer dereference
        if device.LastRequestTime != nil {
            durationSinceLastRequest := time.Since(*device.LastRequestTime)
            activeBoards[i] = map[string]interface{}{
                "esp_id":                device.EspID,
                "last_request_duration": durationSinceLastRequest.String(),
            }
        } else {
            // Handle case where LastRequestTime is nil (no pending commands)
            activeBoards[i] = map[string]interface{}{
                "esp_id":                device.EspID,
                "last_request_duration": "No commands since last request",
            }
        }
    }

    c.JSON(http.StatusOK, gin.H{"active_boards": activeBoards})
}

