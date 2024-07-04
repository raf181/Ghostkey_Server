package main

import (
    "log"
    //"net/http"
    "os"

    "github.com/gin-contrib/sessions"
    "github.com/gin-contrib/sessions/cookie"
    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

var db *gorm.DB

func main() {
    var err error
    db, err = gorm.Open(sqlite.Open("database.db"), &gorm.Config{})
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }

    // Auto migrate database
    db.AutoMigrate(&User{}, &ESPDevice{}, &Command{})

    r := gin.Default()

    // Get the secret key from an environment variable
    secretKey := os.Getenv("SECRET_KEY")
    if secretKey == "" {
        log.Fatalf("SECRET_KEY environment variable is required")
    }

    log.Printf("Using secret key: %s", secretKey) // Log the secret key for verification

    store := cookie.NewStore([]byte(secretKey))
    r.Use(sessions.Sessions("mysession", store))

    // Register routes
    registerRoutes(r)

    // Run the server
    if err := r.Run(":5000"); err != nil {
        log.Fatalf("Failed to run server: %v", err)
    }
}
