package main

import (
	"context"
	"log"
	"math"
	"net/http"
	"os"

	"upchecks-backend/internal/db"
	"upchecks-backend/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

		ctx := c.Request.Context()

		svc, err := queries.GetServiceByID(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
			return
		}

		// Calculate uptime for each period
		periods := map[string]pgtype.Interval{
			"24h": {Days: 1, Valid: true},
			"7d":  {Days: 7, Valid: true},
			"30d": {Days: 30, Valid: true},
			"90d": {Days: 90, Valid: true},
		}

		type uptimeResult struct {
			TotalChecks      int32   `json:"total_checks"`
			SuccessfulChecks int32   `json:"successful_checks"`
			UptimePercent    float64 `json:"uptime_percent"`
			AvgLatencyMs     int32   `json:"avg_latency_ms"`
		}

		uptime := make(map[string]uptimeResult)
		for period, interval := range periods {
			stats, err := queries.GetUptimeForPeriod(ctx, db.GetUptimeForPeriodParams{
				ServiceID: id,
				Column2:   interval,
			})
			if err != nil {
				continue
			}

			var pct float64
			if stats.TotalChecks > 0 {
				pct = math.Round(float64(stats.SuccessfulChecks)/float64(stats.TotalChecks)*10000) / 100
			}

			uptime[period] = uptimeResult{
				TotalChecks:      stats.TotalChecks,
				SuccessfulChecks: stats.SuccessfulChecks,
				UptimePercent:    pct,
				AvgLatencyMs:     stats.AvgLatency,
			}
		}

		// Get last check
		var lastLatency int32
		var lastSuccess bool
		lastCheck, err := queries.GetLastCheck(ctx, id)
		if err == nil {
			lastLatency = lastCheck.Latency
			lastSuccess = lastCheck.Success
		}

		c.JSON(http.StatusOK, gin.H{
			"service":         svc.Name,
			"endpoint":        svc.Endpoint,
			"uptime":          uptime,
			"last_latency_ms": lastLatency,
			"last_success":    lastSuccess,
		})
	})

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
