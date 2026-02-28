//models.go
package main

import (
    "time"

    "gorm.io/gorm"
    "golang.org/x/crypto/bcrypt"
)

// GossipPayload is the payload that is sent between nodes in the gossip protocol
type VersionVector map[string]time.Time
type GossipPayload struct {
    Users         []User
    ESPDevices    []ESPDevice
    Commands      []Command
    FileMetadata  []FileMetadata
    VersionVector VersionVector
}

type User struct {
    gorm.Model
    Username     string `gorm:"unique;not null"`
    PasswordHash string `gorm:"not null"`
}

func (user *User) SetPassword(password string) error {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        return err
    }
    user.PasswordHash = string(hash)
    return nil
}

func (user *User) CheckPassword(password string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
    return err == nil
}
type ESPDevice struct {
    gorm.Model
    EspID           string     `gorm:"unique;not null"`
    EspSecretKey    string     `gorm:"not null"`
    LastRequestTime *time.Time
}
type Command struct {
    gorm.Model
    EspID   string `gorm:"not null"`
    Command string `gorm:"not null"`
}
type LoadedCommandPayload struct {
    EspID    string   `json:"esp_id" binding:"required"`
    Commands []string `json:"commands" binding:"required"`
}
type FileMetadata struct {
    gorm.Model
    FileName           string
    OriginalFileName   string
    FilePath           string
    EspID              string
    DeliveryKey        string
    EncryptionPassword string
    Status             string // "pending", "completed", or "failed"
    RetryCount         int    // Track number of retry attempts
}
type Counter struct {
    ID    uint `gorm:"primaryKey"`
    Value int
}
