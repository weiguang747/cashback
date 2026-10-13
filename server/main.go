package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	// Read the port from the environment so Docker/hosting platforms can set it.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Listen on all interfaces (":8080"), not "localhost:8080" —
	// inside a container, localhost is unreachable from your Mac.
	setupRouter().Run(":" + port)
}

// setupRouter registers all routes. It's separate from main so tests can use it.
func setupRouter() *gin.Engine {
	router := gin.Default()
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.POST("/auth/sign-in", signIn)
	return router
}
