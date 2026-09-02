package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestTelegramImageDeliveryUsesPhotoCaptionAndTopic(t *testing.T) {
	topicID := int64(42)
	link := "https://ipchronicle.example/probe-snapshots/test?source=telegram&value=1"
	eventJSON, err := json.Marshal(eventEnvelope{
		SchemaVersion: 1, Locale: "zh-CN", Type: EventProbeFieldChange,
		ObservedAt: "2026-09-02T10:42:00Z", RecordedAt: "2026-09-02T10:42:01Z",
		Node: &resourceRef{Name: "tokyo-edge-01"}, Egress: &egressRef{Name: "103.212.45.18", Family: "ipv4"},
		Data: mustJSON(t, ProbeChangeData{Changes: []FieldChange{
			{FieldID: "Type.Usage.ipapi", Before: `"ISP"`, After: `"Hosting"`},
		}}),
		Link: &link,
	})
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/bottest-token/sendPhoto" {
			t.Fatalf("Telegram image request = %s %s", request.Method, request.URL.String())
		}
		fields, photo := readMultipartTelegramRequest(t, request)
		if fields["chat_id"] != "-1001234567890" || fields["message_thread_id"] != strconv.FormatInt(topicID, 10) ||
			fields["parse_mode"] != "HTML" || fields["caption"] != `<a href="https://ipchronicle.example/probe-snapshots/test?source=telegram&amp;value=1">查看详情</a>` {
			t.Fatalf("Telegram image fields = %#v", fields)
		}
		decoded, err := png.Decode(bytes.NewReader(photo))
		if err != nil {
			t.Fatalf("decode Telegram photo: %v", err)
		}
		if decoded.Bounds().Dx() != telegramImageWidth || decoded.Bounds().Dy() < 400 {
			t.Fatalf("Telegram photo bounds = %v", decoded.Bounds())
		}
		return successfulTelegramResponse(request), nil
	})}
	service := &Service{httpClient: client}
	result := service.sendTelegram(context.Background(), TelegramConfiguration{
		ChatID: "-1001234567890", Token: "test-token", TopicID: &topicID,
		MessageFormat: TelegramFormatImage,
	}, eventJSON, "IP 质量发生变化", "")
	if !result.Empty() {
		t.Fatalf("Telegram image delivery result = %#v", result)
	}
}

func TestTelegramImageSupportsEveryEventType(t *testing.T) {
	tests := []struct {
		eventType string
		data      any
	}{
		{EventProbeFieldChange, ProbeChangeData{Changes: []FieldChange{{FieldID: "Factor.VPN.IPQS", Before: "false", After: "true"}}}},
		{EventAddressChange, AddressData{Kind: "address-added", Family: "ipv4", ProxyPath: true}},
		{EventAddressCheckFailure, AddressData{Kind: "check-failure", Family: "ipv6", FailureReason: imageStringPointer("no-valid-response")}},
		{EventAddressCheckRecover, AddressData{Kind: "recovery", Family: "ipv4"}},
		{EventProbeFailure, ProbeOutcomeData{Status: "failed", FailureStage: imageStringPointer("timeout")}},
		{EventProbeRecovery, ProbeOutcomeData{Status: "succeeded"}},
		{EventAddressGap, GapData{Kind: "address", DroppedCount: 4, FirstSequence: 10, LastSequence: 13}},
		{EventProbeGap, GapData{Kind: "probe", DroppedCount: 2, FirstSequence: 20, LastSequence: 21}},
		{EventFormatMismatch, FormatData{Kind: "mismatch", IssueCount: 3}},
		{EventFormatChanged, FormatData{Kind: "changed", IssueCount: 2}},
		{EventFormatRecovery, FormatData{Kind: "recovered"}},
		{EventTest, map[string]string{"message": "test"}},
	}
	for _, test := range tests {
		t.Run(test.eventType, func(t *testing.T) {
			var egress *egressRef
			if test.eventType != EventAddressGap && test.eventType != EventTest {
				egress = &egressRef{Name: "2001:db8::1", Family: "ipv6"}
			}
			eventJSON, err := json.Marshal(eventEnvelope{
				SchemaVersion: 1, Locale: "en", Type: test.eventType,
				ObservedAt: "2026-09-02T10:42:00Z", RecordedAt: "2026-09-02T10:42:01Z",
				Node:   &resourceRef{Name: "edge-node"},
				Egress: egress, Data: mustJSON(t, test.data),
			})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := renderTelegramImage(eventJSON)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds().Dx() != telegramImageWidth || decoded.Bounds().Dy() < 400 || len(encoded) < 4096 {
				t.Fatalf("rendered image = %v, %d bytes", decoded.Bounds(), len(encoded))
			}
		})
	}
}

