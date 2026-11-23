package public

import (
	"context"
	"errors"
	"strings"
	"uuid"

	openapitypes "github.com/oapi-codegen/runtime/types"

	publicapi "audit/api/server/publicapi"
	"audit/internal/audit/app"
	"audit/internal/audit/domain"
)

func NewHandler(svc *app.Service) publicapi.StrictServerInterface {
	return &handler{
		svc: svc,
	}
}

type handler struct {
	svc *app.Service
}

func (h handler) ListAuditEntries(
	ctx context.Context,
	req publicapi.ListAuditEntriesRequestObject,
) (publicapi.ListAuditEntriesResponseObject, error) {
	params := req.Params
	filter := domain.AuditFilter{From: params.From, To: params.To}
	if params.ActorId != nil {
		actor := uuid.UUID(*params.ActorId)
		filter.ActorID = &actor
	}
	if params.ModuleKey != nil {
		filter.ModuleKey = *params.ModuleKey
	}
	if params.Action != nil {
		filter.Action = *params.Action
	}
	if params.TargetType != nil {
		filter.TargetType = *params.TargetType
	}
	if params.TargetId != nil {
		filter.TargetID = *params.TargetId
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}
	if params.Offset != nil {
		filter.Offset = *params.Offset
	}
	entries, err := h.svc.ListAuditEntries(ctx, filter)
	if errors.Is(err, domain.ErrInvalidAuditFilter) {
		return publicapi.ListAuditEntries400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	response := publicapi.ListAuditEntries200JSONResponse{Entries: make([]publicapi.AuditEntry, 0, len(entries))}
	for _, entry := range entries {
		response.Entries = append(response.Entries, auditEntryResponse(entry))
	}
	return response, nil
}

func (h handler) GetAuditEntry(
	ctx context.Context,
	req publicapi.GetAuditEntryRequestObject,
) (publicapi.GetAuditEntryResponseObject, error) {
	if strings.TrimSpace(req.Params.EventId) == "" {
		return publicapi.GetAuditEntry400Response{}, nil
	}
	entry, err := h.svc.GetAuditEntry(ctx, req.Params.EventId)
	if errors.Is(err, domain.ErrAuditEntryNotFound) {
		return publicapi.GetAuditEntry404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetAuditEntry200JSONResponse(auditEntryResponse(entry)), nil
}

func auditEntryResponse(entry *domain.AuditEntry) publicapi.AuditEntry {
	data := entry.Data()
	response := publicapi.AuditEntry{
		EventId:    data.EventID,
		ModuleKey:  data.ModuleKey,
		Action:     data.Action,
		Metadata:   data.Metadata,
		OccurredAt: data.OccurredAt,
		RecordedAt: data.RecordedAt,
	}
	if data.ActorID != nil {
		actor := openapitypes.UUID(*data.ActorID)
		response.ActorId = &actor
	}
	if data.TargetType != "" {
		response.TargetType = &data.TargetType
	}
	if data.TargetID != "" {
		response.TargetId = &data.TargetID
	}
	if data.CorrelationID != "" {
		response.CorrelationId = &data.CorrelationID
	}
	return response
}

func (h handler) Echo(ctx context.Context, req publicapi.EchoRequestObject) (publicapi.EchoResponseObject, error) {
	id, err := h.svc.Echo(ctx, req.Body.Body)
	if err != nil {
		return nil, err
	}

	return publicapi.Echo200JSONResponse{
		OperationID: id.String(),
	}, nil
}
