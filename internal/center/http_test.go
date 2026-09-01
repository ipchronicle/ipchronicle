package center

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
	"github.com/ipchronicle/ipchronicle/internal/center/syncws"
	"github.com/ipchronicle/ipchronicle/internal/center/systemsettings"
	centerupdates "github.com/ipchronicle/ipchronicle/internal/center/updates"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

func TestHealthDoesNotRequireAuthentication(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	response := performRequest(handler, http.MethodGet, "/healthz", nil, "", nil)
	if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}

func TestAdministratorLoginStatusAndLogout(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	unauthenticated := performRequest(handler, http.MethodGet, "/api/v1/system/status", nil, "", nil)
	assertErrorCode(t, unauthenticated, http.StatusUnauthorized, api.Unauthenticated)
	unauthenticatedOverview := performRequest(handler, http.MethodGet, "/api/v1/overview", nil, "", nil)
	assertErrorCode(t, unauthenticatedOverview, http.StatusUnauthorized, api.Unauthenticated)

	missingOrigin := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "", nil)
	assertErrorCode(t, missingOrigin, http.StatusForbidden, api.OriginNotAllowed)

	login := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "http://example.test", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var session api.AuthenticatedSession
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.Account.Username != "admin" || !session.Account.UsesDefaultCredentials || session.CsrfToken == "" {
		t.Fatalf("unexpected login response: %#v", session)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies = %#v, want exactly one", cookies)
	}
	if cookies[0].Name != administratorSessionCookie || !cookies[0].HttpOnly || cookies[0].Secure {
		t.Fatalf("unexpected session cookie: %#v", cookies[0])
	}
	cookie := cookies[0]

	statusResponse := performRequest(handler, http.MethodGet, "/api/v1/system/status", nil, "", cookie)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status api.SystemStatus
	if err := json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Service != api.IpchronicleCenter || status.Status != api.Ok || status.SourceRevision != "test-revision" ||
		!status.TransportWarning || status.ConfigSchemaVersion != 1 || status.HistorySchemaVersion != 1 {
		t.Fatalf("unexpected status response: %#v", status)
	}
	overviewResponse := performRequest(handler, http.MethodGet, "/api/v1/overview", nil, "", cookie)
	if overviewResponse.Code != http.StatusOK {
		t.Fatalf("overview status = %d, body = %s", overviewResponse.Code, overviewResponse.Body.String())
	}
	var overview api.Overview
	if err := json.NewDecoder(overviewResponse.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if len(overview.Nodes) != 0 || len(overview.ActiveTasks) != 0 || overview.CheckedAt.IsZero() {
		t.Fatalf("unexpected empty overview: %#v", overview)
	}

	missingCSRF := performRequest(handler, http.MethodPost, "/api/v1/auth/logout", nil, "http://example.test", cookie)
	assertErrorCode(t, missingCSRF, http.StatusForbidden, api.CsrfFailed)
	wrongOrigin := performRequestWithCSRF(handler, http.MethodPost, "/api/v1/auth/logout", nil, "https://wrong.example", cookie, session.CsrfToken)
	assertErrorCode(t, wrongOrigin, http.StatusForbidden, api.OriginNotAllowed)
	logout := performRequestWithCSRF(handler, http.MethodPost, "/api/v1/auth/logout", nil, "http://example.test", cookie, session.CsrfToken)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}
	revoked := performRequest(handler, http.MethodGet, "/api/v1/auth/session", nil, "", cookie)
	assertErrorCode(t, revoked, http.StatusUnauthorized, api.Unauthenticated)
}

func TestMalformedJSONUsesStructuredError(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	response := performRequest(handler, http.MethodPost, "/api/v1/auth/login", []byte("{"), "http://example.test", nil)
	assertErrorCode(t, response, http.StatusBadRequest, api.InvalidRequest)
}

func TestExternalOriginSystemSettingDoesNotAuthorizeBrowserRequests(t *testing.T) {
	handler, nodeService, _ := newTestHTTPHandlerWithNodes(t, nil)
	if _, err := nodeService.RotateEnrollmentKey(context.Background(), "UTC"); err != nil {
		t.Fatal(err)
	}
	cookie, session := loginTestAdministrator(t, handler)

	automatic := performRequest(handler, http.MethodGet, "/api/v1/system/settings", nil, "", cookie)
	if automatic.Code != http.StatusOK {
		t.Fatalf("automatic settings status = %d, body = %s", automatic.Code, automatic.Body.String())
	}
	var settings api.SystemSettings
	if err := json.NewDecoder(automatic.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Automatic || settings.ExternalOrigin != "" || settings.EffectiveOrigin != "http://example.test" {
		t.Fatalf("automatic settings = %#v", settings)
	}

	invalid := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/system/settings",
		[]byte(`{"externalOrigin":"https://public.example/path"}`),
		"http://example.test", cookie, session.CsrfToken,
	)
	assertErrorCode(t, invalid, http.StatusBadRequest, api.InvalidSystemSettings)

	custom := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/system/settings",
		[]byte(`{"externalOrigin":"HTTPS://PUBLIC.EXAMPLE/"}`),
		"http://example.test", cookie, session.CsrfToken,
	)
	if custom.Code != http.StatusOK {
		t.Fatalf("custom settings status = %d, body = %s", custom.Code, custom.Body.String())
	}
	if err := json.NewDecoder(custom.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Automatic || settings.ExternalOrigin != "https://public.example" || settings.EffectiveOrigin != "https://public.example" {
		t.Fatalf("custom settings = %#v", settings)
	}

	enrollment := performRequest(handler, http.MethodGet, "/api/v1/agent-enrollment", nil, "", cookie)
	if enrollment.Code != http.StatusOK || !strings.Contains(enrollment.Body.String(), `"registrationKey"`) ||
		strings.Contains(enrollment.Body.String(), "https://public.example") ||
		strings.Contains(enrollment.Body.String(), "install-agent.sh") {
		t.Fatalf("custom enrollment response = %d, %s", enrollment.Code, enrollment.Body.String())
	}

	wrongRequestOrigin := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/system/settings",
		[]byte(`{"externalOrigin":""}`),
		"https://public.example", cookie, session.CsrfToken,
	)
	assertErrorCode(t, wrongRequestOrigin, http.StatusForbidden, api.OriginNotAllowed)

	reset := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/system/settings",
		[]byte(`{"externalOrigin":""}`),
		"http://example.test", cookie, session.CsrfToken,
	)
	if reset.Code != http.StatusOK {
		t.Fatalf("automatic reset status = %d, body = %s", reset.Code, reset.Body.String())
	}
}

