// routes.go

package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	//"gorm.io/gorm"
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

    // CARGO
    r.POST("/cargo_delivery", cargoDelivery)
    r.POST("/register_mailer", registerMail)
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

// CARGO
// cargoDelivery handles file delivery to the server
var (
    idCounter = 1
    idMutex   sync.Mutex
)

func cargoDelivery(c *gin.Context) {
    espID := c.PostForm("esp_id")
    deliveryKey := c.PostForm("delivery_key")
    encryptionPassword := c.PostForm("encryption_password")

    if espID == "" || deliveryKey == "" || encryptionPassword == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ESP ID, delivery key, and encryption password are required"})
        return
    }

    file, header, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "File upload failed", "details": err.Error()})
        return
    }
    defer file.Close()

    uniqueID := getNextID()
    fileName := fmt.Sprintf("%d-%s", uniqueID, header.Filename)
    outputDir := "cargo_files"
    if _, err := os.Stat(outputDir); os.IsNotExist(err) {
        err := os.Mkdir(outputDir, 0755)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory", "details": err.Error()})
            return
        }
    }

    outputPath := filepath.Join(outputDir, fileName)
    out, err := os.Create(outputPath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file", "details": err.Error()})
        return
    }
    defer out.Close()

    if _, err := io.Copy(out, file); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write file", "details": err.Error()})
        return
    }

    fileMetadata := FileMetadata{
        FileName:           fileName,
        OriginalFileName:   header.Filename,
        FilePath:           outputPath,
        EspID:              espID,
        DeliveryKey:        deliveryKey,
        EncryptionPassword: encryptionPassword,
    }
    if err := db.Create(&fileMetadata).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file metadata", "details": err.Error()})
        return
    }

    // Send file to Depo server
    err = sendFileToDepo(outputPath, fileName, espID, deliveryKey, encryptionPassword)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deliver file to Depo server", "details": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "File delivered successfully"})
}


func getNextID() int {
    idMutex.Lock()
    defer idMutex.Unlock()
    // Simulate persisting idCounter in a database
    idCounter++
    return idCounter
}

func saveFileMetadataToDatabase(fileName, originalFileName, filePath, espID, deliveryKey, encryptionPassword string) error {
    // Example: Save file metadata to the database
    // You can modify the table structure as needed to store both filenames
    // Here we assume you have a `FileMetadata` struct and a database connection (`db`)
    fileMetadata := FileMetadata{
        FileName:           fileName,
        OriginalFileName:   originalFileName,
        FilePath:           filePath,
        EspID:              espID,
        DeliveryKey:        deliveryKey,
        EncryptionPassword: encryptionPassword,
    }
    if err := db.Create(&fileMetadata).Error; err != nil {
        return err
    }
    return nil
}

func registerMail(c *gin.Context) {
    espID := c.PostForm("esp_id")
    deliveryKey := c.PostForm("delivery_key")
    encryptionPassword := c.PostForm("encryption_password")

    if espID == "" || deliveryKey == "" || encryptionPassword == "" {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID, delivery key, and encryption password are required"})
        return
    }

    var device ESPDevice
    if err := db.Where("esp_id = ?", espID).First(&device).Error; err == nil {
        c.JSON(http.StatusBadRequest, gin.H{"message": "ESP ID already exists"})
        return
    }

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

func sendFileToDepo(filePath, fileName, espID, deliveryKey, encryptionPassword string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)

    // Add file
    part, err := writer.CreateFormFile("file", filepath.Base(fileName))
    if err != nil {
        return err
    }
    _, err = io.Copy(part, file)
    if err != nil {
        return err
    }

    // Add other fields
    _ = writer.WriteField("esp_id", espID)
    _ = writer.WriteField("delivery_key", deliveryKey)
    _ = writer.WriteField("encryption_password", encryptionPassword)

    err = writer.Close()
    if err != nil {
        return err
    }

    req, err := http.NewRequest("POST", "http://localhost:6000/upload_file", body)
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", writer.FormDataContentType())

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return err
    }

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("failed to upload file to Depo server: %s", string(respBody))
    }

    return nil
}