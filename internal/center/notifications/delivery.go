package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

const maximumSenderResponseBytes = 1024 * 1024

type DeliveryError struct {
	Code      string
	Retryable bool
}

func (e DeliveryError) Empty() bool {
	return e.Code == ""
}

func (s *Service) processOneDelivery(ctx context.Context, javascript bool) (bool, error) {
	now := s.now().UTC().Unix()
	var records []historydb.NotificationDelivery
	var err error
	if javascript {
		records, err = s.historyQueries.ListReadyJavaScriptNotificationDeliveries(ctx, &now)
	} else {
		records, err = s.historyQueries.ListReadyFixedNotificationDeliveries(ctx, historydb.ListReadyFixedNotificationDeliveriesParams{
			NextAttemptAt: &now, Limit: 1,
		})
	}
	if err != nil || len(records) == 0 {
		return false, err
	}
	record := records[0]
	claimed, err := s.historyQueries.ClaimNotificationDelivery(ctx, historydb.ClaimNotificationDeliveryParams{
		LastAttemptAt: &now, UpdatedAt: now, ID: record.ID, NextAttemptAt: &now,
	})
	if err != nil || claimed == 0 {
		return false, err
	}
	record, err = s.historyQueries.GetNotificationDelivery(ctx, record.ID)
	if err != nil {
		return true, err
	}
	deliveryError := s.dispatch(ctx, record)
	finishedAt := s.now().UTC().Unix()
	if deliveryError.Empty() {
		changed, err := s.historyQueries.CompleteNotificationDelivery(ctx, historydb.CompleteNotificationDeliveryParams{
			CompletedAt: &finishedAt, UpdatedAt: finishedAt, ID: record.ID,
		})
		return true, requireDeliveryTransition(changed, err)
	}
	if deliveryError.Retryable && record.AttemptCount < maximumAttempts {
		next := finishedAt + int64(retryDelay(record.AttemptCount).Seconds())
		changed, err := s.historyQueries.RetryNotificationDelivery(ctx, historydb.RetryNotificationDeliveryParams{
			NextAttemptAt: &next, UpdatedAt: finishedAt, ID: record.ID,
		})
		return true, requireDeliveryTransition(changed, err)
	}
	code := deliveryError.Code
	changed, err := s.historyQueries.FailNotificationDelivery(ctx, historydb.FailNotificationDeliveryParams{
		CompletedAt: &finishedAt, ErrorCode: &code, UpdatedAt: finishedAt, ID: record.ID,
	})
	return true, requireDeliveryTransition(changed, err)
}

func (s *Service) dispatch(ctx context.Context, record historydb.NotificationDelivery) DeliveryError {
	sender, err := s.configQueries.GetNotificationSender(ctx, record.SenderID)
	if err != nil {
		return DeliveryError{Code: "sender-unavailable"}
	}
	if sender.Enabled != 1 {
		return DeliveryError{Code: "sender-disabled"}
	}
	if sender.Kind != record.SenderKind {
		return DeliveryError{Code: "sender-changed"}
	}
	decoded, err := s.senderFromRecord(sender)
	if err != nil {
		return DeliveryError{Code: "sender-configuration-invalid"}
	}
	switch decoded.Kind {
	case SenderTelegram:
		return s.sendTelegram(ctx, *decoded.Configuration.Telegram, record.Title, record.Body)
	case SenderWebhook:
		return s.sendWebhook(ctx, *decoded.Configuration.Webhook, record.EventJson)
	case SenderJavaScript:
		return s.javascript.Run(ctx, JavaScriptRequest{
			Script: decoded.Configuration.JavaScript.Source,
			Event:  record.EventJson, Title: record.Title, Body: record.Body,
		})
	default:
		return DeliveryError{Code: "sender-configuration-invalid"}
	}
}

func (s *Service) sendTelegram(ctx context.Context, configuration TelegramConfiguration, title, body string) DeliveryError {
	endpoint := "https://api.telegram.org/bot" + url.PathEscape(configuration.Token) + "/sendMessage"
	payload, err := json.Marshal(struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{ChatID: configuration.ChatID, Text: title + "\n\n" + body})
	if err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	request.Header.Set("Content-Type", "application/json")
	return s.sendHTTPRequest(request)
}

func (s *Service) sendWebhook(ctx context.Context, configuration WebhookConfiguration, event []byte) DeliveryError {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, configuration.URL, bytes.NewReader(event))
	if err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "IPChronicle-Notification/1")
	for name, value := range configuration.Headers {
		request.Header.Set(name, value)
	}
	return s.sendHTTPRequest(request)
}

func (s *Service) sendHTTPRequest(request *http.Request) DeliveryError {
	response, err := s.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return DeliveryError{Code: "request-timeout", Retryable: true}
		}
		return DeliveryError{Code: "request-failed", Retryable: true}
	}
	defer response.Body.Close()
	read, err := io.CopyN(io.Discard, response.Body, maximumSenderResponseBytes+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return DeliveryError{Code: "response-read-failed", Retryable: true}
	}
	if read > maximumSenderResponseBytes {
		return DeliveryError{Code: "response-too-large"}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return DeliveryError{}
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		return DeliveryError{Code: fmt.Sprintf("http-%d", response.StatusCode), Retryable: true}
	}
	return DeliveryError{Code: fmt.Sprintf("http-%d", response.StatusCode)}
}

func retryDelay(attempt int64) time.Duration {
	switch attempt {
	case 1:
		return 10 * time.Second
	case 2:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

func requireDeliveryTransition(changed int64, err error) error {
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("notification delivery lost its durable state transition")
	}
	return nil
}

func safeWorkerCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 64 {
		return "worker-failed"
	}
	for _, character := range value {
		if character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return "worker-failed"
		}
	}
	return value
}
