package service

import (
	"context"
	"log"
	"net/http"
	"time"

	"upchecks-backend/internal/db"
)

type CheckService struct {
	db *db.Queries
}

func NewCheckService(queries *db.Queries) *CheckService {
	return &CheckService{db: queries}
}

func (s *CheckService) Start(ctx context.Context) {
	services, err := s.db.GetAllServices(ctx)
	if err != nil {
		log.Printf("failed to fetch services: %v", err)
		return
	}

	log.Printf("starting checks for %d services", len(services))

	for _, svc := range services {
		go s.checkLoop(ctx, svc)
	}
}

func (s *CheckService) checkLoop(ctx context.Context, svc db.Service) {
	log.Printf("checking %s (%s) every %ds", svc.Name, svc.Endpoint, svc.Interval)

	// Run immediately on start, then on interval
	s.executeCheck(ctx, svc)

	ticker := time.NewTicker(time.Duration(svc.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("stopping checks for %s", svc.Name)
			return
		case <-ticker.C:
			current, err := s.db.GetServiceByID(ctx, svc.ID)
			if err != nil {
				log.Printf("failed to fetch service %s: %v", svc.Name, err)
				continue
			}
			if !current.Enabled {
				log.Printf("service %s is disabled, stopping checks", svc.Name)
				return
			}
			s.executeCheck(ctx, current)
		}
	}
}

func (s *CheckService) executeCheck(ctx context.Context, svc db.Service) {
	client := &http.Client{Timeout: 10 * time.Second}

	start := time.Now()
	resp, err := client.Get(svc.Endpoint)
	latency := time.Since(start).Milliseconds()

	success := err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 400
	var statusCode *int32
	if resp != nil {
		sc := int32(resp.StatusCode)
		statusCode = &sc
		resp.Body.Close()
	}

	_, dbErr := s.db.CreateCheck(ctx, db.CreateCheckParams{
		ServiceID:  svc.ID,
		Success:    success,
		StatusCode: statusCode,
		Latency:    int32(latency),
	})
	if dbErr != nil {
		log.Printf("failed to save check for %s: %v", svc.Name, dbErr)
		return
	}

	status := "OK"
	if !success {
		status = "FAIL"
	}
	log.Printf("[%s] %s - %dms", status, svc.Name, latency)
}
