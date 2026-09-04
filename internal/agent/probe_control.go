package agent

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/agent/agentlogs"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	"github.com/ipchronicle/ipchronicle/internal/generated/agentapi"
)

const probeUploadRetryInterval = 15 * time.Second

type localProbeUploadError struct {
	cause error
}

func (err localProbeUploadError) Error() string { return err.cause.Error() }
func (err localProbeUploadError) Unwrap() error { return err.cause }

func probeStatusToAPI(status state.ProbeStatus) (*agentapi.AgentProbeStatus, error) {
	result := &agentapi.AgentProbeStatus{
		NextScheduledAt: status.NextScheduledAt, LastOccurrenceAt: status.LastOccurrenceAt,
		HistoryResetGeneration: status.HistoryResetGeneration, HistoryResetAt: status.HistoryResetAt,
	}
	if status.ActiveRunID != nil {
		id, err := uuid.Parse(*status.ActiveRunID)
		if err != nil {
			return nil, err
		}
		result.ActiveRunId = &id
	}
	if status.LastOccurrenceTrigger != nil {
		value := agentapi.ProbeTrigger(*status.LastOccurrenceTrigger)
		result.LastOccurrenceTrigger = &value
	}
	if status.LastOccurrenceStatus != nil {
		value := agentapi.AgentProbeOccurrenceStatus(*status.LastOccurrenceStatus)
		result.LastOccurrenceStatus = &value
	}
	if status.LastSkipReason != nil {
		value := agentapi.AgentProbeSkipReason(*status.LastSkipReason)
		result.LastSkipReason = &value
	}
	if status.HistoryResetGeneration != nil {
		addressItems := status.HistoryResetDiscardedAddressItems
		probeItems := status.HistoryResetDiscardedProbeItems
		result.HistoryResetDiscardedAddressItems = &addressItems
		result.HistoryResetDiscardedProbeItems = &probeItems
	}
	return result, nil
}

func taskReportToAPI(report *state.ProbeTaskReport) (*agentapi.AgentTaskReport, error) {
	if report == nil {
		return nil, nil
	}
	id, err := uuid.Parse(report.ID)
	if err != nil {
		return nil, err
	}
	result := &agentapi.AgentTaskReport{
		Id: id, Status: agentapi.AgentTaskReportStatus(report.Status),
		AcknowledgedAt: report.AcknowledgedAt, StartedAt: report.StartedAt, CompletedAt: report.CompletedAt,
	}
	if report.RunID != nil {
		runID, err := uuid.Parse(*report.RunID)
		if err != nil {
			return nil, err
		}
		result.RunId = &runID
	}
	if report.RejectionReason != nil {
		value := agentapi.AgentProbeSkipReason(*report.RejectionReason)
		result.RejectionReason = &value
	}
	return result, nil
}

func agentUpdateReportToAPI(report *state.AgentUpdateReport) (*agentapi.AgentTaskReport, error) {
	if report == nil {
		return nil, nil
	}
	id, err := uuid.Parse(report.ID)
	if err != nil {
		return nil, err
	}
	return &agentapi.AgentTaskReport{
		Id: id, Status: agentapi.AgentTaskReportStatus(report.Status),
		AcknowledgedAt: report.AcknowledgedAt, StartedAt: report.StartedAt, CompletedAt: report.CompletedAt,
		PreviousVersion: report.PreviousVersion, ResultVersion: report.ResultVersion,
		FailureCode: report.FailureCode, Diagnostic: report.Diagnostic,
	}, nil
}

