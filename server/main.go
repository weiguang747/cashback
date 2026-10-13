package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"cashback/server/internal/httpx"
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
	router := gin.New()
	router.Use(gin.Logger(), httpx.Recovery())
	httpx.UseJSONErrors(router)

	// Health check stays unversioned: it's for Docker/hosting, not the app.
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Everything the app calls lives under /v1, so a future breaking change
	// can ship as /v2 without breaking app versions already installed.
	v1 := router.Group("/v1")
	v1.POST("/auth/sign-in", signIn)
	v1.GET("/transactions", listTransactions)

	return router
}