func TestNotificationAPIConfigurationRulesAndDeliveryHistory(t *testing.T) {
	received := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("X-Test-Token") != "local-secret" {
			t.Errorf("unexpected notification request method or retained header")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		received <- string(body)
		response.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(receiver.Close)

	handler, _, notificationService, _ := newTestHTTPHandlerWithNotifications(t, nil)
	workerContext, stopWorkers := context.WithCancel(context.Background())
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		notificationService.Run(workerContext, log.New(io.Discard, "", 0))
	}()
	t.Cleanup(func() {
		stopWorkers()
		<-workersDone
	})

	unauthenticated := performRequest(handler, http.MethodGet, "/api/v1/notification-senders", nil, "", nil)
	assertErrorCode(t, unauthenticated, http.StatusUnauthorized, api.Unauthenticated)
	cookie, session := loginTestAdministrator(t, handler)
	createSenderBody, err := json.Marshal(map[string]any{
		"name": "local webhook", "kind": "webhook", "enabled": true,
		"webhook": map[string]any{
			"url": receiver.URL, "headers": map[string]string{"X-Test-Token": "local-secret"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	missingCSRF := performRequest(
		handler, http.MethodPost, "/api/v1/notification-senders", createSenderBody,
		"http://example.test", cookie,
	)
	assertErrorCode(t, missingCSRF, http.StatusForbidden, api.CsrfFailed)
	createdSender := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/notification-senders", createSenderBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if createdSender.Code != http.StatusCreated {
		t.Fatalf("create notification sender status = %d, body = %s", createdSender.Code, createdSender.Body.String())
	}
	var sender api.NotificationSender
	if err := json.NewDecoder(createdSender.Body).Decode(&sender); err != nil {
		t.Fatal(err)
	}
	if sender.Webhook == nil || len(sender.Webhook.HeaderNames) != 1 || sender.Webhook.HeaderNames[0] != "X-Test-Token" ||
		strings.Contains(createdSender.Body.String(), "local-secret") {
		t.Fatalf("sender response disclosed or lost hidden headers: %#v", sender)
	}

	updateSenderBody, err := json.Marshal(map[string]any{
		"name": "renamed webhook", "enabled": true,
		"webhook": map[string]any{"url": receiver.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	senderPath := "/api/v1/notification-senders/" + sender.Id.String()
	updatedSender := performRequestWithCSRF(
		handler, http.MethodPut, senderPath, updateSenderBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if updatedSender.Code != http.StatusOK {
		t.Fatalf("update notification sender status = %d, body = %s", updatedSender.Code, updatedSender.Body.String())
	}

	createRuleBody, err := json.Marshal(map[string]any{
		"name": "address changes", "enabled": true, "senderId": sender.Id,
		"eventType": "address-change",
	})
	if err != nil {
		t.Fatal(err)
	}
	createdRule := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/notification-rules", createRuleBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if createdRule.Code != http.StatusCreated {
		t.Fatalf("create notification rule status = %d, body = %s", createdRule.Code, createdRule.Body.String())
	}
	var rule api.NotificationRule
	if err := json.NewDecoder(createdRule.Body).Decode(&rule); err != nil {
		t.Fatal(err)
	}
	updateRuleBody, err := json.Marshal(map[string]any{
		"name": "all address changes", "enabled": true, "senderId": sender.Id,
		"eventType": "address-change",
	})
	if err != nil {
		t.Fatal(err)
	}
	rulePath := "/api/v1/notification-rules/" + rule.Id.String()
	updatedRule := performRequestWithCSRF(
		handler, http.MethodPut, rulePath, updateRuleBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if updatedRule.Code != http.StatusOK {
		t.Fatalf("update notification rule status = %d, body = %s", updatedRule.Code, updatedRule.Body.String())
	}
	rules := performRequest(handler, http.MethodGet, "/api/v1/notification-rules", nil, "", cookie)
	if rules.Code != http.StatusOK {
		t.Fatalf("list notification rules status = %d, body = %s", rules.Code, rules.Body.String())
	}
	var ruleList api.NotificationRuleList
	if err := json.NewDecoder(rules.Body).Decode(&ruleList); err != nil || len(ruleList.Items) != 1 || ruleList.Items[0].Name != "all address changes" {
		t.Fatalf("notification rule list = %#v, %v", ruleList, err)
	}

	testDelivery := performRequestWithCSRF(
		handler, http.MethodPost, senderPath+"/test-deliveries", nil,
		"http://example.test", cookie, session.CsrfToken,
	)
	if testDelivery.Code != http.StatusAccepted {
		t.Fatalf("create test delivery status = %d, body = %s", testDelivery.Code, testDelivery.Body.String())
	}
	var delivery api.NotificationDelivery
	if err := json.NewDecoder(testDelivery.Body).Decode(&delivery); err != nil || !delivery.Test {
		t.Fatalf("queued notification delivery = %#v, %v", delivery, err)
	}
	switch delivery.Status {
	case "pending", "running", "succeeded":
	default:
		t.Fatalf("initial notification delivery state = %#v", delivery)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, `"type":"test"`) {
			t.Fatalf("test delivery body = %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("local notification test delivery was not received")
	}

	query := url.Values{"senderId": []string{sender.Id.String()}, "status": []string{"succeeded"}}
	var deliveryPage api.NotificationDeliveryPage
	deadline := time.Now().Add(2 * time.Second)
	for {
		deliveries := performRequest(
			handler, http.MethodGet, "/api/v1/notification-deliveries?"+query.Encode(), nil, "", cookie,
		)
		if deliveries.Code != http.StatusOK {
			t.Fatalf("list notification deliveries status = %d, body = %s", deliveries.Code, deliveries.Body.String())
		}
		if err := json.NewDecoder(deliveries.Body).Decode(&deliveryPage); err != nil {
			t.Fatal(err)
		}
		if len(deliveryPage.Items) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("succeeded notification delivery page = %#v", deliveryPage)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if deliveryPage.Items[0].Id != delivery.Id || deliveryPage.Items[0].AttemptCount != 1 {
		t.Fatalf("completed notification delivery = %#v", deliveryPage.Items[0])
	}

	deletedRule := performRequestWithCSRF(
		handler, http.MethodDelete, rulePath, nil,
		"http://example.test", cookie, session.CsrfToken,
	)
	if deletedRule.Code != http.StatusNoContent {
		t.Fatalf("delete notification rule status = %d, body = %s", deletedRule.Code, deletedRule.Body.String())
	}
	deletedSender := performRequestWithCSRF(
		handler, http.MethodDelete, senderPath, nil,
		"http://example.test", cookie, session.CsrfToken,
	)
	if deletedSender.Code != http.StatusNoContent {
		t.Fatalf("delete notification sender status = %d, body = %s", deletedSender.Code, deletedSender.Body.String())
	}
}

func TestHistoryAPIAuthorizationMutationAndRetention(t *testing.T) {
	handler, nodeService, _ := newTestHTTPHandlerWithNodes(t, nil)
	fixture := seedHTTPHistory(t, nodeService)

	unauthenticated := performRequest(handler, http.MethodGet, "/api/v1/history/probe-snapshots", nil, "", nil)
	assertErrorCode(t, unauthenticated, http.StatusUnauthorized, api.Unauthenticated)
	cookie, session := loginTestAdministrator(t, handler)

	state := performRequest(handler, http.MethodGet, "/api/v1/history", nil, "", cookie)
	if state.Code != http.StatusOK {
		t.Fatalf("history state status = %d, body = %s", state.Code, state.Body.String())
	}

	retentionBody := []byte(`{"mode":"age","maxAgeDays":1}`)
	missingRetentionCSRF := performRequest(
		handler, http.MethodPut, "/api/v1/history/retention", retentionBody,
		"http://example.test", cookie,
	)
	assertErrorCode(t, missingRetentionCSRF, http.StatusForbidden, api.CsrfFailed)
	invalidRetention := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/history/retention",
		[]byte(`{"mode":"age","maxAgeDays":0}`), "http://example.test", cookie, session.CsrfToken,
	)
	assertErrorCode(t, invalidRetention, http.StatusBadRequest, api.InvalidRequest)
	retention := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/history/retention", retentionBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if retention.Code != http.StatusOK {
		t.Fatalf("retention update status = %d, body = %s", retention.Code, retention.Body.String())
	}

	starPath := "/api/v1/probe-snapshots/" + fixture.latestSnapshot.String() + "/star"
	missingStarCSRF := performRequest(handler, http.MethodPut, starPath, nil, "http://example.test", cookie)
	assertErrorCode(t, missingStarCSRF, http.StatusForbidden, api.CsrfFailed)
	for attempt := 0; attempt < 2; attempt++ {
		starred := performRequestWithCSRF(
			handler, http.MethodPut, starPath, nil, "http://example.test", cookie, session.CsrfToken,
		)
		if starred.Code != http.StatusOK {
			t.Fatalf("star attempt %d status = %d, body = %s", attempt, starred.Code, starred.Body.String())
		}
		var snapshot api.ProbeSnapshot
		if err := json.NewDecoder(starred.Body).Decode(&snapshot); err != nil {
			t.Fatal(err)
		}
		if !snapshot.Starred {
			t.Fatalf("star attempt %d returned unstarred snapshot", attempt)
		}
	}
	unstarred := performRequestWithCSRF(
		handler, http.MethodDelete, starPath, nil, "http://example.test", cookie, session.CsrfToken,
	)
	if unstarred.Code != http.StatusOK {
		t.Fatalf("unstar status = %d, body = %s", unstarred.Code, unstarred.Body.String())
	}
	var snapshot api.ProbeSnapshot
	if err := json.NewDecoder(unstarred.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Starred {
		t.Fatal("unstar returned a starred snapshot")
	}

	missingCleanupCSRF := performRequest(
		handler, http.MethodPost, "/api/v1/history/cleanup", nil,
		"http://example.test", cookie,
	)
	assertErrorCode(t, missingCleanupCSRF, http.StatusForbidden, api.CsrfFailed)
	cleanup := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/history/cleanup", nil,
		"http://example.test", cookie, session.CsrfToken,
	)
	if cleanup.Code != http.StatusOK {
		t.Fatalf("history cleanup status = %d, body = %s", cleanup.Code, cleanup.Body.String())
	}
}

func TestHistoryAPIFiltersOrderingAndComparison(t *testing.T) {
	handler, nodeService, _ := newTestHTTPHandlerWithNodes(t, nil)
	fixture := seedHTTPHistory(t, nodeService)
	cookie, _ := loginTestAdministrator(t, handler)

	reportQuery := url.Values{
		"nodeId":       []string{fixture.nodeID.String()},
		"egressId":     []string{fixture.otherEgress.String()},
		"from":         []string{fixture.from.Format(time.RFC3339)},
		"to":           []string{fixture.to.Format(time.RFC3339)},
		"runStatus":    []string{"succeeded"},
		"trigger":      []string{"schedule"},
		"changed":      []string{"false"},
		"formatStatus": []string{"mismatch"},
		"page":         []string{"1"},
		"pageSize":     []string{"10"},
	}
	reports := performRequest(
		handler, http.MethodGet, "/api/v1/history/probe-snapshots?"+reportQuery.Encode(), nil, "", cookie,
	)
	if reports.Code != http.StatusOK {
		t.Fatalf("filtered reports status = %d, body = %s", reports.Code, reports.Body.String())
	}
	var reportPage api.ProbeSnapshotHistoryPage
	if err := json.NewDecoder(reports.Body).Decode(&reportPage); err != nil {
		t.Fatal(err)
	}
	if reportPage.Total != 1 || len(reportPage.Items) != 1 || reportPage.Items[0].Id != fixture.otherSnapshot {
		t.Fatalf("filtered reports = %#v", reportPage)
	}

	addressQuery := url.Values{
		"nodeId":    []string{fixture.nodeID.String()},
		"egressId":  []string{fixture.primaryEgress.String()},
		"from":      []string{fixture.from.Format(time.RFC3339)},
		"to":        []string{fixture.to.Format(time.RFC3339)},
		"eventKind": []string{"first-observation"},
		"family":    []string{"ipv4"},
		"page":      []string{"1"},
		"pageSize":  []string{"10"},
	}
	addresses := performRequest(
		handler, http.MethodGet, "/api/v1/history/address-events?"+addressQuery.Encode(), nil, "", cookie,
	)
	if addresses.Code != http.StatusOK {
		t.Fatalf("filtered addresses status = %d, body = %s", addresses.Code, addresses.Body.String())
	}
	var addressPage api.AddressHistoryPage
	if err := json.NewDecoder(addresses.Body).Decode(&addressPage); err != nil {
		t.Fatal(err)
	}
	if addressPage.Total != 1 || len(addressPage.Events) != 1 || addressPage.Events[0].Event.Id != fixture.addressEvent {
		t.Fatalf("filtered addresses = %#v", addressPage)
	}

	addressQuery.Set("page", "2")
	addressQuery.Set("gapPage", "2")
	addressQuery.Set("pageSize", "1")
	addressQuery.Del("egressId")
	addresses = performRequest(
		handler, http.MethodGet, "/api/v1/history/address-events?"+addressQuery.Encode(), nil, "", cookie,
	)
	if addresses.Code != http.StatusOK {
		t.Fatalf("independently paged addresses status = %d, body = %s", addresses.Code, addresses.Body.String())
	}
	if err := json.NewDecoder(addresses.Body).Decode(&addressPage); err != nil {
		t.Fatal(err)
	}
	if addressPage.Total != 1 || len(addressPage.Events) != 0 || addressPage.GapTotal != 2 || len(addressPage.Gaps) != 1 {
		t.Fatalf("independently paged addresses = %#v", addressPage)
	}

	for path, target := range map[string]any{
		"/api/v1/history/probe-gaps?page=2&pageSize=1":    &api.ProbeHistoryGapPage{},
		"/api/v1/history/format-events?page=2&pageSize=1": &api.ProbeFormatEventPage{},
	} {
		response := performRequest(handler, http.MethodGet, path, nil, "", cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("paged history %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
		switch page := target.(type) {
		case *api.ProbeHistoryGapPage:
			if page.Total != 2 || len(page.Items) != 1 {
				t.Fatalf("probe gap page = %#v", page)
			}
		case *api.ProbeFormatEventPage:
			if page.Total != 2 || len(page.Items) != 1 {
				t.Fatalf("format event page = %#v", page)
			}
		}
	}

	latest := performRequest(
		handler, http.MethodGet, "/api/v1/probe-snapshots/"+fixture.latestSnapshot.String(), nil, "", cookie,
	)
	if latest.Code != http.StatusOK {
		t.Fatalf("latest snapshot status = %d, body = %s", latest.Code, latest.Body.String())
	}
	var latestSnapshot api.ProbeSnapshot
	if err := json.NewDecoder(latest.Body).Decode(&latestSnapshot); err != nil {
		t.Fatal(err)
	}
	if latestSnapshot.PreviousSnapshotId == nil || *latestSnapshot.PreviousSnapshotId != fixture.firstSnapshot {
		t.Fatalf("latest previous snapshot = %#v", latestSnapshot.PreviousSnapshotId)
	}

	sameEgressQuery := url.Values{
		"beforeSnapshotId": []string{fixture.firstSnapshot.String()},
		"afterSnapshotId":  []string{fixture.latestSnapshot.String()},
	}
	comparison := performRequest(
		handler, http.MethodGet, "/api/v1/history/comparison?"+sameEgressQuery.Encode(), nil, "", cookie,
	)
	if comparison.Code != http.StatusOK {
		t.Fatalf("comparison status = %d, body = %s", comparison.Code, comparison.Body.String())
	}
	var compared api.ProbeSnapshotComparison
	if err := json.NewDecoder(comparison.Body).Decode(&compared); err != nil {
		t.Fatal(err)
	}
	if compared.BeforeId != fixture.firstSnapshot || compared.AfterId != fixture.latestSnapshot || compared.EgressId != fixture.primaryEgress {
		t.Fatalf("comparison = %#v", compared)
	}

	differentEgressQuery := url.Values{
		"beforeSnapshotId": []string{fixture.firstSnapshot.String()},
		"afterSnapshotId":  []string{fixture.otherSnapshot.String()},
	}
	conflict := performRequest(
		handler, http.MethodGet, "/api/v1/history/comparison?"+differentEgressQuery.Encode(), nil, "", cookie,
	)
	assertErrorCode(t, conflict, http.StatusConflict, api.SnapshotEgressMismatch)
}

func TestNetworkProxyAPINeverRevealsStoredPassword(t *testing.T) {
	handler, nodeService, _ := newTestHTTPHandlerWithNodes(t, nil)
	login := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "http://example.test", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var session api.AuthenticatedSession
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0]
	ctx := context.Background()
	enrollment, err := nodeService.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := nodeService.Register(ctx, enrollment.Key, nodes.Metadata{
		Hostname: "proxy-api.example", AgentVersion: "0.1.0", OperatingSystem: "linux",
		Architecture: "amd64", Capabilities: []string{"control-v1"}, PhysicalMemoryBytes: 512 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	secret := "proxy-password-must-not-be-returned"
	body, err := json.Marshal(api.NetworkProxyCreate{
		Name: "Primary proxy", Scheme: api.NetworkProxySchemeSocks5,
		Host: "proxy.example.test", Port: 1080, Password: &secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	created := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/nodes/"+registration.NodeID.String()+"/network-proxies", body,
		"http://example.test", cookie, session.CsrfToken,
	)
	if created.Code != http.StatusCreated || bytes.Contains(created.Body.Bytes(), []byte(secret)) || bytes.Contains(created.Body.Bytes(), []byte(`"password"`)) {
		t.Fatalf("create proxy response = %d %s", created.Code, created.Body.String())
	}
	var proxy api.NetworkProxy
	if err := json.NewDecoder(created.Body).Decode(&proxy); err != nil {
		t.Fatal(err)
	}
	if !proxy.PasswordConfigured {
		t.Fatalf("create proxy response did not report configured password: %#v", proxy)
	}
	listed := performRequest(handler, http.MethodGet, "/api/v1/nodes/"+registration.NodeID.String()+"/network-proxies", nil, "", cookie)
	if listed.Code != http.StatusOK || bytes.Contains(listed.Body.Bytes(), []byte(secret)) || bytes.Contains(listed.Body.Bytes(), []byte(`"password"`)) {
		t.Fatalf("list proxy response = %d %s", listed.Code, listed.Body.String())
	}
}

func TestNetworkObservationAndPublicAddressAPIWorkflow(t *testing.T) {
	handler, nodeService, _ := newTestHTTPHandlerWithNodes(t, nil)
	login := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "http://example.test", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var session api.AuthenticatedSession
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0]

	settings := performRequest(handler, http.MethodGet, "/api/v1/network-observation-settings", nil, "", cookie)
	if settings.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body = %s", settings.Code, settings.Body.String())
	}
	invalidSettings := []byte(`{"ipv4Services":["https://one.example/ip","https://one.example/other"],"ipv6Services":["https://six-one.example/ip","https://six-two.example/ip"]}`)
	invalid := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/network-observation-settings", invalidSettings,
		"http://example.test", cookie, session.CsrfToken,
	)
	assertErrorCode(t, invalid, http.StatusBadRequest, api.InvalidObservationSettings)
	validSettings := []byte(`{"ipv4Services":["https://one.example/ip","https://two.example/ip"],"ipv6Services":["https://six-one.example/ip","https://six-two.example/ip"]}`)
	updated := performRequestWithCSRF(
		handler, http.MethodPut, "/api/v1/network-observation-settings", validSettings,
		"http://example.test", cookie, session.CsrfToken,
	)
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte("https://two.example/ip")) {
		t.Fatalf("settings update = %d %s", updated.Code, updated.Body.String())
	}

	ctx := context.Background()
	enrollment, err := nodeService.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := nodeService.Register(ctx, enrollment.Key, nodes.Metadata{
		Hostname: "api-edge.example", AgentVersion: "0.1.0", OperatingSystem: "linux",
		Architecture: "amd64", Capabilities: []string{"control-v1"}, PhysicalMemoryBytes: 512 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := "10.0.0.1"
	inventory := nodes.NetworkInventory{
		CapturedAt: time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC),
		Interfaces: []nodes.NetworkInterface{{Name: "eth0", Index: 2, Up: true}},
		Addresses: []nodes.NetworkAddress{{
			InterfaceName: "eth0", Address: "10.0.0.5", PrefixLength: 24, Family: "ipv4", Scope: "private",
		}},
		Routes: []nodes.NetworkRoute{{
			InterfaceName: "eth0", Family: "ipv4", Destination: "0.0.0.0/0", Gateway: &gateway, Metric: 100, Default: true,
		}},
	}
	if _, err := nodeService.Poll(ctx, registration.Credential, nodes.Metadata{
		Hostname: "api-edge.example", AgentVersion: "0.1.0", OperatingSystem: "linux",
		Architecture: "amd64", Capabilities: []string{"control-v1"}, PhysicalMemoryBytes: 512 * 1024 * 1024,
	}, 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	networkResponse := performRequest(
		handler, http.MethodGet, "/api/v1/nodes/"+registration.NodeID.String()+"/network", nil, "", cookie,
	)
	if networkResponse.Code != http.StatusOK {
		t.Fatalf("network status = %d, body = %s", networkResponse.Code, networkResponse.Body.String())
	}
	var network api.NodeNetworkState
	if err := json.NewDecoder(networkResponse.Body).Decode(&network); err != nil {
		t.Fatal(err)
	}
	if len(network.PublicAddresses) != 0 || len(network.NetworkProxies) != 0 {
		t.Fatalf("initial public network state = %#v", network)
	}
	if bytes.Contains(networkResponse.Body.Bytes(), []byte(`"inventory"`)) ||
		bytes.Contains(networkResponse.Body.Bytes(), []byte(`"egresses"`)) ||
		bytes.Contains(networkResponse.Body.Bytes(), []byte(`"candidates"`)) ||
		bytes.Contains(networkResponse.Body.Bytes(), []byte(`"addressStates"`)) {
		t.Fatalf("network response exposed discovery internals: %s", networkResponse.Body.String())
	}

	createBody, err := json.Marshal(api.NetworkProxyCreate{
		Name: "API proxy", Scheme: api.NetworkProxySchemeSocks5, Host: "proxy.example.test", Port: 1080,
	})
	if err != nil {
		t.Fatal(err)
	}
	created := performRequestWithCSRF(
		handler, http.MethodPost,
		"/api/v1/nodes/"+registration.NodeID.String()+"/network-proxies", createBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("node proxy creation = %d %s", created.Code, created.Body.String())
	}
	var proxy api.NetworkProxy
	if err := json.NewDecoder(created.Body).Decode(&proxy); err != nil {
		t.Fatal(err)
	}
	if proxy.NodeId != registration.NodeID || !proxy.Enabled || proxy.Status != api.NetworkProxyStatusChecking ||
		proxy.Ipv4.Status != api.NetworkProxyFamilyStatusChecking || proxy.Ipv6.Status != api.NetworkProxyFamilyStatusChecking {
		t.Fatalf("created node proxy = %#v", proxy)
	}
	listed := performRequest(
		handler, http.MethodGet, "/api/v1/nodes/"+registration.NodeID.String()+"/network-proxies", nil, "", cookie,
	)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"API proxy"`)) {
		t.Fatalf("node proxy list = %d %s", listed.Code, listed.Body.String())
	}
	updateBody, err := json.Marshal(api.NetworkProxyUpdate{
		Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
		Enabled: false, PasswordAction: api.Keep,
	})
	if err != nil {
		t.Fatal(err)
	}
	updateResponse := performRequestWithCSRF(
		handler, http.MethodPut,
		"/api/v1/nodes/"+registration.NodeID.String()+"/network-proxies/"+proxy.Id.String(), updateBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("node proxy update = %d %s", updateResponse.Code, updateResponse.Body.String())
	}
	if err := json.NewDecoder(updateResponse.Body).Decode(&proxy); err != nil {
		t.Fatal(err)
	}
	if proxy.Enabled || proxy.Status != api.NetworkProxyStatusDisabled {
		t.Fatalf("disabled node proxy = %#v", proxy)
	}
	queued := performRequestWithCSRF(
		handler, http.MethodDelete,
		"/api/v1/nodes/"+registration.NodeID.String()+"/network-proxies/"+proxy.Id.String(), nil,
		"http://example.test", cookie, session.CsrfToken,
	)
	if queued.Code != http.StatusAccepted {
		t.Fatalf("node proxy deletion = %d %s", queued.Code, queued.Body.String())
	}
	var deletion api.NetworkProxy
	if err := json.NewDecoder(queued.Body).Decode(&deletion); err != nil {
		t.Fatal(err)
	}
	if deletion.Id != proxy.Id || deletion.DeletionStatus == nil || *deletion.DeletionStatus != api.NetworkProxyDeletionStatusPending {
		t.Fatalf("node proxy deletion response = %#v", deletion)
	}
	pendingResponse := performRequest(
		handler, http.MethodGet, "/api/v1/nodes/"+registration.NodeID.String()+"/network", nil, "", cookie,
	)
	if pendingResponse.Code != http.StatusOK {
		t.Fatalf("pending network status = %d, body = %s", pendingResponse.Code, pendingResponse.Body.String())
	}
	if err := json.NewDecoder(pendingResponse.Body).Decode(&network); err != nil {
		t.Fatal(err)
	}
	if len(network.NetworkProxies) != 1 || network.NetworkProxies[0].DeletionStatus == nil ||
		*network.NetworkProxies[0].DeletionStatus != api.NetworkProxyDeletionStatusPending {
		t.Fatalf("pending node proxy = %#v", network.NetworkProxies)
	}
}

func TestAgentControlRequestBodyHasBoundedInventoryHeadroom(t *testing.T) {
	handler := limitAPIRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	withinLimit := performRequest(
		handler, http.MethodPost, "/api/v1/agent/control",
		bytes.Repeat([]byte("x"), 96*1024), "", nil,
	)
	if withinLimit.Code != http.StatusNoContent {
		t.Fatalf("96 KiB control request status = %d, want %d", withinLimit.Code, http.StatusNoContent)
	}
	tooLarge := performRequest(
		handler, http.MethodPost, "/api/v1/agent/control",
		bytes.Repeat([]byte("x"), maxAgentControlRequestBodySize+1), "", nil,
	)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized control request status = %d, want %d", tooLarge.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestAdministratorEnrollmentAndAgentCredentialBoundaries(t *testing.T) {
	handler, nodeService, _ := newTestHTTPHandlerWithNodes(t, nil)
	unauthenticated := performRequest(handler, http.MethodGet, "/api/v1/agent-enrollment", nil, "", nil)
	assertErrorCode(t, unauthenticated, http.StatusUnauthorized, api.Unauthenticated)

	login := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "http://example.test", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var session api.AuthenticatedSession
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0]
	initial := performRequest(handler, http.MethodGet, "/api/v1/agent-enrollment", nil, "", cookie)
	if initial.Code != http.StatusOK || !bytes.Contains(initial.Body.Bytes(), []byte(`"hasKey":false`)) {
		t.Fatalf("initial enrollment response = %d %s", initial.Code, initial.Body.String())
	}
	missingTimezone := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/agent-enrollment/key", nil,
		"http://example.test", cookie, session.CsrfToken,
	)
	assertErrorCode(t, missingTimezone, http.StatusBadRequest, api.InvalidRequest)
	invalidRotationBody, err := json.Marshal(api.AgentEnrollmentKeyRotation{DefaultProbeTimezone: "agent-local"})
	if err != nil {
		t.Fatal(err)
	}
	invalidTimezone := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/agent-enrollment/key", invalidRotationBody,
		"http://example.test", cookie, session.CsrfToken,
	)
	assertErrorCode(t, invalidTimezone, http.StatusBadRequest, api.InvalidRequest)
	rotationBody, err := json.Marshal(api.AgentEnrollmentKeyRotation{DefaultProbeTimezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	rotated := performRequestWithCSRF(handler, http.MethodPost, "/api/v1/agent-enrollment/key", rotationBody, "http://example.test", cookie, session.CsrfToken)
	if rotated.Code != http.StatusOK || !bytes.Contains(rotated.Body.Bytes(), []byte(`"registrationKey"`)) ||
		!bytes.Contains(rotated.Body.Bytes(), []byte(`"defaultProbeTimezone":"Asia/Shanghai"`)) ||
		bytes.Contains(rotated.Body.Bytes(), []byte("install-agent.sh")) ||
		bytes.Contains(rotated.Body.Bytes(), []byte("--version")) {
		t.Fatalf("rotated enrollment response = %d %s", rotated.Code, rotated.Body.String())
	}
	enrollment, err := nodeService.Enrollment(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	registrationBody, err := json.Marshal(api.AgentRegistrationRequest{
		RegistrationKey: enrollment.Key,
		Metadata: api.AgentMetadata{
			Hostname: "edge-1", AgentVersion: "0.1.0", OperatingSystem: api.Linux,
			Architecture: api.Amd64, Capabilities: []string{"control-v1"}, PhysicalMemoryBytes: 512 * 1024 * 1024,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	registration := performRequest(handler, http.MethodPost, "/api/v1/agent/enroll", registrationBody, "", nil)
	if registration.Code != http.StatusCreated {
		t.Fatalf("registration status = %d, body = %s", registration.Code, registration.Body.String())
	}
	var registered api.AgentRegistrationResult
	if err := json.NewDecoder(registration.Body).Decode(&registered); err != nil {
		t.Fatal(err)
	}
	pollBody, err := json.Marshal(api.AgentPollRequest{
		AppliedConfigurationRevision: 0,
		Metadata: api.AgentMetadata{
			Hostname: "edge-1", AgentVersion: "0.1.0", OperatingSystem: api.Linux,
			Architecture: api.Amd64, Capabilities: []string{"control-v1"}, PhysicalMemoryBytes: 512 * 1024 * 1024,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	missingCredential := performRequest(handler, http.MethodPost, "/api/v1/agent/control", pollBody, "", nil)
	assertErrorCode(t, missingCredential, http.StatusUnauthorized, api.AgentUnauthenticated)
	pollRequest := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/agent/control", bytes.NewReader(pollBody))
	pollRequest.RemoteAddr = "192.0.2.10:1234"
	pollRequest.Header.Set("Content-Type", "application/json")
	pollRequest.Header.Set("Authorization", "Bearer "+registered.Credential)
	poll := httptest.NewRecorder()
	handler.ServeHTTP(poll, pollRequest)
	if poll.Code != http.StatusOK {
		t.Fatalf("Agent poll status = %d, body = %s", poll.Code, poll.Body.String())
	}

	list := performRequest(handler, http.MethodGet, "/api/v1/nodes", nil, "", cookie)
	if list.Code != http.StatusOK {
		t.Fatalf("node list status = %d, body = %s", list.Code, list.Body.String())
	}
	var nodes api.NodeList
	if err := json.NewDecoder(list.Body).Decode(&nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes.Items) != 1 || nodes.Items[0].Status != api.NodeStatusOnline {
		t.Fatalf("unexpected node list: %#v", nodes.Items)
	}

	disableBody := []byte(`{"enabled":false}`)
	disabled := performRequestWithCSRF(handler, http.MethodPut, "/api/v1/agent-enrollment", disableBody, "http://example.test", cookie, session.CsrfToken)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable enrollment status = %d, body = %s", disabled.Code, disabled.Body.String())
	}
	rejected := performRequest(handler, http.MethodPost, "/api/v1/agent/enroll", registrationBody, "", nil)
	assertErrorCode(t, rejected, http.StatusForbidden, api.RegistrationDisabled)
	pollRequest = httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/agent/control", bytes.NewReader(pollBody))
	pollRequest.RemoteAddr = "192.0.2.10:1234"
	pollRequest.Header.Set("Content-Type", "application/json")
	pollRequest.Header.Set("Authorization", "Bearer "+registered.Credential)
	pollAfterDisable := httptest.NewRecorder()
	handler.ServeHTTP(pollAfterDisable, pollRequest)
	if pollAfterDisable.Code != http.StatusOK {
		t.Fatalf("existing Agent rejected after disabling enrollment: %d %s", pollAfterDisable.Code, pollAfterDisable.Body.String())
	}

	nodeUpdate := performRequestWithCSRF(
		handler, http.MethodPatch, "/api/v1/nodes/"+registered.NodeId.String(),
		[]byte(`{"enabled":false}`), "http://example.test", cookie, session.CsrfToken,
	)
	if nodeUpdate.Code != http.StatusOK {
		t.Fatalf("disable node status = %d, body = %s", nodeUpdate.Code, nodeUpdate.Body.String())
	}
	configurationRequest := httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/agent/configuration", nil)
	configurationRequest.RemoteAddr = "192.0.2.10:1234"
	configurationRequest.Header.Set("Authorization", "Bearer "+registered.Credential)
	configurationResponse := httptest.NewRecorder()
	handler.ServeHTTP(configurationResponse, configurationRequest)
	if configurationResponse.Code != http.StatusOK {
		t.Fatalf("Agent configuration status = %d, body = %s", configurationResponse.Code, configurationResponse.Body.String())
	}
	var configuration api.AgentConfigurationSnapshot
	if err := json.NewDecoder(configurationResponse.Body).Decode(&configuration); err != nil {
		t.Fatal(err)
	}
	if configuration.Revision != 2 || configuration.Enabled || len(configuration.HistoryGeneration) != 64 {
		t.Fatalf("unexpected Agent configuration: %#v", configuration)
	}

	revoked := performRequestWithCSRF(
		handler, http.MethodPost, "/api/v1/nodes/"+registered.NodeId.String()+"/revoke",
		nil, "http://example.test", cookie, session.CsrfToken,
	)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke node status = %d, body = %s", revoked.Code, revoked.Body.String())
	}
	pollRequest = httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/agent/control", bytes.NewReader(pollBody))
	pollRequest.RemoteAddr = "192.0.2.10:1234"
	pollRequest.Header.Set("Content-Type", "application/json")
	pollRequest.Header.Set("Authorization", "Bearer "+registered.Credential)
	pollAfterRevoke := httptest.NewRecorder()
	handler.ServeHTTP(pollAfterRevoke, pollRequest)
	assertErrorCode(t, pollAfterRevoke, http.StatusForbidden, api.AgentRevoked)
}

func TestProbeSchedulePreviewUsesSharedScheduleSemantics(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	query := url.Values{
		"cron":     {"0 0 0 * * *"},
		"timezone": {"Asia/Shanghai"},
	}
	path := "/api/v1/probe-schedules/preview?" + query.Encode()
	unauthenticated := performRequest(handler, http.MethodGet, path, nil, "", nil)
	assertErrorCode(t, unauthenticated, http.StatusUnauthorized, api.Unauthenticated)

	cookie, _ := loginTestAdministrator(t, handler)
	before := time.Now().UTC()
	response := performRequest(handler, http.MethodGet, path, nil, "", cookie)
	after := time.Now().UTC()
	if response.Code != http.StatusOK {
		t.Fatalf("schedule preview = %d %s", response.Code, response.Body.String())
	}
	var preview api.ProbeSchedulePreview
	if err := json.NewDecoder(response.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	local := preview.NextScheduledAt.In(location)
	if local.Hour() != 0 || local.Minute() != 0 || local.Second() != 0 ||
		!preview.NextScheduledAt.After(before) || preview.NextScheduledAt.After(after.Add(24*time.Hour)) {
		t.Fatalf("unexpected schedule preview: %s", preview.NextScheduledAt)
	}

	invalidQuery := url.Values{"cron": {"0 0 0 * *"}, "timezone": {"Asia/Shanghai"}}
	invalid := performRequest(
		handler, http.MethodGet,
		"/api/v1/probe-schedules/preview?"+invalidQuery.Encode(), nil, "", cookie,
	)
	assertErrorCode(t, invalid, http.StatusBadRequest, api.InvalidProbeSettings)
}

func TestTemporarySyncWebSocketAuthenticationWakeAndStop(t *testing.T) {
	ctx := context.Background()
	handler, nodeService, syncHub := newTestHTTPHandlerWithNodes(t, nil)
	enrollment, err := nodeService.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	metadata := nodes.Metadata{
		Hostname: "sync-edge", AgentVersion: "0.1.0", OperatingSystem: "linux", Architecture: "amd64",
		Capabilities: []string{"control-v1", nodes.SyncWakeCapability}, PhysicalMemoryBytes: 512 * 1024 * 1024,
	}
	registration, err := nodeService.Register(ctx, enrollment.Key, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nodeService.StartSyncSession(ctx, registration.NodeID); err != nil {
		t.Fatal(err)
	}
	poll, err := nodeService.Poll(ctx, registration.Credential, metadata, 0, nil, nil, nil, nil)
	if err != nil || poll.SyncSession == nil {
		t.Fatalf("sync poll = %#v, %v", poll, err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/sync/" + poll.SyncSession.ID.String()
	_, response, err := websocket.Dial(ctx, websocketURL, nil)
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated WebSocket = HTTP %#v, %v", responseStatus(response), err)
	}

	connection, response, err := websocket.Dial(ctx, websocketURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + registration.Credential}},
	})
	if err != nil {
		t.Fatalf("authenticated WebSocket = HTTP %#v, %v", responseStatus(response), err)
	}
	t.Cleanup(func() { connection.CloseNow() })
	readContext, cancel := context.WithTimeout(ctx, time.Second)
	messageType, message, err := connection.Read(readContext)
	cancel()
	if err != nil || messageType != websocket.MessageText || string(message) != `{"type":"wake"}` {
		t.Fatalf("initial WebSocket wake = %s %q, %v", messageType, message, err)
	}
	deadline := time.Now().Add(time.Second)
	for !syncHub.Connected(registration.NodeID.String(), poll.SyncSession.ID.String()) {
		if time.Now().After(deadline) {
			t.Fatal("authenticated WebSocket was not registered")
		}
		time.Sleep(time.Millisecond)
	}

	if _, err := nodeService.SetEnabled(ctx, registration.NodeID, false); err != nil {
		t.Fatal(err)
	}
	readContext, cancel = context.WithTimeout(ctx, time.Second)
	messageType, message, err = connection.Read(readContext)
	cancel()
	if err != nil || messageType != websocket.MessageText || string(message) != `{"type":"wake"}` {
		t.Fatalf("configuration WebSocket wake = %s %q, %v", messageType, message, err)
	}
	connectionClosed := make(chan error, 1)
	go func() {
		_, _, err := connection.Read(context.Background())
		connectionClosed <- err
	}()
	if _, err := nodeService.StopSyncSession(ctx, registration.NodeID); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-connectionClosed:
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Fatalf("stopped WebSocket close = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stopped WebSocket remained open")
	}
}

func responseStatus(response *http.Response) any {
	if response == nil {
		return nil
	}
	return response.StatusCode
}

func TestTrustedProxyControlsForwardedHTTPS(t *testing.T) {
	prefix := netip.MustParsePrefix("10.0.0.0/8")
	handler := newTestHTTPHandler(t, []netip.Prefix{prefix})
	request := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login", bytes.NewReader(loginBody()))
	request.RemoteAddr = "10.0.0.10:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("proxied login status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("trusted HTTPS proxy did not produce a Secure cookie: %#v", cookies)
	}
}

func TestUntrustedClientCannotSupplyForwardedHTTPS(t *testing.T) {
	handler := newTestHTTPHandler(t, nil)
	request := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login", bytes.NewReader(loginBody()))
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://example.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assertErrorCode(t, response, http.StatusForbidden, api.OriginNotAllowed)
}

func newTestHTTPHandler(t *testing.T, trustedProxies []netip.Prefix) http.Handler {
	t.Helper()
	handler, _, _ := newTestHTTPHandlerWithNodes(t, trustedProxies)
	return handler
}

func newTestHTTPHandlerWithNodes(t *testing.T, trustedProxies []netip.Prefix) (http.Handler, *nodes.Service, *syncws.Hub) {
	handler, nodeService, _, syncHub := newTestHTTPHandlerWithNotifications(t, trustedProxies)
	return handler, nodeService, syncHub
}

func newTestHTTPHandlerWithNotifications(t *testing.T, trustedProxies []netip.Prefix) (http.Handler, *nodes.Service, *notifications.Service, *syncws.Hub) {
	t.Helper()
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	administrator := admin.NewService(store.Config, store.ConfigQueries, store.MasterKey)
	if err := administrator.Bootstrap(context.Background(), "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	syncHub := syncws.NewHub()
	nodeService := nodes.NewService(store.Config, store.History, store.ConfigQueries, store.MasterKey, syncHub)
	systemSettingsService := systemsettings.NewService(store.ConfigQueries)
	notificationService := notifications.NewService(notifications.ServiceOptions{
		ConfigDatabase: store.Config, HistoryDatabase: store.History,
		ConfigQueries: store.ConfigQueries, HistoryQueries: store.HistoryQueries,
		MasterKey: store.MasterKey, SystemSettings: systemSettingsService, Executable: "/proc/self/exe",
	})
	updateService := centerupdates.NewService(centerupdates.ServiceOptions{
		Queries: store.ConfigQueries, Waker: syncHub,
		CurrentVersion: "0.0.0-test", CurrentRevision: "test-revision",
	})
	return NewHTTPHandler(HTTPOptions{
		Version: "0.0.0-test", Revision: "test-revision", Web: http.NotFoundHandler(),
		Administrator: administrator, Nodes: nodeService, Notifications: notificationService, Updates: updateService, SyncHub: syncHub,
		SystemSettings: systemSettingsService, Store: store, TrustedProxies: trustedProxies,
	}), nodeService, notificationService, syncHub
}

type httpHistoryFixture struct {
	nodeID         uuid.UUID
	primaryEgress  uuid.UUID
	otherEgress    uuid.UUID
	firstSnapshot  uuid.UUID
	latestSnapshot uuid.UUID
	otherSnapshot  uuid.UUID
	addressEvent   uuid.UUID
	from           time.Time
	to             time.Time
}

func seedHTTPHistory(t *testing.T, service *nodes.Service) httpHistoryFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	metadata := nodes.Metadata{
		Hostname: "history-edge", AgentVersion: "0.1.0", OperatingSystem: "linux", Architecture: "amd64",
		Capabilities:        []string{"control-v1", "configuration-v8", "complete-probe-v1"},
		PhysicalMemoryBytes: 512 * 1024 * 1024,
	}
	enrollment, err := service.RotateEnrollmentKey(ctx, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := service.Register(ctx, enrollment.Key, metadata)
	if err != nil {
		t.Fatal(err)
	}
	gateway4 := "10.0.0.1"
	gateway6 := "fe80::1"
	inventory := nodes.NetworkInventory{
		CapturedAt: now,
		Interfaces: []nodes.NetworkInterface{{Name: "eth0", Index: 2, Up: true}},
		Addresses: []nodes.NetworkAddress{
			{InterfaceName: "eth0", Address: "10.0.0.5", PrefixLength: 24, Family: "ipv4", Scope: "private"},
			{InterfaceName: "eth0", Address: "2001:4860::5", PrefixLength: 64, Family: "ipv6", Scope: "global"},
		},
		Routes: []nodes.NetworkRoute{
			{InterfaceName: "eth0", Family: "ipv4", Destination: "0.0.0.0/0", Gateway: &gateway4, Metric: 100, Default: true},
			{InterfaceName: "eth0", Family: "ipv6", Destination: "::/0", Gateway: &gateway6, Metric: 100, Default: true},
		},
	}
	if _, err := service.Poll(ctx, registration.Credential, metadata, 0, nil, nil, &inventory, nil); err != nil {
		t.Fatal(err)
	}
	configuration, err := service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.DiscoveryPaths) < 2 {
		t.Fatalf("history fixture discovery paths = %d, want at least 2", len(configuration.DiscoveryPaths))
	}
	var primary, other nodes.NetworkEgress
	for _, egress := range configuration.DiscoveryPaths {
		switch egress.Family {
		case "ipv4":
			primary = egress
		case "ipv6":
			other = egress
		}
	}
	if primary.ID == uuid.Nil || other.ID == uuid.Nil {
		t.Fatalf("history fixture discovery path families = %#v", configuration.DiscoveryPaths)
	}
	systemState, err := service.History(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publicIPv4 := "8.8.8.8"
	publicIPv6 := "2001:4860:4860::8888"
	localInterface := "eth0"
	localIPv4 := "10.0.0.5"
	localIPv6 := "2001:4860::5"
	if _, err := service.Poll(
		ctx, registration.Credential, metadata, 0, nil, nil, nil, nil,
		nodes.AddressUpload{States: []nodes.AddressState{
			httpConfirmedAddressState(primary.ID, systemState.Generation, "ipv4", publicIPv4, localInterface, localIPv4, now),
			httpConfirmedAddressState(other.ID, systemState.Generation, "ipv6", publicIPv6, localInterface, localIPv6, now),
		}},
	); err != nil {
		t.Fatal(err)
	}
	network, err := service.Network(ctx, registration.NodeID)
	if err != nil || len(network.PublicAddresses) != 2 {
		t.Fatalf("history fixture public addresses = %#v, %v", network.PublicAddresses, err)
	}
	var primaryAddress, otherAddress nodes.PublicAddress
	for _, address := range network.PublicAddresses {
		switch address.Family {
		case "ipv4":
			primaryAddress = address
		case "ipv6":
			otherAddress = address
		}
	}
	if primaryAddress.ID == uuid.Nil || otherAddress.ID == uuid.Nil {
		t.Fatalf("history fixture public address families = %#v", network.PublicAddresses)
	}
	configuration, err = service.Configuration(ctx, registration.Credential)
	if err != nil {
		t.Fatal(err)
	}

	firstObserved := now.Add(-3 * time.Minute)
	latestObserved := now.Add(-2 * time.Minute)
	otherObserved := now.Add(-time.Minute)
	firstSnapshot := uploadHTTPProbeSnapshot(
		t, service, registration.Credential, configuration, primaryAddress.ID, 1, firstObserved,
		[]byte(`{"Head":{"IP":"198.51.100.1"}}`),
	)
	latestSnapshot := uploadHTTPProbeSnapshot(
		t, service, registration.Credential, configuration, primaryAddress.ID, 2, latestObserved,
		[]byte(`{"Head":{"IP":"198.51.100.2"}}`),
	)
	otherSnapshot := uploadHTTPProbeSnapshot(
		t, service, registration.Credential, configuration, otherAddress.ID, 1, otherObserved,
		[]byte(`{"Head":{"IP":"2001:db8::1"}}`),
	)
	publicAddress := publicIPv4
	localAddress := "10.0.0.5"
	addressEvent := uuid.New()
	if _, err := service.Poll(
		ctx, registration.Credential, metadata, 0, nil, nil, nil, nil,
		nodes.AddressUpload{Events: []nodes.AddressEvent{{
			ID: addressEvent, EgressID: primary.ID, HistoryGeneration: configuration.HistoryGeneration,
			Sequence: 1, Kind: "first-observation", Family: "ipv4", PublicAddress: &publicAddress,
			LocalInterface: &localInterface, LocalAddress: &localAddress, LikelyNAT: true, ObservedAt: now.Add(-30 * time.Second),
		}}},
	); err != nil {
		t.Fatal(err)
	}
	addressGaps := make([]nodes.AddressGap, 0, 2)
	for index := int64(0); index < 2; index++ {
		observedAt := now.Add(time.Duration(index) * time.Second)
		addressGaps = append(addressGaps, nodes.AddressGap{
			ID: uuid.New(), EgressID: primary.ID, HistoryGeneration: configuration.HistoryGeneration,
			DroppedCount: 1, FirstSequence: index + 2, LastSequence: index + 2,
			FirstObservedAt: observedAt, LastObservedAt: observedAt,
		})
	}
	if _, err := service.Poll(
		ctx, registration.Credential, metadata, 0, nil, nil, nil, nil,
		nodes.AddressUpload{Gaps: addressGaps},
	); err != nil {
		t.Fatal(err)
	}
	for index := int64(0); index < 2; index++ {
		observedAt := now.Add(time.Duration(index) * time.Second)
		gap := nodes.ProbeGapArtifact{
			ID: uuid.New(), EgressID: primaryAddress.ID, HistoryGeneration: configuration.HistoryGeneration,
			DroppedCount: 1, FirstSequence: index + 3, LastSequence: index + 3,
			FirstObservedAt: observedAt, LastObservedAt: observedAt,
		}
		if _, err := service.UploadProbeArtifact(ctx, registration.Credential, nodes.ProbeArtifact{
			ID: gap.ID, Revision: 1, Gap: &gap,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return httpHistoryFixture{
		nodeID: registration.NodeID, primaryEgress: primaryAddress.ID, otherEgress: otherAddress.ID,
		firstSnapshot: firstSnapshot, latestSnapshot: latestSnapshot, otherSnapshot: otherSnapshot,
		addressEvent: addressEvent, from: now.Add(-5 * time.Minute), to: now.Add(time.Minute),
	}
}

func httpConfirmedAddressState(
	pathID uuid.UUID,
	historyGeneration, family, publicAddress, localInterface, localAddress string,
	checkedAt time.Time,
) nodes.AddressState {
	return nodes.AddressState{
		EgressID: pathID, HistoryGeneration: historyGeneration, Family: family,
		Status: "confirmed", PublicAddress: &publicAddress,
		LocalInterface: &localInterface, LocalAddress: &localAddress,
		LikelyNAT: localAddress != publicAddress, LastCheckedAt: checkedAt,
		LastSucceededAt: &checkedAt, LastChangedAt: &checkedAt,
	}
}

func uploadHTTPProbeSnapshot(
	t *testing.T,
	service *nodes.Service,
	credential string,
	configuration nodes.Configuration,
	egressID uuid.UUID,
	sequence int64,
	observedAt time.Time,
	raw []byte,
) uuid.UUID {
	t.Helper()
	runID := uuid.New()
	executionID := uuid.New()
	runStartedAt := observedAt.Add(-2 * time.Second)
	executionStartedAt := observedAt.Add(-time.Second)
	running := nodes.ProbeRunArtifact{
		ID: runID, ConfigurationRevision: configuration.Revision, HistoryGeneration: configuration.HistoryGeneration,
		Trigger: "schedule", StartedAt: runStartedAt, Status: "running",
		Executions: []nodes.ProbeExecutionManifest{{ID: executionID, EgressID: egressID, Sequence: sequence}},
	}
	if _, err := service.UploadProbeArtifact(context.Background(), credential, nodes.ProbeArtifact{
		ID: runID, Revision: 1, Run: &running,
	}); err != nil {
		t.Fatal(err)
	}
	execution := nodes.ProbeExecutionArtifact{
		ID: executionID, EgressID: egressID, Sequence: sequence, Status: "succeeded",
		StartedAt: &executionStartedAt, CompletedAt: &observedAt, RawResult: raw,
	}
	if _, err := service.UploadProbeArtifact(context.Background(), credential, nodes.ProbeArtifact{
		ID: executionID, Revision: 1, Run: &running, Execution: &execution,
	}); err != nil {
		t.Fatal(err)
	}
	terminal := running
	completedAt := observedAt.Add(time.Second)
	terminal.CompletedAt = &completedAt
	terminal.Status = "succeeded"
	if _, err := service.UploadProbeArtifact(context.Background(), credential, nodes.ProbeArtifact{
		ID: runID, Revision: 2, Run: &terminal,
	}); err != nil {
		t.Fatal(err)
	}
	return executionID
}

func loginTestAdministrator(t *testing.T, handler http.Handler) (*http.Cookie, api.AuthenticatedSession) {
	t.Helper()
	login := performRequest(handler, http.MethodPost, "/api/v1/auth/login", loginBody(), "http://example.test", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var session api.AuthenticatedSession
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies = %#v, want exactly one", cookies)
	}
	return cookies[0], session
}

func loginBody() []byte {
	return []byte(`{"username":"admin","password":"admin"}`)
}

func performRequest(handler http.Handler, method, path string, body []byte, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(body))
	request.RemoteAddr = "192.0.2.10:1234"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func performRequestWithCSRF(handler http.Handler, method, path string, body []byte, origin string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(body))
	request.RemoteAddr = "192.0.2.10:1234"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Origin", origin)
	request.Header.Set("X-CSRF-Token", csrf)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, status int, code api.ErrorCode) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != code {
		t.Fatalf("error code = %s, want %s", body.Code, code)
	}
}