func (client *ControlClient) runProbeUploader(
	ctx context.Context,
	store *state.Store,
	identity state.Identity,
	wake <-chan struct{},
	logger *log.Logger,
	events agentlogs.Sink,
) error {
	for {
		found, err := client.uploadNextProbeArtifact(ctx, store, identity, events)
		if err == nil && found {
			continue
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		var localError localProbeUploadError
		if errors.As(err, &localError) || errors.Is(err, ErrAgentRevoked) {
			return err
		}
		wait := time.Second
		if err != nil {
			logger.Printf("complete-probe artifact upload failed: %v", err)
			wait = probeUploadRetryInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		case <-wake:
			if !timer.Stop() {
				<-timer.C
			}
		}
	}
}

func (client *ControlClient) uploadNextProbeArtifact(
	ctx context.Context,
	store *state.Store,
	identity state.Identity,
	sinks ...agentlogs.Sink,
) (bool, error) {
	var events agentlogs.Sink
	if len(sinks) > 0 {
		events = sinks[0]
	}
	artifact, err := store.NextProbeArtifact()
	if err != nil {
		return false, localProbeUploadError{cause: err}
	}
	if artifact.ID == "" {
		return false, nil
	}
	request, err := probeArtifactToAPI(artifact)
	if err != nil {
		return false, localProbeUploadError{cause: err}
	}
	startedAt := time.Now()
	response, err := client.client.UploadProbeArtifactWithResponse(ctx, request, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+identity.Credential)
		return nil
	})
	duration := time.Since(startedAt).Milliseconds()
	target := identity.CenterURL + "/api/v1/agent/probe-artifacts"
	if err != nil {
		emitProbeUploadFailure(events, artifact, "artifact-upload-failed", "Complete-probe result upload failed",
			target, 0, "", nil, duration, err)
		return false, err
	}
	if response.JSON200 == nil {
		emitProbeUploadFailure(events, artifact, "artifact-upload-rejected", "Complete-probe result upload was rejected",
			target, response.StatusCode(), response.ContentType(), response.Body, duration, nil)
		responseErr := responseError("upload complete-probe artifact", response.StatusCode(), response.JSON400, response.JSON401, response.JSON403)
		if response.JSON400 != nil {
			return false, localProbeUploadError{cause: responseErr}
		}
		return false, responseErr
	}
	receipt := response.JSON200
	if receipt.ArtifactId.String() != artifact.ID || receipt.Revision != artifact.Revision || !receipt.Disposition.Valid() {
		emitProbeUploadInvalid(events, artifact, target, response.StatusCode(), response.ContentType(), response.Body, duration)
		return false, localProbeUploadError{cause: errors.New("center returned an invalid complete-probe artifact receipt")}
	}
	if err := store.AcknowledgeProbeArtifact(state.ProbeArtifactReceipt{
		ID: receipt.ArtifactId.String(), Revision: receipt.Revision,
	}); err != nil {
		return false, localProbeUploadError{cause: err}
	}
	event := probeArtifactLogEvent(artifact)
	event.Level = "debug"
	event.Component = "probe-uploader"
	event.EventType = "artifact-upload-succeeded"
	event.Message = "Complete-probe result upload succeeded"
	method := http.MethodPost
	status := response.StatusCode()
	event.RequestMethod = &method
	event.RequestTarget = agentlogs.SanitizeRequestTarget(target)
	event.HTTPStatus = &status
	event.DurationMilliseconds = &duration
	if events != nil {
		events.Emit(event)
	}
	return true, nil
}

func emitProbeUploadFailure(
	events agentlogs.Sink,
	artifact state.ProbeArtifact,
	eventType, message, target string,
	status int,
	contentType string,
	body []byte,
	duration int64,
	requestErr error,
) {
	if events == nil {
		return
	}
	event := probeArtifactLogEvent(artifact)
	event.Level = "warn"
	event.Component = "probe-uploader"
	event.EventType = eventType
	event.Message = message
	category := "http-status"
	if requestErr != nil {
		category = agentlogs.ClassifyRequestError(requestErr)
	} else if status == http.StatusTooManyRequests {
		category = "rate-limit"
	}
	method := http.MethodPost
	event.FailureCategory = &category
	event.RequestMethod = &method
	event.RequestTarget = agentlogs.SanitizeRequestTarget(target)
	event.DurationMilliseconds = &duration
	event.ResponseBody = append([]byte(nil), body...)
	event.ResponseTruncated = len(body) > maxAgentAPIResponseSize
	if status != 0 {
		event.HTTPStatus = &status
	}
	if contentType != "" {
		event.ResponseContentType = &contentType
	}
	events.Emit(event)
}

func emitProbeUploadInvalid(
	events agentlogs.Sink,
	artifact state.ProbeArtifact,
	target string,
	status int,
	contentType string,
	body []byte,
	duration int64,
) {
	if events == nil {
		return
	}
	event := probeArtifactLogEvent(artifact)
	event.Level = "warn"
	event.Component = "probe-uploader"
	event.EventType = "artifact-receipt-invalid"
	event.Message = "Center returned an invalid complete-probe result receipt"
	category := "invalid-response"
	method := http.MethodPost
	event.FailureCategory = &category
	event.RequestMethod = &method
	event.RequestTarget = agentlogs.SanitizeRequestTarget(target)
	event.HTTPStatus = &status
	event.DurationMilliseconds = &duration
	event.ResponseBody = append([]byte(nil), body...)
	if contentType != "" {
		event.ResponseContentType = &contentType
	}
	events.Emit(event)
}

