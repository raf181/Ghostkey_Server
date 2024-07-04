package main

import (
    "net/http"
    "time"
    "os"

    "github.com/gin-contrib/sessions"
    "github.com/gin-gonic/gin"
)

func registerRoutes(r *gin.Engine) {
    r.POST("/register_user", registerUser)
    r.POST("/login", login)
    r.POST("/logout", logout)
    r.POST("/register_device", registerDevice)
    r.POST("/command", command)
    r.GET("/get_command", getCommand)
    r.POST("/remove_command", removeCommand)
    r.GET("/get_all_commands", getAllCommands)
    r.GET("/last_request_time", lastRequestTime)
}

func registerUser(c *gin.Context) {
    session := sessions.Default(c)
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

    var command Command
    if err := db.Where("esp_id = ?", espID).Order("id").First(&command).Error; err != nil {
        c.JSON(http.StatusOK, gin.H{"command": nil})
        return
    }

    now := time.Now().UTC()
    device.LastRequestTime = &now

    if err := db.Save(&device).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update device"})
        return
    }

    if err := db.Delete(&command).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to delete command"})
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

func lastRequestTime(c *gin.Context) {
    espID := c.Query("esp_id")

    if espID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID is required"})
        return
    }

    var device ESPDevice
    if err := db.Where("esp_id = ?", espID).First(&device).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{"message": "ESP ID not found"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"last_request_time": device.LastRequestTime})
}