func TestTelegramImageBoundsChangesAndUsesFieldSemantics(t *testing.T) {
	changes := []FieldChange{
		{FieldID: "Type.Usage.ipapi", Before: `"ISP"`, After: `"Hosting"`},
		{FieldID: "Factor.VPN.IPQS", Before: "false", After: "true"},
		{FieldID: "Media.Netflix.Status", Before: `"Unlocked"`, After: `"NF.only"`},
		{FieldID: "Mail.Gmail", Before: "false", After: "true"},
		{FieldID: "Score.IPQS", Before: `"4"`, After: `"86"`},
	}
	for range 7 {
		changes = append(changes, FieldChange{FieldID: "Info.Organization", Before: `"A"`, After: `"B"`})
	}
	event := eventEnvelope{
		Locale: "zh-CN", Type: EventProbeFieldChange, ObservedAt: "2026-09-02T10:42:00Z",
		Data: mustJSON(t, ProbeChangeData{Changes: changes}),
	}
	content := buildTelegramImageContent(event)
	if len(content.changes) != maximumImageChanges || content.remaining != 2 {
		t.Fatalf("bounded image changes = %d + %d", len(content.changes), content.remaining)
	}
	want := [][2]imageTone{
		{toneGreen, toneRed}, {toneGreen, toneRed}, {toneGreen, toneAmber},
		{toneRed, toneGreen}, {toneGreen, toneRed},
	}
	for index, tones := range want {
		actual := content.changes[index]
		if actual.beforeTone != tones[0] || actual.afterTone != tones[1] {
			t.Errorf("change %d tones = %v -> %v, want %v -> %v", index, actual.beforeTone, actual.afterTone, tones[0], tones[1])
		}
	}
	wantValues := [][2]string{
		{"家宽", "机房"}, {"未检测到", "检测到"}, {"解锁", "仅自制内容"},
		{"不可用", "可用"}, {"4（低）", "86（存在风险）"},
	}
	for index, values := range wantValues {
		actual := content.changes[index]
		if actual.before != values[0] || actual.after != values[1] {
			t.Errorf("change %d values = %q -> %q, want %q -> %q", index, actual.before, actual.after, values[0], values[1])
		}
	}
	if strings.Contains(content.changes[0].label, "Type.Usage") {
		t.Fatalf("image exposed internal field ID: %#v", content.changes[0])
	}
}

func TestProbeValueToneParsesPercentageScores(t *testing.T) {
	if tone := probeValueTone("Score.ipapi", `"4.00%"`, "4.00%（高）"); tone != toneRed {
		t.Fatalf("ipapi percentage score tone = %v, want %v", tone, toneRed)
	}
}

func TestTelegramSenderRejectsMissingMessageFormat(t *testing.T) {
	configuration := SenderConfiguration{Telegram: &TelegramConfiguration{ChatID: "123", Token: "token"}}
	if err := validateSender("Telegram", SenderTelegram, configuration); err != ErrInvalidSender {
		t.Fatalf("missing Telegram message format error = %v", err)
	}
	configuration.Telegram.MessageFormat = TelegramFormatImage
	if err := validateSender("Telegram", SenderTelegram, configuration); err != nil {
		t.Fatalf("image Telegram sender validation = %v", err)
	}
	configuration.Telegram.MessageFormat = TelegramFormatText
	if err := validateSender("Telegram", SenderTelegram, configuration); err != nil {
		t.Fatalf("text Telegram sender validation = %v", err)
	}
}

func TestTelegramTextIncludesGenericEventDetails(t *testing.T) {
	reason := "no-valid-response"
	eventJSON, err := json.Marshal(eventEnvelope{
		Locale: "zh-CN", Type: EventAddressCheckFailure,
		Node: &resourceRef{Name: "edge-node"}, Egress: &egressRef{Name: "203.0.113.10", Family: "ipv4"},
		Data: mustJSON(t, AddressData{FailureReason: &reason, ProxyPath: true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	message := telegramMessage(eventJSON, "地址检查失败", "")
	for _, expected := range []string{"未收到有效的发现结果", "发现路径：网络代理", "203.0.113.10"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("Telegram text does not contain %q: %s", expected, message)
		}
	}
}

func readMultipartTelegramRequest(t *testing.T, request *http.Request) (map[string]string, []byte) {
	t.Helper()
	reader, err := request.MultipartReader()
	if err != nil {
		t.Fatal(err)
	}
	fields := make(map[string]string)
	var photo []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		value, err := io.ReadAll(io.LimitReader(part, 10*1024*1024))
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() == "photo" {
			if part.FileName() != "ipchronicle-notification.png" {
				t.Fatalf("Telegram photo filename = %q", part.FileName())
			}
			photo = value
		} else {
			fields[part.FormName()] = string(value)
		}
	}
	if len(photo) == 0 {
		t.Fatal("Telegram multipart request has no photo")
	}
	return fields, photo
}

func successfulTelegramResponse(request *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Request: request,
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func imageStringPointer(value string) *string {
	return &value
}
