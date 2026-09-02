package center

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
	"github.com/ipchronicle/ipchronicle/internal/probefields"
)

func (s apiServer) ListNotificationProbeFields(ctx context.Context, _ api.ListNotificationProbeFieldsRequestObject) (api.ListNotificationProbeFieldsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListNotificationProbeFields401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	items := make([]api.NotificationProbeField, 0)
	for _, definition := range probefields.Catalog() {
		if definition.Compare {
			items = append(items, api.NotificationProbeField{
				Id: definition.ID, Group: definition.Group, Path: strings.Join(definition.Path, "."),
			})
		}
	}
	return api.ListNotificationProbeFields200JSONResponse{Items: items}, nil
}

func (s apiServer) ListNotificationSenders(ctx context.Context, _ api.ListNotificationSendersRequestObject) (api.ListNotificationSendersResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListNotificationSenders401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	senders, err := s.notifications.Senders(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]api.NotificationSender, 0, len(senders))
	for _, sender := range senders {
		items = append(items, notificationSenderResponse(sender))
	}
	return api.ListNotificationSenders200JSONResponse{Items: items}, nil
}

func (s apiServer) CreateNotificationSender(ctx context.Context, request api.CreateNotificationSenderRequestObject) (api.CreateNotificationSenderResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateNotificationSender401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateNotificationSender403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.CreateNotificationSender400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	sender, err := s.notifications.CreateSender(ctx, notificationSenderCreate(*request.Body))
	switch {
	case errors.Is(err, notifications.ErrInvalidSender):
		return api.CreateNotificationSender400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNotificationSender)}, nil
	case errors.Is(err, notifications.ErrSenderNameInUse):
		return api.CreateNotificationSender409JSONResponse{ConflictJSONResponse: conflict(api.NotificationSenderNameInUse)}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateNotificationSender201JSONResponse(notificationSenderResponse(sender)), nil
}

func (s apiServer) UpdateNotificationSender(ctx context.Context, request api.UpdateNotificationSenderRequestObject) (api.UpdateNotificationSenderResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNotificationSender401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNotificationSender403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNotificationSender400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	sender, err := s.notifications.UpdateSender(ctx, request.SenderId, notificationSenderUpdate(*request.Body))
	switch {
	case errors.Is(err, notifications.ErrInvalidSender):
		return api.UpdateNotificationSender400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNotificationSender)}, nil
	case errors.Is(err, notifications.ErrSenderNotFound):
		return api.UpdateNotificationSender404JSONResponse{NotFoundJSONResponse: notFound(api.NotificationSenderNotFound)}, nil
	case errors.Is(err, notifications.ErrSenderNameInUse):
		return api.UpdateNotificationSender409JSONResponse{ConflictJSONResponse: conflict(api.NotificationSenderNameInUse)}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateNotificationSender200JSONResponse(notificationSenderResponse(sender)), nil
}

func (s apiServer) DeleteNotificationSender(ctx context.Context, request api.DeleteNotificationSenderRequestObject) (api.DeleteNotificationSenderResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.DeleteNotificationSender401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.DeleteNotificationSender403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	err = s.notifications.DeleteSender(ctx, request.SenderId)
	switch {
	case errors.Is(err, notifications.ErrSenderNotFound):
		return api.DeleteNotificationSender404JSONResponse{NotFoundJSONResponse: notFound(api.NotificationSenderNotFound)}, nil
	case errors.Is(err, notifications.ErrSenderInUse):
		return api.DeleteNotificationSender409JSONResponse{ConflictJSONResponse: conflict(api.NotificationSenderInUse)}, nil
	case errors.Is(err, notifications.ErrSenderHasActiveWork):
		return api.DeleteNotificationSender409JSONResponse{ConflictJSONResponse: conflict(api.NotificationSenderActive)}, nil
	case err != nil:
		return nil, err
	}
	return api.DeleteNotificationSender204Response{}, nil
}

