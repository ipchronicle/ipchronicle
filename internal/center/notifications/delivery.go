package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	"github.com/ipchronicle/ipchronicle/internal/probefields"
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
		return s.sendTelegram(ctx, *decoded.Configuration.Telegram, record.EventJson, record.Title, record.Body)
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

func (s *Service) sendTelegram(ctx context.Context, configuration TelegramConfiguration, eventJSON []byte, title, body string) DeliveryError {
	switch configuration.MessageFormat {
	case TelegramFormatImage:
		return s.sendTelegramImage(ctx, configuration, eventJSON)
	case TelegramFormatText:
		return s.sendTelegramText(ctx, configuration, eventJSON, title, body)
	default:
		return DeliveryError{Code: "sender-configuration-invalid"}
	}
}

func (s *Service) sendTelegramText(ctx context.Context, configuration TelegramConfiguration, eventJSON []byte, title, body string) DeliveryError {
	endpoint := "https://api.telegram.org/bot" + url.PathEscape(configuration.Token) + "/sendMessage"
	payload, err := json.Marshal(struct {
		ChatID            string `json:"chat_id"`
		Text              string `json:"text"`
		ParseMode         string `json:"parse_mode"`
		TopicID           *int64 `json:"message_thread_id,omitempty"`
		LinkPreviewOption struct {
			Disabled bool `json:"is_disabled"`
		} `json:"link_preview_options"`
	}{
		ChatID: configuration.ChatID, Text: telegramMessage(eventJSON, title, body), ParseMode: "HTML",
		TopicID: configuration.TopicID, LinkPreviewOption: struct {
			Disabled bool `json:"is_disabled"`
		}{Disabled: true},
	})
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

func (s *Service) sendTelegramImage(ctx context.Context, configuration TelegramConfiguration, eventJSON []byte) DeliveryError {
	imageData, err := renderTelegramImage(eventJSON)
	if err != nil {
		return DeliveryError{Code: "image-render-failed"}
	}
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	writeField := func(name, value string) bool {
		return writer.WriteField(name, value) == nil
	}
	if !writeField("chat_id", configuration.ChatID) {
		return DeliveryError{Code: "request-invalid"}
	}
	if configuration.TopicID != nil && !writeField("message_thread_id", strconv.FormatInt(*configuration.TopicID, 10)) {
		return DeliveryError{Code: "request-invalid"}
	}
	caption := telegramPhotoCaption(eventJSON)
	if caption != "" && (!writeField("caption", caption) || !writeField("parse_mode", "HTML")) {
		return DeliveryError{Code: "request-invalid"}
	}
	photo, err := writer.CreateFormFile("photo", "ipchronicle-notification.png")
	if err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	if _, err := photo.Write(imageData); err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	if err := writer.Close(); err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	endpoint := "https://api.telegram.org/bot" + url.PathEscape(configuration.Token) + "/sendPhoto"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &payload)
	if err != nil {
		return DeliveryError{Code: "request-invalid"}
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return s.sendHTTPRequest(request)
}

func telegramPhotoCaption(eventJSON []byte) string {
	var event eventEnvelope
	if len(eventJSON) == 0 || json.Unmarshal(eventJSON, &event) != nil || event.Link == nil || len(*event.Link) > 900 {
		return ""
	}
	return "<a href=\"" + html.EscapeString(*event.Link) + "\">" +
		localizedNotification(event.Locale, "View details", "查看详情") + "</a>"
}

func telegramMessage(eventJSON []byte, title, body string) string {
	var event eventEnvelope
	if len(eventJSON) > 0 && json.Unmarshal(eventJSON, &event) == nil && event.Type != "" {
		return renderTelegramEvent(title, event)
	}
	title = truncateUTF8(strings.TrimSpace(title), 256)
	body = truncateUTF8(strings.TrimSpace(body), 3700)
	message := "<b>" + html.EscapeString(title) + "</b>"
	if body != "" {
		message += "\n\n" + html.EscapeString(body)
	}
	return message
}

func renderTelegramEvent(title string, event eventEnvelope) string {
	const maximumChanges = 10
	zh := event.Locale == "zh-CN"
	var message strings.Builder
	message.WriteString("<b>")
	message.WriteString(html.EscapeString(truncateUTF8(strings.TrimSpace(title), 256)))
	message.WriteString("</b>")
	if event.Node != nil {
		message.WriteString("\n\n<b>")
		if zh {
			message.WriteString("节点")
		} else {
			message.WriteString("Node")
		}
		if zh {
			message.WriteString("</b>：")
		} else {
			message.WriteString("</b>: ")
		}
		message.WriteString(html.EscapeString(truncateUTF8(event.Node.Name, 128)))
	}
	if event.Egress != nil {
		message.WriteString("\n<b>")
		if zh {
			message.WriteString("公网 IP")
		} else {
			message.WriteString("Public IP")
		}
		if zh {
			message.WriteString("</b>：<code>")
		} else {
			message.WriteString("</b>: <code>")
		}
		message.WriteString(html.EscapeString(truncateUTF8(event.Egress.Name, 128)))
		message.WriteString("</code>")
	}
	if event.Type == EventProbeFieldChange {
		var data ProbeChangeData
		if json.Unmarshal(event.Data, &data) == nil && len(data.Changes) > 0 {
			message.WriteString("\n\n<b>")
			if zh {
				fmt.Fprintf(&message, "变更项（%d）", len(data.Changes))
			} else {
				fmt.Fprintf(&message, "Changes (%d)", len(data.Changes))
			}
			message.WriteString("</b>")
			for _, change := range data.Changes[:min(len(data.Changes), maximumChanges)] {
				name, ok := probefields.DisplayName(change.FieldID, event.Locale)
				if !ok {
					name = localizedNotification(event.Locale, "Unknown probe field", "未知探测字段")
				}
				before := truncateUTF8(probefields.DisplayValue(change.FieldID, change.Before, event.Locale), 120)
				after := truncateUTF8(probefields.DisplayValue(change.FieldID, change.After, event.Locale), 120)
				message.WriteString("\n• <b>")
				message.WriteString(html.EscapeString(name))
				message.WriteString("</b>\n  <code>")
				message.WriteString(html.EscapeString(before))
				message.WriteString("</code> → <code>")
				message.WriteString(html.EscapeString(after))
				message.WriteString("</code>")
			}
			if remaining := len(data.Changes) - maximumChanges; remaining > 0 {
				message.WriteString("\n")
				if zh {
					fmt.Fprintf(&message, "另有 %d 项，请打开详情查看。", remaining)
				} else {
					fmt.Fprintf(&message, "%d more changes are available in the details.", remaining)
				}
			}
		}
	} else {
		status, _, facts := genericEventContent(event)
		if status != "" {
			message.WriteString("\n\n<b>")
			message.WriteString(html.EscapeString(status))
			message.WriteString("</b>")
		}
		for _, fact := range facts {
			message.WriteString("\n")
			message.WriteString(html.EscapeString(fact.label))
			if zh {
				message.WriteString("：")
			} else {
				message.WriteString(": ")
			}
			message.WriteString(html.EscapeString(fact.value))
		}
	}
	if event.Link != nil && len(*event.Link) <= 1024 {
		message.WriteString("\n\n<a href=\"")
		message.WriteString(html.EscapeString(*event.Link))
		message.WriteString("\">")
		message.WriteString(localizedNotification(event.Locale, "View details", "查看详情"))
		message.WriteString("</a>")
	}
	return message.String()
}

func localizedNotification(locale, english, chinese string) string {
	if locale == "zh-CN" {
		return chinese
	}
	return english
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
