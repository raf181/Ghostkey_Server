// Models.go

package main

import (
    "time"

    "gorm.io/gorm"
    "golang.org/x/crypto/bcrypt"
)

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