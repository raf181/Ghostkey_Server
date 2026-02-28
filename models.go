// models.go
package main

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// VersionVector is a map that tracks the last update time for each node.
// Used in the gossip protocol to resolve conflicts.
type VersionVector map[string]time.Time

// GossipPayload is the payload sent between nodes in the gossip protocol.
// It contains the data to synchronize and the version vector.
type GossipPayload struct {
	Users         []User         // List of users to synchronize
	ESPDevices    []ESPDevice    // List of ESP devices to synchronize
	Commands      []Command      // List of commands to synchronize
	FileMetadata  []FileMetadata // List of file metadata to synchronize
	VersionVector VersionVector  // Version vector for conflict resolution
	NodeID        string         // ID of the node that created this payload
}

// User represents a registered user with a username and password hash.
type User struct {
	gorm.Model
	Username     string `gorm:"unique;not null"` // Unique username
	PasswordHash string `gorm:"not null"`        // Hashed password
}

// SetPassword hashes the given password and stores it in PasswordHash.
func (user *User) SetPassword(password string) error {
	// Generate a bcrypt hash of the password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err // Return error if hashing fails
	}
	user.PasswordHash = string(hash) // Store the hashed password
	return nil
}

// CheckPassword compares the given password with the stored PasswordHash.
func (user *User) CheckPassword(password string) bool {
	// Compare the hashed password with the provided password
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil // Return true if passwords match
}

// ESPDevice represents an ESP device with its credentials.
type ESPDevice struct {
	gorm.Model
	EspID           string     `gorm:"unique;not null"` // Unique identifier for the ESP device
	EspSecretKey    string     `gorm:"not null"`        // Secret key for authentication
	LastRequestTime *time.Time // Timestamp of the last request received
}

// Command represents a command to be executed by an ESP device.
type Command struct {
	gorm.Model
	EspID   string `gorm:"not null"` // Identifier of the target ESP device
	Command string `gorm:"not null"` // Command to be executed
}

// LoadedCommandPayload is the payload received from an ESP device after executing commands.
type LoadedCommandPayload struct {
	EspID    string   `json:"esp_id" binding:"required"`   // Identifier of the ESP device
	Commands []string `json:"commands" binding:"required"` // List of executed commands
}

// FileMetadata contains metadata about files to be delivered to ESP devices.
type FileMetadata struct {
	gorm.Model
	FileName           string // Name of the stored file
	OriginalFileName   string // Original name of the uploaded file
	FilePath           string // Path where the file is stored
	EspID              string // Identifier of the target ESP device
	DeliveryKey        string // Key used to validate file delivery
	EncryptionPassword string // Password used for file encryption
	Status             string // Current status: "pending", "completed", or "failed"
	RetryCount         int    // Number of retry attempts for delivery
}

// Counter is used to keep track of a numerical value.
type Counter struct {
	ID    uint `gorm:"primaryKey"` // Primary key identifier
	Value int  // Current counter value
}
