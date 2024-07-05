// main.go

package main

import (
    "log"
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

    db.AutoMigrate(&User{}, &ESPDevice{}, &Command{})

    r := gin.Default()

    secretKey := os.Getenv("SECRET_KEY")
    if secretKey == "" {
        log.Fatalf("SECRET_KEY environment variable is required")
    }

    log.Printf("Using secret key: %s", secretKey)

    store := cookie.NewStore([]byte(secretKey))
    r.Use(sessions.Sessions("mysession", store))

    registerRoutes(r)

    if err := r.Run(":5000"); err != nil {
        log.Fatalf("Failed to run server: %v", err)
    }
}