func (s apiServer) CreateNotificationTestDelivery(ctx context.Context, request api.CreateNotificationTestDeliveryRequestObject) (api.CreateNotificationTestDeliveryResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateNotificationTestDelivery401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateNotificationTestDelivery403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	delivery, err := s.notifications.CreateTestDelivery(ctx, request.SenderId)
	if errors.Is(err, notifications.ErrSenderNotFound) {
		return api.CreateNotificationTestDelivery404JSONResponse{NotFoundJSONResponse: notFound(api.NotificationSenderNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	response, err := notificationDeliveryResponse(delivery)
	if err != nil {
		return nil, err
	}
	return api.CreateNotificationTestDelivery202JSONResponse(response), nil
}

func (s apiServer) ListNotificationRules(ctx context.Context, _ api.ListNotificationRulesRequestObject) (api.ListNotificationRulesResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListNotificationRules401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	rules, err := s.notifications.Rules(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]api.NotificationRule, 0, len(rules))
	for _, rule := range rules {
		items = append(items, notificationRuleResponse(rule))
	}
	return api.ListNotificationRules200JSONResponse{Items: items}, nil
}

func (s apiServer) CreateNotificationRule(ctx context.Context, request api.CreateNotificationRuleRequestObject) (api.CreateNotificationRuleResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateNotificationRule401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateNotificationRule403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.CreateNotificationRule400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	rule, err := s.notifications.CreateRule(ctx, notificationRuleWrite(*request.Body))
	switch {
	case errors.Is(err, notifications.ErrInvalidRule):
		return api.CreateNotificationRule400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNotificationRule)}, nil
	case errors.Is(err, notifications.ErrRuleNameInUse):
		return api.CreateNotificationRule409JSONResponse{ConflictJSONResponse: conflict(api.NotificationRuleNameInUse)}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateNotificationRule201JSONResponse(notificationRuleResponse(rule)), nil
}

func (s apiServer) UpdateNotificationRule(ctx context.Context, request api.UpdateNotificationRuleRequestObject) (api.UpdateNotificationRuleResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNotificationRule401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNotificationRule403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNotificationRule400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	rule, err := s.notifications.UpdateRule(ctx, request.RuleId, notificationRuleWrite(*request.Body))
	switch {
	case errors.Is(err, notifications.ErrInvalidRule):
		return api.UpdateNotificationRule400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNotificationRule)}, nil
	case errors.Is(err, notifications.ErrRuleNotFound):
		return api.UpdateNotificationRule404JSONResponse{NotFoundJSONResponse: notFound(api.NotificationRuleNotFound)}, nil
	case errors.Is(err, notifications.ErrRuleNameInUse):
		return api.UpdateNotificationRule409JSONResponse{ConflictJSONResponse: conflict(api.NotificationRuleNameInUse)}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateNotificationRule200JSONResponse(notificationRuleResponse(rule)), nil
}

