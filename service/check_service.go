package service

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"sync"
	"time"

	"upchecks-backend/internal/db"

	"github.com/google/uuid"
)

type CheckService struct {
	db            *db.Queries
	notifications *NotificationService
	active        map[uuid.UUID]context.CancelFunc
	mu            sync.Mutex
}

func NewCheckService(queries *db.Queries, notifications *NotificationService) *CheckService {
	return &CheckService{
		db:            queries,
		notifications: notifications,
		active:        make(map[uuid.UUID]context.CancelFunc),
	}
}

func (s *CheckService) Start(ctx context.Context) {
	s.syncServices(ctx)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncServices(ctx)
		}
	}
}

func (s *CheckService) syncServices(ctx context.Context) {
	services, err := s.db.GetAllServices(ctx)
	if err != nil {
		log.Printf("failed to fetch services: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Track which services are still active
	activeIDs := make(map[uuid.UUID]bool)
	for _, svc := range services {
		activeIDs[svc.ID] = true

		if _, exists := s.active[svc.ID]; !exists {
			log.Printf("starting checks for %s (%s)", svc.Name, svc.Endpoint)
			svcCtx, cancel := context.WithCancel(ctx)
			s.active[svc.ID] = cancel
			go s.checkLoop(svcCtx, svc)
		}
	}

	// Stop goroutines for services that were disabled or deleted
	for id, cancel := range s.active {
		if !activeIDs[id] {
			log.Printf("stopping checks for removed/disabled service %s", id)
			cancel()
			delete(s.active, id)
		}
	}
}

func (s *CheckService) checkLoop(ctx context.Context, svc db.Service) {
	log.Printf("checking %s (%s) every %ds", svc.Name, svc.Endpoint, svc.Interval)

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
			s.executeCheck(ctx, current)
		}
	}
}

func (s *CheckService) executeCheck(ctx context.Context, svc db.Service) {
	var success bool
	var statusCode *int32
	var latency int64

	switch svc.Type {
	case "smtp":
		success, latency = s.checkSMTP(svc.Endpoint)
	default:
		success, statusCode, latency = s.checkHTTP(svc.Endpoint)
	}

	check, dbErr := s.db.CreateCheck(ctx, db.CreateCheckParams{
		ServiceID:  svc.ID,
		Success:    success,
		StatusCode: statusCode,
		Latency:    int32(latency),
	})
	if dbErr != nil {
		log.Printf("failed to save check for %s: %v", svc.Name, dbErr)
		return
	}

	s.notifications.CheckAndNotify(ctx, svc, check)

	status := "OK"
	if !success {
		status = "FAIL"
	}
	log.Printf("[%s] %s - %dms", status, svc.Name, latency)
}

func (s *CheckService) checkHTTP(endpoint string) (success bool, statusCode *int32, latency int64) {
	client := &http.Client{Timeout: 10 * time.Second}

	start := time.Now()
	resp, err := client.Get(endpoint)
	latency = time.Since(start).Milliseconds()

	success = err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 400
	if resp != nil {
		sc := int32(resp.StatusCode)
		statusCode = &sc
		resp.Body.Close()
	}
	return
}

func (s *CheckService) checkSMTP(endpoint string) (success bool, latency int64) {
	// Ensure host:port format, default to :587 for STARTTLS
	host := endpoint
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		endpoint = endpoint + ":587"
	}

	start := time.Now()

	conn, err := net.DialTimeout("tcp", endpoint, 10*time.Second)
	if err != nil {
		latency = time.Since(start).Milliseconds()
		return false, latency
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		latency = time.Since(start).Milliseconds()
		return false, latency
	}
	defer client.Close()

	// Upgrade to TLS via STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		err = client.StartTLS(&tls.Config{ServerName: host})
		if err != nil {
			latency = time.Since(start).Milliseconds()
			return false, latency
		}
	}

	latency = time.Since(start).Milliseconds()
	return true, latency
}
