package main

import (
	"context"
	"log"
	"os"

	"net/http"

	"upchecks-backend/internal/db"
	"upchecks-backend/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	queries := db.New(pool)
	notificationService := service.NewNotificationService(queries)
	checkService := service.NewCheckService(queries, notificationService)

	go checkService.Start(ctx)

	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "running"})
	})

	r.GET("/services/:id/stats", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service id"})
			return
		}

		svc, err := queries.GetServiceByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
			return
		}

		stats, err := queries.GetServiceStats(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
			return
		}

		var uptimePercent float64
		if stats.TotalChecks > 0 {
			uptimePercent = float64(stats.SuccessfulChecks) / float64(stats.TotalChecks) * 100
		}

		c.JSON(http.StatusOK, gin.H{
			"service":         svc.Name,
			"endpoint":        svc.Endpoint,
			"total_checks":    stats.TotalChecks,
			"uptime_percent":  uptimePercent,
			"avg_latency_ms":  stats.AvgLatency,
			"last_latency_ms": stats.LastLatency,
			"last_success":    stats.LastSuccess,
		})
	})

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
