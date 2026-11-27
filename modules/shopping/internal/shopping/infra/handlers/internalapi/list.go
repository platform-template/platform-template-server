package internalapi

import (
	"context"
	"errors"
	"uuid"

	shoppinginternalapi "shopping/api/server/internalapi"
	"shopping/internal/shopping/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h handler) IsListOwner(
	ctx context.Context,
	req *shoppinginternalapi.IsListOwnerRequest,
) (*shoppinginternalapi.IsListOwnerResponse, error) {
	user, err := uuid.Parse(req.GetUserId())
	if err != nil || user == uuid.Nil() {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID")
	}
	list, err := uuid.Parse(req.GetListId())
	if err != nil || list == uuid.Nil() {
		return nil, status.Error(codes.InvalidArgument, "invalid list ID")
	}
	_, err = h.svc.GetList(ctx, user, list)
	if errors.Is(err, domain.ErrListNotFound) {
		return &shoppinginternalapi.IsListOwnerResponse{}, nil
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "check list owner failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "check list owner failed")
	}
	return &shoppinginternalapi.IsListOwnerResponse{IsOwner: true}, nil
}