func (s apiServer) DeleteNotificationRule(ctx context.Context, request api.DeleteNotificationRuleRequestObject) (api.DeleteNotificationRuleResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.DeleteNotificationRule401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.DeleteNotificationRule403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	err = s.notifications.DeleteRule(ctx, request.RuleId)
	if errors.Is(err, notifications.ErrRuleNotFound) {
		return api.DeleteNotificationRule404JSONResponse{NotFoundJSONResponse: notFound(api.NotificationRuleNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.DeleteNotificationRule204Response{}, nil
}

func (s apiServer) ListNotificationDeliveries(ctx context.Context, request api.ListNotificationDeliveriesRequestObject) (api.ListNotificationDeliveriesResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListNotificationDeliveries401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	filter := notifications.DeliveryFilter{Page: 1, PageSize: 50}
	if request.Params.Page != nil {
		filter.Page = int(*request.Params.Page)
	}
	if request.Params.PageSize != nil {
		filter.PageSize = int(*request.Params.PageSize)
	}
	if request.Params.SenderId != nil {
		filter.SenderID = request.Params.SenderId
	}
	if request.Params.Status != nil {
		filter.Status = string(*request.Params.Status)
	}
	page, err := s.notifications.Deliveries(ctx, filter)
	if errors.Is(err, notifications.ErrInvalidDeliveryQuery) {
		return api.ListNotificationDeliveries400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNotificationDeliveryQuery)}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.NotificationDelivery, 0, len(page.Items))
	for _, delivery := range page.Items {
		item, err := notificationDeliveryResponse(delivery)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return api.ListNotificationDeliveries200JSONResponse{
		Items: items, Page: int64(page.Page), PageSize: int64(page.PageSize),
		TotalItems: page.TotalItems, TotalPages: int64(page.TotalPages),
	}, nil
}

func notificationSenderCreate(input api.NotificationSenderCreate) notifications.SenderCreate {
	configuration := notifications.SenderConfiguration{}
	if input.Telegram != nil {
		configuration.Telegram = &notifications.TelegramConfiguration{
			ChatID: input.Telegram.ChatId, Token: input.Telegram.Token, TopicID: input.Telegram.TopicId,
		}
	}
	if input.Webhook != nil {
		configuration.Webhook = &notifications.WebhookConfiguration{URL: input.Webhook.Url, Headers: input.Webhook.Headers}
	}
	if input.Javascript != nil {
		configuration.JavaScript = &notifications.JavaScriptConfiguration{Source: input.Javascript.Source}
	}
	return notifications.SenderCreate{Name: input.Name, Kind: string(input.Kind), Enabled: input.Enabled, Configuration: configuration}
}

func notificationSenderUpdate(input api.NotificationSenderUpdate) notifications.SenderUpdate {
	result := notifications.SenderUpdate{Name: input.Name, Enabled: input.Enabled}
	if input.Telegram != nil {
		result.Telegram = &notifications.TelegramUpdate{
			ChatID: input.Telegram.ChatId, Token: input.Telegram.Token, TopicID: input.Telegram.TopicId,
		}
	}
	if input.Webhook != nil {
		result.Webhook = &notifications.WebhookUpdate{URL: input.Webhook.Url, Headers: input.Webhook.Headers}
	}
	if input.Javascript != nil {
		result.JavaScript = &notifications.JavaScriptConfiguration{Source: input.Javascript.Source}
	}
	return result
}

func notificationSenderResponse(sender notifications.Sender) api.NotificationSender {
	result := api.NotificationSender{
		Id: sender.ID, Name: sender.Name, Kind: api.NotificationSenderKind(sender.Kind), Enabled: sender.Enabled,
		CreatedAt: sender.CreatedAt, UpdatedAt: sender.UpdatedAt,
	}
	switch sender.Kind {
	case notifications.SenderTelegram:
		result.Telegram = &api.TelegramSenderView{
			ChatId: sender.Configuration.Telegram.ChatID, TokenConfigured: true,
			TopicId: sender.Configuration.Telegram.TopicID,
		}
	case notifications.SenderWebhook:
		result.Webhook = &api.WebhookSenderView{
			Url:         sender.Configuration.Webhook.URL,
			HeaderNames: notifications.HeaderNames(*sender.Configuration.Webhook),
		}
	case notifications.SenderJavaScript:
		result.Javascript = &api.JavaScriptSenderConfiguration{Source: sender.Configuration.JavaScript.Source}
	}
	return result
}

func notificationRuleWrite(input api.NotificationRuleWrite) notifications.RuleCreate {
	return notifications.RuleCreate{
		Name: input.Name, Enabled: input.Enabled, SenderID: input.SenderId,
		EventType: string(input.EventType), FieldID: input.FieldId,
		NodeID: input.NodeId, EgressID: input.EgressId,
	}
}

func notificationRuleResponse(rule notifications.Rule) api.NotificationRule {
	return api.NotificationRule{
		Id: rule.ID, Name: rule.Name, Enabled: rule.Enabled, SenderId: rule.SenderID,
		EventType: api.NotificationEventType(rule.EventType), FieldId: rule.FieldID,
		NodeId: rule.NodeID, EgressId: rule.EgressID, PublicAddress: rule.PublicAddress,
		CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt,
	}
}

func notificationDeliveryResponse(delivery notifications.Delivery) (api.NotificationDelivery, error) {
	var event map[string]interface{}
	if err := json.Unmarshal(delivery.Event, &event); err != nil {
		return api.NotificationDelivery{}, err
	}
	return api.NotificationDelivery{
		Id: delivery.ID, EventId: delivery.EventID, SenderId: delivery.SenderID,
		SenderName: delivery.SenderName, SenderKind: api.NotificationSenderKind(delivery.SenderKind),
		EventType: delivery.EventType, NodeId: delivery.NodeID, EgressId: delivery.EgressID,
		Test: delivery.Test, Status: api.NotificationDeliveryStatus(delivery.Status),
		AttemptCount: delivery.AttemptCount, NextAttemptAt: delivery.NextAttemptAt,
		LastAttemptAt: delivery.LastAttemptAt, CompletedAt: delivery.CompletedAt,
		ErrorCode: delivery.ErrorCode, MatchedRuleIds: delivery.MatchedRuleIDs,
		Event: event, Title: delivery.Title, Body: delivery.Body,
		CreatedAt: delivery.CreatedAt, UpdatedAt: delivery.UpdatedAt,
	}, nil
}
