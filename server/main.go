package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"cashback/server/internal/database"
	"cashback/server/internal/httpx"
	"cashback/server/internal/idempotency"
)

func main() {
	// Read the port from the environment so Docker/hosting platforms can set it.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set, e.g. postgres://cashback:cashback@localhost:5432/cashback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := database.Open(ctx, dbURL)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("parse REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	err = rdb.Ping(ctx).Err()
	cancel()
	if err != nil {
		log.Fatalf("connect to redis: %v", err)
	}

	// Listen on all interfaces (":8080"), not "localhost:8080" —
	// inside a container, localhost is unreachable from your Mac.
	if err := setupRouter(newPgStore(pool), idempotency.NewRedis(rdb)).Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

// setupRouter registers all routes. It's separate from main so tests can use it.
func setupRouter(s store, idem idempotency.Store) *gin.Engine {
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
	v1.POST("/auth/sign-in", signIn(s))

	signedIn := v1.Group("", requireUser(s))
	signedIn.GET("/accounts", listAccounts(s))
	signedIn.GET("/transactions", listTransactions(s))
	signedIn.POST("/payments", createPayment(s, idem))

	return router
}
