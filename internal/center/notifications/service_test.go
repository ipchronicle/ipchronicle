package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	"github.com/ipchronicle/ipchronicle/internal/center/systemsettings"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "notification-worker" {
		if err := RunJavaScriptWorker(os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSenderRulesAggregateOneDeliveryAndEncryptConfiguration(t *testing.T) {
	service, store, nodeID, publicAddressID := newNotificationTestService(t)
	secret := "telegram-secret-that-must-not-appear"
	sender, err := service.CreateSender(context.Background(), SenderCreate{
		Name: "owner", Kind: SenderTelegram, Enabled: true,
		Configuration: SenderConfiguration{Telegram: &TelegramConfiguration{ChatID: "12345", Token: secret}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var encrypted []byte
	if err := store.Config.QueryRow(`SELECT configuration_encrypted FROM notification_senders WHERE id = ?`, sender.ID.String()).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte(secret)) {
		t.Fatal("sender token is present in configuration.db ciphertext")
	}
	fieldOne := "ipquality.ipinfo.CountryCode"
	fieldTwo := "ipquality.ipinfo.Organization"
	for index, field := range []string{fieldOne, fieldTwo} {
		rule, err := service.CreateRule(context.Background(), RuleCreate{
			Name: "rule-" + string(rune('a'+index)), Enabled: true, SenderID: sender.ID,
			EventType: EventProbeFieldChange, FieldID: &field, NodeID: &nodeID,
			EgressID: &publicAddressID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if rule.PublicAddress == nil || *rule.PublicAddress != "203.0.113.10" {
			t.Fatalf("created rule public address = %v", rule.PublicAddress)
		}
	}
	rules, err := service.Rules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].PublicAddress == nil || *rules[0].PublicAddress != "203.0.113.10" {
		t.Fatalf("listed rules = %#v", rules)
	}
	node := nodeID.String()
	egress := publicAddressID.String()
	err = CreateEvent(context.Background(), store.HistoryQueries, EventInput{
		Type: EventProbeFieldChange, SourceKind: "probe-change-set", SourceID: uuid.NewString(),
		NodeID: &node, EgressID: &egress, ObservedAt: 100, RecordedAt: 101,
		Payload: ProbeChangeData{
			ExecutionID: uuid.NewString(), SnapshotID: uuid.NewString(), Sequence: 2,
			Changes: []FieldChange{
				{FieldID: fieldOne, Group: "ipquality", Path: "IPQuality.ipinfo.CountryCode", ValueType: "string", Before: `"US"`, After: `"DE"`},
				{FieldID: fieldTwo, Group: "ipquality", Path: "IPQuality.ipinfo.Organization", ValueType: "string", Before: `"A"`, After: `"B"`},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Unix(200, 0).UTC() }
	if err := service.processPendingEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := service.Deliveries(context.Background(), DeliveryFilter{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || len(page.Items[0].MatchedRuleIDs) != 2 || page.Items[0].Status != "pending" {
		t.Fatalf("aggregated deliveries = %#v", page.Items)
	}
	if !bytes.Contains(page.Items[0].Event, []byte(fieldOne)) || !bytes.Contains(page.Items[0].Event, []byte(fieldTwo)) {
		t.Fatalf("delivery event lost matched changes: %s", page.Items[0].Event)
	}
	if err := service.processPendingEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err = service.Deliveries(context.Background(), DeliveryFilter{Page: 1, PageSize: 50})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("idempotent event processing = %#v, %v", page, err)
	}
}

func TestNotificationLinksReadCurrentExternalOriginSetting(t *testing.T) {
	service, _, _, _ := newNotificationTestService(t)
	event := historydb.NotificationEvent{EventType: EventAddressChange}

	link, err := service.eventLink(context.Background(), event)
	if err != nil || link != nil {
		t.Fatalf("automatic notification link = %v, %v", link, err)
	}
	if _, err := service.systemSettings.Update(context.Background(), "https://first.example"); err != nil {
		t.Fatal(err)
	}
	link, err = service.eventLink(context.Background(), event)
	if err != nil || link == nil || *link != "https://first.example/history" {
		t.Fatalf("first notification link = %v, %v", link, err)
	}
	if _, err := service.systemSettings.Update(context.Background(), "https://second.example"); err != nil {
		t.Fatal(err)
	}
	link, err = service.eventLink(context.Background(), event)
	if err != nil || link == nil || *link != "https://second.example/history" {
		t.Fatalf("updated notification link = %v, %v", link, err)
	}
}

func TestWebhookTestDeliveryUsesRealPayloadPath(t *testing.T) {
	received := make(chan map[string]any, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("X-Test-Token") != "local-secret" {
			t.Errorf("unexpected request method or header")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		var event map[string]any
		if err := json.Unmarshal(body, &event); err != nil {
			t.Error(err)
		}
		received <- event
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(receiver.Close)
	service, _, _, _ := newNotificationTestService(t)
	sender, err := service.CreateSender(context.Background(), SenderCreate{
		Name: "local webhook", Kind: SenderWebhook, Enabled: true,
		Configuration: SenderConfiguration{Webhook: &WebhookConfiguration{
			URL: receiver.URL, Headers: map[string]string{"X-Test-Token": "local-secret"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := service.CreateTestDelivery(context.Background(), sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	worked, err := service.processOneDelivery(context.Background(), false)
	if err != nil || !worked {
		t.Fatalf("process test delivery = %v, %v", worked, err)
	}
	delivery, err = service.Delivery(context.Background(), delivery.ID)
	if err != nil || delivery.Status != "succeeded" || delivery.AttemptCount != 1 {
		t.Fatalf("completed test delivery = %#v, %v", delivery, err)
	}
	select {
	case event := <-received:
		if event["type"] != EventTest || event["schemaVersion"] != float64(1) {
			t.Fatalf("received event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("local webhook did not receive the test delivery")
	}
}

func TestFailedWebhookReachesTerminalAttemptLimit(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(receiver.Close)
	service, _, _, _ := newNotificationTestService(t)
	clock := time.Unix(1000, 0).UTC()
	service.now = func() time.Time { return clock }
	sender, err := service.CreateSender(context.Background(), SenderCreate{
		Name: "failing webhook", Kind: SenderWebhook, Enabled: true,
		Configuration: SenderConfiguration{Webhook: &WebhookConfiguration{URL: receiver.URL, Headers: map[string]string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := service.CreateTestDelivery(context.Background(), sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := int64(1); attempt <= maximumAttempts; attempt++ {
		worked, err := service.processOneDelivery(context.Background(), false)
		if err != nil || !worked {
			t.Fatalf("attempt %d = %v, %v", attempt, worked, err)
		}
		current, err := service.Delivery(context.Background(), delivery.ID)
		if err != nil {
			t.Fatal(err)
		}
		if attempt < maximumAttempts {
			if current.Status != "retrying" || current.NextAttemptAt == nil {
				t.Fatalf("retry state after attempt %d = %#v", attempt, current)
			}
			clock = current.NextAttemptAt.Add(time.Second)
		} else if current.Status != "failed" || current.ErrorCode == nil || *current.ErrorCode != "http-503" {
			t.Fatalf("terminal state = %#v", current)
		}
	}
}

func TestNotificationQueueOverflowIsTerminal(t *testing.T) {
	service, store, _, _ := newNotificationTestService(t)
	sender, err := service.CreateSender(context.Background(), SenderCreate{
		Name: "bounded webhook", Kind: SenderWebhook, Enabled: true,
		Configuration: SenderConfiguration{Webhook: &WebhookConfiguration{
			URL: "https://notification.invalid/delivery", Headers: map[string]string{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maximumActiveDeliveriesPerSender; index++ {
		delivery, err := service.CreateTestDelivery(context.Background(), sender.ID)
		if err != nil || delivery.Status != "pending" {
			t.Fatalf("active delivery %d = %#v, %v", index, delivery, err)
		}
	}
	overflow, err := service.CreateTestDelivery(context.Background(), sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if overflow.Status != "failed" || overflow.ErrorCode == nil || *overflow.ErrorCode != "queue-full" ||
		overflow.CompletedAt == nil || overflow.NextAttemptAt != nil {
		t.Fatalf("overflow delivery = %#v", overflow)
	}
	active, err := store.HistoryQueries.CountActiveNotificationDeliveriesForSender(context.Background(), sender.ID.String())
	if err != nil || active != maximumActiveDeliveriesPerSender {
		t.Fatalf("active deliveries = %d, want %d: %v", active, maximumActiveDeliveriesPerSender, err)
	}
}

func TestJavaScriptWorkerHTTPAndIsolation(t *testing.T) {
	received := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received <- string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(receiver.Close)
	runner := ProcessJavaScriptRunner{Executable: os.Args[0], Timeout: functionalWorkerTestTimeout()}
	script := `
		if (typeof process !== "undefined" || typeof require !== "undefined" || typeof fetch !== "undefined") {
			throw new Error("host leaked");
		}
		var response = ipchronicle.http.request({
			method: "POST",
			url: ` + quoteJavaScript(receiver.URL) + `,
			headers: {"Content-Type": "application/json"},
			body: JSON.stringify({id: ipchronicle.event.id, title: ipchronicle.title})
		});
		if (response.status !== 200 || response.body !== "ok") throw new Error("unexpected response");
	`
	result := runner.Run(context.Background(), JavaScriptRequest{
		Script: script, Event: json.RawMessage(`{"id":"event-1"}`), Title: "title", Body: "body",
	})
	if !result.Empty() {
		t.Fatalf("worker result = %#v", result)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, "event-1") || !strings.Contains(body, "title") {
			t.Fatalf("worker request body = %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("worker HTTP request was not received")
	}
}

func TestJavaScriptWorkerRedactsExceptionAndStopsRunawayWork(t *testing.T) {
	timeout := boundaryWorkerTestTimeout()
	runner := ProcessJavaScriptRunner{Executable: os.Args[0], Timeout: timeout}
	secret := "secret-value-must-not-return"
	result := runner.Run(context.Background(), JavaScriptRequest{
		Script: `throw new Error("` + secret + `")`, Event: json.RawMessage(`{"id":"event-1"}`),
		Title: "title", Body: "body",
	})
	if result.Code != "script-error" || strings.Contains(result.Code, secret) {
		t.Fatalf("exception result = %#v", result)
	}
	started := time.Now()
	result = runner.Run(context.Background(), JavaScriptRequest{
		Script: `for (;;) {}`, Event: json.RawMessage(`{"id":"event-1"}`), Title: "title", Body: "body",
	})
	if result.Code != "worker-timeout" ||
		time.Since(started) > javaScriptWorkerStartupTimeout+timeout+javaScriptWorkerShutdownGrace+time.Second {
		t.Fatalf("runaway result = %#v after %s", result, time.Since(started))
	}
}

func TestJavaScriptWorkerStopsBlockedHTTPAndAllocation(t *testing.T) {
	timeout := boundaryWorkerTestTimeout()
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		timer := time.NewTimer(2 * timeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			_, _ = w.Write([]byte("late"))
		case <-request.Context().Done():
		}
	}))
	t.Cleanup(receiver.Close)
	runner := ProcessJavaScriptRunner{Executable: os.Args[0], Timeout: timeout}
	blocked := runner.Run(context.Background(), JavaScriptRequest{
		Script: `ipchronicle.http.request({url: ` + quoteJavaScript(receiver.URL) + `})`,
		Event:  json.RawMessage(`{"id":"event-1"}`), Title: "title", Body: "body",
	})
	if blocked.Code != "worker-timeout" {
		t.Fatalf("blocked HTTP result = %#v", blocked)
	}
	allocation := runner.Run(context.Background(), JavaScriptRequest{
		Script: `var values=[]; for (;;) { values.push("x".repeat(262144) + values.length); }`,
		Event:  json.RawMessage(`{"id":"event-1"}`), Title: "title", Body: "body",
	})
	if allocation.Empty() || allocation.Code != "worker-failed" && allocation.Code != "worker-timeout" {
		t.Fatalf("allocation result = %#v", allocation)
	}
}

func boundaryWorkerTestTimeout() time.Duration {
	if raceInstrumentationEnabled {
		return 3 * time.Second
	}
	return 500 * time.Millisecond
}

func functionalWorkerTestTimeout() time.Duration {
	if raceInstrumentationEnabled {
		return javaScriptWorkerTimeout
	}
	return 3 * time.Second
}

func newNotificationTestService(t *testing.T) (*Service, *database.Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	store, err := database.Open(ctx, database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	administrator := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	if err := administrator.Bootstrap(ctx, "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	nodeID := uuid.New()
	pathID := uuid.New()
	publicAddressID := uuid.New()
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, hostname, credential_digest, agent_version,
			operating_system, architecture, desired_configuration_revision, registered_at
		) VALUES (?, 'edge-1', 'edge-1', ?, 'test', 'linux', 'amd64', 1, 1)
	`, nodeID.String(), bytes.Repeat([]byte{1}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO network_egresses (
			id, node_id, name, kind, family, enabled, available, automatic,
			lightweight_interval_seconds, probe_on_address_change, created_at, updated_at
		) VALUES (?, ?, 'default IPv4', 'default', 'ipv4', 1, 1, 1, 600, 1, 1, 1)
	`, pathID.String(), nodeID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO public_addresses (
			id, address, family, probe_enabled, probe_on_rediscovery,
			selected_path_id, first_seen_at, last_seen_at, updated_at
		) VALUES (?, '203.0.113.10', 'ipv4', 1, 1, ?, 1, 1, 1)
	`, publicAddressID.String(), pathID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO public_address_nodes (
			public_address_id, node_id, first_seen_at, last_seen_at
		) VALUES (?, ?, 1, 1)
	`, publicAddressID.String(), nodeID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Config.ExecContext(ctx, `
		INSERT INTO public_address_paths (
			public_address_id, path_id, node_id, local_interface, local_address,
			proxy_path, likely_nat, temporary, available, last_checked_at,
			last_succeeded_at
		) VALUES (?, ?, ?, 'eth0', '10.0.0.2', 0, 1, 0, 1, 1, 1)
	`, publicAddressID.String(), pathID.String(), nodeID.String()); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceOptions{
		ConfigDatabase: store.Config, HistoryDatabase: store.History,
		ConfigQueries: store.ConfigQueries, HistoryQueries: store.HistoryQueries,
		MasterKey: store.MasterKey, SystemSettings: systemsettings.NewService(store.ConfigQueries), Executable: os.Args[0],
	})
	return service, store, nodeID, publicAddressID
}

func quoteJavaScript(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