func probeArtifactLogEvent(artifact state.ProbeArtifact) agentlogs.Event {
	event := agentlogs.Event{}
	if artifact.Run != nil {
		event.TaskID = artifact.Run.TaskID
		revision := artifact.Run.ConfigurationRevision
		event.ConfigurationRevision = &revision
	}
	if artifact.Execution != nil {
		event.PublicAddressID = &artifact.Execution.EgressID
	} else if artifact.Gap != nil {
		event.PublicAddressID = &artifact.Gap.EgressID
	}
	return event
}

func probeArtifactToAPI(artifact state.ProbeArtifact) (agentapi.AgentProbeArtifact, error) {
	id, err := uuid.Parse(artifact.ID)
	if err != nil {
		return agentapi.AgentProbeArtifact{}, err
	}
	result := agentapi.AgentProbeArtifact{ArtifactId: id, Revision: artifact.Revision}
	if artifact.Gap != nil {
		gapID, err := uuid.Parse(artifact.Gap.ID)
		if err != nil {
			return agentapi.AgentProbeArtifact{}, err
		}
		egressID, err := uuid.Parse(artifact.Gap.EgressID)
		if err != nil {
			return agentapi.AgentProbeArtifact{}, err
		}
		result.Gap = &agentapi.AgentProbeGapArtifact{
			Id: gapID, EgressId: egressID, HistoryGeneration: artifact.Gap.HistoryGeneration,
			DroppedCount: artifact.Gap.DroppedCount, FirstSequence: artifact.Gap.FirstSequence,
			LastSequence: artifact.Gap.LastSequence, FirstObservedAt: artifact.Gap.FirstObservedAt,
			LastObservedAt: artifact.Gap.LastObservedAt,
		}
		return result, nil
	}
	if artifact.Run == nil {
		return agentapi.AgentProbeArtifact{}, errors.New("complete-probe artifact has no run or gap")
	}
	run, err := probeRunArtifactToAPI(*artifact.Run)
	if err != nil {
		return agentapi.AgentProbeArtifact{}, err
	}
	result.Run = &run
	if artifact.Execution != nil {
		executionID, err := uuid.Parse(artifact.Execution.ID)
		if err != nil {
			return agentapi.AgentProbeArtifact{}, err
		}
		egressID, err := uuid.Parse(artifact.Execution.EgressID)
		if err != nil {
			return agentapi.AgentProbeArtifact{}, err
		}
		execution := agentapi.AgentProbeExecutionArtifact{
			Id: executionID, EgressId: egressID, Ordinal: int(artifact.Execution.Ordinal),
			Sequence: artifact.Execution.Sequence, Status: agentapi.ProbeExecutionStatus(artifact.Execution.Status),
			StartedAt: artifact.Execution.StartedAt, CompletedAt: artifact.Execution.CompletedAt,
			Diagnostic: artifact.Execution.Diagnostic,
		}
		if artifact.Execution.FailureStage != nil {
			value := agentapi.ProbeFailureStage(*artifact.Execution.FailureStage)
			execution.FailureStage = &value
		}
		if len(artifact.RawResult) > 0 {
			raw := append([]byte(nil), artifact.RawResult...)
			execution.RawResult = &raw
		}
		result.Execution = &execution
	}
	return result, nil
}

func probeRunArtifactToAPI(run state.ProbeRun) (agentapi.AgentProbeRunArtifact, error) {
	id, err := uuid.Parse(run.ID)
	if err != nil {
		return agentapi.AgentProbeRunArtifact{}, err
	}
	result := agentapi.AgentProbeRunArtifact{
		Id: id, NodeConfigurationRevision: run.ConfigurationRevision,
		HistoryGeneration: run.HistoryGeneration, Trigger: agentapi.ProbeTrigger(run.Trigger),
		StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Status: agentapi.ProbeRunStatus(run.Status),
		Executions: make([]agentapi.AgentProbeExecutionManifest, 0, len(run.Executions)),
	}
	if run.TaskID != nil {
		id, err := uuid.Parse(*run.TaskID)
		if err != nil {
			return agentapi.AgentProbeRunArtifact{}, err
		}
		result.TaskId = &id
	}
	for _, manifest := range run.Executions {
		id, err := uuid.Parse(manifest.ID)
		if err != nil {
			return agentapi.AgentProbeRunArtifact{}, err
		}
		egressID, err := uuid.Parse(manifest.EgressID)
		if err != nil {
			return agentapi.AgentProbeRunArtifact{}, err
		}
		result.Executions = append(result.Executions, agentapi.AgentProbeExecutionManifest{
			Id: id, EgressId: egressID, Ordinal: int(manifest.Ordinal), Sequence: manifest.Sequence,
		})
	}
	return result, nil
}
