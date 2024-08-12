package main

import (
	"bytes"
	"encoding/json"

	// "io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	// "golang.org/x/crypto/bcrypt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

var db *gorm.DB

func main() {
	var err error
	db, err = gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	db.AutoMigrate(&User{}, &ESPDevice{}, &Command{}, &FileMetadata{})

	r := gin.Default()

	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		log.Fatalf("SECRET_KEY environment variable is required")
	}

	log.Printf("Using secret key: %s", secretKey)

	store := cookie.NewStore([]byte(secretKey))
	r.Use(sessions.Sessions("mysession", store))

	registerRoutes(r)

	// Start gossip protocol
	go startGossip()

	if err := r.Run(":5000"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

func startGossip() {
	ticker := time.NewTicker(1 * time.Minute) // Adjust the interval as needed
	for range ticker.C {
		gossip()
	}
}
func gossip() {
	nodes := []string{"http://localhost:5002", "http://localhost:5003", "http://localhost:5004"} // Add your node URLs here
	targetNode := nodes[rand.Intn(len(nodes))]

	var localVersionVector VersionVector
	db.Model(&Command{}).Pluck("updated_at", &localVersionVector)

	// Fetch local commands
	var localCommands []Command
	db.Find(&localCommands)

	// Fetch local devices
	var localDevices []ESPDevice
	db.Find(&localDevices)

	// Fetch local users
	var localUsers []User
	db.Find(&localUsers)

	// Construct payload
	payload := GossipPayload{
		VersionVector: localVersionVector,
		Commands:      localCommands,
		ESPDevices:    localDevices,
		Users:         localUsers,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal gossip payload: %v", err)
		return
	}

	resp, err := http.Post(targetNode+"/gossip", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("Failed to gossip with %s: %v", targetNode, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Received non-OK status code from %s: %v", targetNode, resp.StatusCode)
		return
	}

	var remotePayload GossipPayload
	if err := json.NewDecoder(resp.Body).Decode(&remotePayload); err != nil {
		log.Printf("Failed to decode gossip payload from %s: %v", targetNode, err)
		return
	}

	// Merge remote commands
	for _, remoteCommand := range remotePayload.Commands {
		var localCommand Command
		if err := db.Where("id = ?", remoteCommand.ID).First(&localCommand).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteCommand)
			} else {
				log.Printf("Failed to check existing command: %v", err)
			}
		} else {
			if remoteCommand.UpdatedAt.After(localCommand.UpdatedAt) {
				db.Save(&remoteCommand)
			}
		}
	}

	// Merge remote devices
	for _, remoteDevice := range remotePayload.ESPDevices {
		var localDevice ESPDevice
		if err := db.Where("esp_id = ?", remoteDevice.EspID).First(&localDevice).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteDevice)
			} else {
				log.Printf("Failed to check existing device: %v", err)
			}
		} else {
			if remoteDevice.UpdatedAt.After(localDevice.UpdatedAt) {
				db.Save(&remoteDevice)
			}
		}
	}

	// Merge remote users
	for _, remoteUser := range remotePayload.Users {
		var localUser User
		if err := db.Where("id = ?", remoteUser.ID).First(&localUser).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteUser)
			} else {
				log.Printf("Failed to check existing user: %v", err)
			}
		} else {
			if remoteUser.UpdatedAt.After(localUser.UpdatedAt) {
				db.Save(&remoteUser)
			}
		}
	}
}
