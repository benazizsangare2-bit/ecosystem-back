package main

import (
	"ecosystem/database"
	"ecosystem/routes"
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	database.ConnectDatabase()
	db := &database.Database{DB: database.DB}
	db.InitDatabase()

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST", "GET", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	routes.Setup(router)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3030"
	}
	if err := router.Run(":" + port); err != nil {
		panic(err)
	}
}
