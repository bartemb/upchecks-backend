package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"upchecks-backend/internal/db"

	"github.com/jackc/pgx/v5"
)

type NotificationService struct {
	db *db.Queries
}

func NewNotificationService(queries *db.Queries) *NotificationService {
	return &NotificationService{db: queries}
}

type channelConfig struct {
	WebhookURL string `json:"webhook_url"`
}

func (s *NotificationService) CheckAndNotify(ctx context.Context, svc db.Service, check db.Check) {
	rules, err := s.db.GetNotificationRulesForService(ctx, svc.ID)
	if err != nil {
		log.Printf("failed to get notification rules for %s: %v", svc.Name, err)
		return
	}

	for _, rule := range rules {
		s.evaluateRule(ctx, svc, check, rule)
	}
}

func (s *NotificationService) evaluateRule(ctx context.Context, svc db.Service, check db.Check, rule db.NotificationRule) {
	channel, err := s.db.GetNotificationChannelByID(ctx, rule.ChannelID)
	if err != nil {
		log.Printf("failed to get notification channel: %v", err)
		return
	}

	var config channelConfig
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		log.Printf("failed to parse channel config: %v", err)
		return
	}

	lastChecks, err := s.db.GetLastNChecksByService(ctx, db.GetLastNChecksByServiceParams{
		ServiceID: svc.ID,
		Limit:     rule.Threshold,
	})
	if err != nil {
		log.Printf("failed to get last checks for %s: %v", svc.Name, err)
		return
	}

	if check.Success {
		s.handleRecovery(ctx, svc, check, rule, channel, config, lastChecks)
	} else {
		s.handleFailure(ctx, svc, check, rule, channel, config, lastChecks)
	}
}

func (s *NotificationService) handleFailure(ctx context.Context, svc db.Service, check db.Check, rule db.NotificationRule, channel db.NotificationChannel, config channelConfig, lastChecks []db.Check) {
	if int32(len(lastChecks)) < rule.Threshold {
		return
	}

	// Check if all last N checks failed
	for _, c := range lastChecks {
		if c.Success {
			return
		}
	}

	// Check if we already sent a notification for this down period
	lastNotification, err := s.db.GetLastNotificationForRule(ctx, rule.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("failed to get last notification: %v", err)
		return
	}

	// If we already notified and there hasn't been a successful check since, skip
	if err == nil {
		// Find if there was a successful check after the last notification
		hasRecovered := false
		allChecks, err := s.db.GetLastNChecksByService(ctx, db.GetLastNChecksByServiceParams{
			ServiceID: svc.ID,
			Limit:     100,
		})
		if err == nil {
			for _, c := range allChecks {
				if c.CreatedAt.Time.Before(lastNotification.SentAt.Time) {
					break
				}
				if c.Success {
					hasRecovered = true
					break
				}
			}
		}
		if !hasRecovered {
			return
		}
	}

	reason := "Timeout"
	if check.StatusCode != nil {
		reason = fmt.Sprintf("Status code %d", *check.StatusCode)
	}

	embed := discordEmbed{
		Title:       fmt.Sprintf("Upchecks - %s is offline ❌", svc.Name),
		Description: fmt.Sprintf("Your service **%s** did not respond for %d times.\n\n**Reason:** %s", svc.Name, rule.Threshold, reason),
		Color:       0xFF0000, // red
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	status := db.NotificationStatusSent
	if err := s.sendDiscordMessage(config.WebhookURL, embed); err != nil {
		log.Printf("failed to send Discord notification for %s: %v", svc.Name, err)
		status = db.NotificationStatusFailed
	}

	s.db.CreateNotificationHistory(ctx, db.CreateNotificationHistoryParams{
		RuleID:    rule.ID,
		ChannelID: channel.ID,
		CheckID:   check.ID,
		Status:    status,
	})
}

func (s *NotificationService) handleRecovery(ctx context.Context, svc db.Service, check db.Check, rule db.NotificationRule, channel db.NotificationChannel, config channelConfig, lastChecks []db.Check) {
	// Need at least 2 checks to detect recovery (current success + previous failure)
	if len(lastChecks) < 2 {
		return
	}

	// Previous check must have been a failure
	if lastChecks[1].Success {
		return
	}

	// Only send recovery if we previously sent a down notification
	_, err := s.db.GetLastNotificationForRule(ctx, rule.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("failed to get last notification: %v", err)
		return
	}

	embed := discordEmbed{
		Title:       fmt.Sprintf("Upchecks - %s is online ✅", svc.Name),
		Description: fmt.Sprintf("Your service **%s** is back online.", svc.Name),
		Color:       0x00FF00, // green
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	status := db.NotificationStatusSent
	if err := s.sendDiscordMessage(config.WebhookURL, embed); err != nil {
		log.Printf("failed to send Discord recovery notification for %s: %v", svc.Name, err)
		status = db.NotificationStatusFailed
	}

	s.db.CreateNotificationHistory(ctx, db.CreateNotificationHistoryParams{
		RuleID:    rule.ID,
		ChannelID: channel.ID,
		CheckID:   check.ID,
		Status:    status,
	})
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

func (s *NotificationService) sendDiscordMessage(webhookURL string, embed discordEmbed) error {
	payload := discordWebhookPayload{
		Embeds: []discordEmbed{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord payload: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send Discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}
