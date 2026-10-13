package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"cashback/server/internal/database"
	"cashback/server/internal/httpx"
	"cashback/server/internal/idempotency"
	"cashback/server/internal/logx"
)

func main() {
	// JSON logs on stdout, redacted (see internal/logx). LOG_LEVEL=debug for more.
	logger := logx.New(os.Stdout, logx.ParseLevel(os.Getenv("LOG_LEVEL")))
	slog.SetDefault(logger)
	// go-redis logs dial and pool errors itself; route them through slog too.
	redis.SetLogger(logx.Library{Name: "redis"})
	// Gin's own debug output (route list, warnings) isn't structured.
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Read the port from the environment so Docker/hosting platforms can set it.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fatal("DATABASE_URL is not set, e.g. postgres://cashback:cashback@localhost:5432/cashback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := database.Open(ctx, dbURL)
	cancel()
	if err != nil {
		fatal("database unavailable", logx.Err(err), "database", describeURL(dbURL))
	}
	defer pool.Close()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		fatal("invalid REDIS_URL", logx.Err(err))
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	err = rdb.Ping(ctx).Err()
	cancel()
	if err != nil {
		fatal("redis unavailable", logx.Err(err), "redis", describeURL(redisURL))
	}

	slog.Info("server starting", "port", port, "database", describeURL(dbURL), "redis", describeURL(redisURL),
		"gin_mode", gin.Mode())

	// Listen on all interfaces (":8080"), not "localhost:8080" —
	// inside a container, localhost is unreachable from your Mac.
	if err := setupRouter(newPgStore(pool), idempotency.NewRedis(rdb)).Run(":" + port); err != nil {
		fatal("server stopped", logx.Err(err))
	}
}

func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// describeURL is a connection URL safe to log: scheme, host and path, with
// any credentials and query (which can carry passwords) dropped.
func describeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return logx.Redacted
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// setupRouter registers all routes. It's separate from main so tests can use it.
func setupRouter(s store, idem idempotency.Store) *gin.Engine {
	router := gin.New()
	// RequestLogger first, so a panic is logged with its request id.
	router.Use(httpx.RequestLogger(slog.Default()), httpx.Recovery())
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
	signedIn.GET("/cashback/campaign", getCampaign(s))
	signedIn.GET("/cashback/history", listCashbackHistory(s))
	signedIn.POST("/cashback/redeem", redeemCashback(s, idem))

	return router
}
