// main.go
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// Config struct to hold configuration values
type Config struct {
	ServerInterface string   `json:"server_interface"`
	GossipNodes     []string `json:"gossip_nodes"`
}

var (
	db     *gorm.DB
	config Config
)

func main() {
	var err error
	// Load configuration file
	configFile, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer configFile.Close()

	// Parse configuration file
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	// Open a connection to the SQLite database
	db, err = gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate the database schema
	db.AutoMigrate(&User{}, &ESPDevice{}, &Command{}, &FileMetadata{}, &Counter{})

	// Create a new Gin router
	r := gin.Default()

	// Get the secret key from the environment variables
	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		log.Fatalf("SECRET_KEY environment variable is required")
	}

	log.Printf("Using secret key: %s", secretKey)

	// Set up session store with the secret key
	store := cookie.NewStore([]byte(secretKey))
	r.Use(sessions.Sessions("mysession", store))

	// Register routes
	registerRoutes(r)

	// Start gossip protocol in a separate goroutine
	go startGossip()

	// Start the file delivery service
	startFileDeliveryService()

	// Run the Gin server on the configured interface
	if err := r.Run(config.ServerInterface); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

// startGossip starts the gossip protocol at regular intervals
func startGossip() {
	ticker := time.NewTicker(1 * time.Minute) // Adjust the interval as needed
	for range ticker.C {
		gossip()
	}
}

// gossip performs the gossip protocol
func gossip() {
	if len(config.GossipNodes) == 0 {
		log.Println("No gossip nodes configured, skipping gossip process")
		return
	}

	// Select a random node from the configured gossip nodes
	targetNode := config.GossipNodes[rand.Intn(len(config.GossipNodes))]
	var localVersionVector VersionVector
	
	// Fetch the local version vector
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

	// Marshal the payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal gossip payload: %v", err)
		return
	}

	// Send the payload to the target node
	resp, err := http.Post(targetNode+"/gossip", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("Failed to gossip with %s: %v", targetNode, err)
		return
	}
	defer resp.Body.Close()

	// Check if the response status is OK
	if resp.StatusCode != http.StatusOK {
		log.Printf("Received non-OK status code from %s: %v", targetNode, resp.StatusCode)
		return
	}

	// Decode the response payload
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
