package center

import (
	"context"
	"errors"

	centerupdates "github.com/ipchronicle/ipchronicle/internal/center/updates"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

func (s apiServer) GetAgentUpdateState(ctx context.Context, _ api.GetAgentUpdateStateRequestObject) (api.GetAgentUpdateStateResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetAgentUpdateState401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	state, err := s.updates.State(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetAgentUpdateState200JSONResponse(agentUpdateStateResponse(state)), nil
}

func (s apiServer) CreateAgentUpdateTasks(ctx context.Context, request api.CreateAgentUpdateTasksRequestObject) (api.CreateAgentUpdateTasksResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateAgentUpdateTasks401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateAgentUpdateTasks403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.CreateAgentUpdateTasks400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	result, err := s.updates.CreateTasks(ctx, request.Body.NodeIds, request.Body.TargetVersion)
	if errors.Is(err, centerupdates.ErrInvalidTarget) || errors.Is(err, centerupdates.ErrTargetUnavailable) {
		return api.CreateAgentUpdateTasks400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.CreateAgentUpdateTasks202JSONResponse(agentUpdateBatchResponse(result)), nil
}

func (s apiServer) UpdateReleaseChannel(ctx context.Context, request api.UpdateReleaseChannelRequestObject) (api.UpdateReleaseChannelResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateReleaseChannel401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateReleaseChannel403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil || !request.Body.Channel.Valid() {
		return api.UpdateReleaseChannel400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	state, err := s.updates.SetChannel(ctx, string(request.Body.Channel))
	if errors.Is(err, centerupdates.ErrInvalidChannel) {
		return api.UpdateReleaseChannel400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateReleaseChannel200JSONResponse(agentUpdateStateResponse(state)), nil
}

func agentUpdateStateResponse(state centerupdates.State) api.AgentUpdateState {
	response := api.AgentUpdateState{
		Channel: api.ReleaseChannel(state.Channel), CurrentVersion: state.CurrentVersion,
		CurrentRevision: state.CurrentRevision, CheckedAt: state.CheckedAt,
		Tasks: make([]api.AgentUpdateTask, 0, len(state.Tasks)),
	}
	if state.Available != nil {
		response.AvailableRelease = &api.AgentUpdateRelease{
			Version: state.Available.Version, Tag: state.Available.Tag,
			Channel: api.ReleaseChannel(state.Available.Channel), Revision: state.Available.Revision,
			PublishedAt:       state.Available.PublishedAt,
			AgentCapabilities: append([]string{}, state.Available.AgentCapabilities...),
		}
	}
	if state.DiscoveryError != nil {
		value := api.AgentUpdateDiscoveryError(*state.DiscoveryError)
		response.DiscoveryError = &value
	}
	for _, task := range state.Tasks {
		response.Tasks = append(response.Tasks, agentUpdateTaskResponse(task))
	}
	return response
}

func agentUpdateBatchResponse(result centerupdates.BatchResult) api.AgentUpdateBatchResult {
	response := api.AgentUpdateBatchResult{
		TargetVersion: result.TargetVersion,
		Items:         make([]api.AgentUpdateBatchItem, 0, len(result.Items)),
	}
	for _, item := range result.Items {
		converted := api.AgentUpdateBatchItem{NodeId: item.NodeID, Accepted: item.Accepted}
		if item.Task != nil {
			task := agentUpdateTaskResponse(*item.Task)
			converted.Task = &task
		}
		if item.Error != nil {
			value := api.AgentUpdateErrorCode(*item.Error)
			converted.Error = &value
		}
		response.Items = append(response.Items, converted)
	}
	return response
}

func agentUpdateTaskResponse(task centerupdates.Task) api.AgentUpdateTask {
	return api.AgentUpdateTask{
		Id: task.ID, NodeId: task.NodeID, TargetVersion: task.TargetVersion,
		Status: api.AgentUpdateTaskStatus(task.Status), CreatedAt: task.CreatedAt,
		ExpiresAt: task.ExpiresAt, AcknowledgedAt: task.AcknowledgedAt,
		StartedAt: task.StartedAt, CompletedAt: task.CompletedAt,
		PreviousVersion: task.PreviousVersion, ResultVersion: task.ResultVersion,
		FailureCode: task.FailureCode, Diagnostic: task.Diagnostic, Offline: task.Offline,
	}
}
