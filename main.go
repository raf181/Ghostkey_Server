// main.go
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// Config struct to hold configuration values
type Config struct {
	ServerInterface string   `json:"server_interface"` // Server listening interface and port
	GossipNodes     []string `json:"gossip_nodes"`     // List of other nodes for gossip protocol
	NodeID          string   `json:"node_id"`          // Unique identifier for this node
	ClusterEnabled  bool     `json:"cluster_enabled"`  // Whether to enable cluster mode
}

var (
	db     *gorm.DB
	config Config
)

// loadSecretFromFile loads the SECRET_KEY from .secrets file
func loadSecretFromFile() string {
	file, err := os.Open(".secrets")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "SECRET_KEY=") {
			return strings.TrimPrefix(line, "SECRET_KEY=")
		}
	}
	return ""
}

// backupDatabase creates a backup of the database
func backupDatabase() {
	backupDir := "./backups"
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Printf("Failed to create backup directory: %v", err)
		return
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupFile := fmt.Sprintf("%s/data_%s.db", backupDir, timestamp)

	// Copy the database file
	input, err := os.Open("data.db")
	if err != nil {
		log.Printf("Failed to open database for backup: %v", err)
		return
	}
	defer input.Close()

	output, err := os.Create(backupFile)
	if err != nil {
		log.Printf("Failed to create backup file: %v", err)
		return
	}
	defer output.Close()

	_, err = io.Copy(output, input)
	if err != nil {
		log.Printf("Failed to copy database: %v", err)
		return
	}

	log.Printf("Database backup created: %s", backupFile)
}

func main() {
	var err error
	// Load configuration from config.json file
	configFile, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer configFile.Close()

	// Parse configuration file
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	// Initialize the SQLite database connection
	db, err = gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Configure database connection pool for better performance
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get database instance: %v", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Perform automatic schema migration
	db.AutoMigrate(&User{}, &ESPDevice{}, &Command{}, &FileMetadata{}, &Counter{})

	// Create a new Gin router for handling HTTP requests
	// Set Gin mode based on environment
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// Add panic recovery middleware
	r.Use(gin.Recovery())

	// Retrieve secret key from environment variables for session store
	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		// Try to load from secrets file
		secretKey = loadSecretFromFile()
		if secretKey == "" {
			log.Fatalf("SECRET_KEY environment variable is required or .secrets file not found")
		}
	}

	log.Printf("Using secret key: [REDACTED - Length: %d characters]", len(secretKey))

	// Set up session middleware using the secret key
	store := cookie.NewStore([]byte(secretKey))
	r.Use(sessions.Sessions("mysession", store))

	// Register all the API routes
	registerRoutes(r)

	// Start periodic database backup (every 6 hours)
	go func() {
		backupTicker := time.NewTicker(6 * time.Hour)
		for range backupTicker.C {
			backupDatabase()
		}
	}()

	// Initialize the sync system if cluster mode is enabled
	if config.ClusterEnabled {
		if config.NodeID == "" {
			// Generate a random node ID if not provided
			config.NodeID = fmt.Sprintf("node-%d", time.Now().UnixNano())
			log.Printf("No node ID provided, generated: %s", config.NodeID)
		}

		log.Printf("Cluster mode enabled, node ID: %s", config.NodeID)
		initSync(config.NodeID)
	} else {
		// Start the gossip protocol in a separate goroutine (legacy mode)
		go startGossip()
	}

	// Start the file delivery background service
	log.Println("Starting file delivery service...")
	startFileDeliveryService()

	// Run a check for storage server availability
	go func() {
		// Initial check
		if isStorageServerOnline() {
			log.Println("Storage server is online and responding to health checks")
		} else {
			log.Println("WARNING: Storage server (Ghostkey_Depo) is offline! File delivery will be queued until it's available.")
			log.Println("Make sure Ghostkey_Depo is running on port 6000 or adjust the configuration.")
		}

		// Periodically check status
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			if isStorageServerOnline() {
				log.Println("Storage server connection status: Online")
			} else {
				log.Println("Storage server connection status: Offline")
			}
		}
	}()

	// Create HTTP server for graceful shutdown
	srv := &http.Server{
		Addr:    config.ServerInterface,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting Ghostkey Server on %s", config.ServerInterface)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to run server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Create a final backup before shutdown
	backupDatabase()

	// Give the server 30 seconds to finish current requests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server shutdown complete")
	}
}

// startGossip starts the gossip protocol at regular intervals
func startGossip() {
	// Create a ticker to trigger gossip at specified intervals
	ticker := time.NewTicker(1 * time.Minute) // Adjust the interval as needed
	for range ticker.C {
		// Call the gossip function when the ticker ticks
		gossip()
	}
}

// gossip performs the gossip protocol
func gossip() {
	// Check if gossip nodes are configured
	if len(config.GossipNodes) == 0 {
		log.Println("No gossip nodes configured, skipping gossip process")
		return
	}

	// Select a random gossip node to communicate with
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

	// Marshal the payload to JSON and send it to the target node
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal gossip payload: %v", err)
		return
	}

	// Send the payload to the target node with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Post(targetNode+"/gossip", "application/json", bytes.NewReader(payloadBytes))
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
