package internalapi

import (
	"context"
	"errors"
	"log/slog"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryinternalapi "inventory/api/server/internalapi"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
)

func NewHandler(svc *app.Service, logger *slog.Logger) inventoryinternalapi.InventoryServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	inventoryinternalapi.UnimplementedInventoryServiceServer
}

func (h handler) Echo(
	ctx context.Context,
	req *inventoryinternalapi.EchoRequest,
) (*inventoryinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &inventoryinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

func (h handler) GetProduct(
	ctx context.Context,
	req *inventoryinternalapi.GetProductRequest,
) (*inventoryinternalapi.Product, error) {
	id, err := parseID(req.GetProductId())
	if err != nil {
		return nil, err
	}
	product, err := h.svc.GetProduct(ctx, id)
	if err != nil {
		return nil, h.operationError(ctx, err)
	}
	return &inventoryinternalapi.Product{
		Id:   product.ID().String(),
		Name: product.Name(),
		Unit: string(product.Unit()),
	}, nil
}

func (h handler) GetStock(
	ctx context.Context,
	req *inventoryinternalapi.GetStockRequest,
) (*inventoryinternalapi.Stock, error) {
	inventoryID, err := parseID(req.GetInventoryId())
	if err != nil {
		return nil, err
	}
	productID, err := parseID(req.GetProductId())
	if err != nil {
		return nil, err
	}
	stock, err := h.svc.GetStock(ctx, inventoryID, productID)
	if err != nil {
		return nil, h.operationError(ctx, err)
	}
	response := &inventoryinternalapi.Stock{InventoryId: inventoryID.String(), ProductId: productID.String()}
	if stock != nil {
		response.Exists = true
		response.Quantity = stock.Quantity
		response.ReservedQuantity = stock.ReservedQuantity
		response.Available = stock.Available()
	}
	return response, nil
}

func (h handler) Reserve(
	ctx context.Context,
	req *inventoryinternalapi.ReserveRequest,
) (*inventoryinternalapi.OperationResponse, error) {
	inventoryID, err := parseID(req.GetInventoryId())
	if err != nil {
		return nil, err
	}
	reservationID, err := parseID(req.GetReservationId())
	if err != nil {
		return nil, err
	}
	items := make([]domain.ReservationItem, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		productID, err := parseID(item.GetProductId())
		if err != nil {
			return nil, err
		}
		items = append(items, domain.ReservationItem{ProductID: productID, Quantity: item.GetQuantity()})
	}
	if err := h.svc.Reserve(ctx, inventoryID, reservationID, items); err != nil {
		return nil, h.operationError(ctx, err)
	}
	return &inventoryinternalapi.OperationResponse{}, nil
}

func (h handler) Release(
	ctx context.Context,
	req *inventoryinternalapi.ReservationRequest,
) (*inventoryinternalapi.OperationResponse, error) {
	inventoryID, reservationID, err := reservationIDs(req)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Release(ctx, inventoryID, reservationID); err != nil {
		return nil, h.operationError(ctx, err)
	}
	return &inventoryinternalapi.OperationResponse{}, nil
}

func (h handler) Consume(
	ctx context.Context,
	req *inventoryinternalapi.ReservationRequest,
) (*inventoryinternalapi.OperationResponse, error) {
	inventoryID, reservationID, err := reservationIDs(req)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Consume(ctx, inventoryID, reservationID); err != nil {
		return nil, h.operationError(ctx, err)
	}
	return &inventoryinternalapi.OperationResponse{}, nil
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), status.Error(codes.InvalidArgument, "invalid identifier")
	}
	return id, nil
}

func reservationIDs(req *inventoryinternalapi.ReservationRequest) (inventoryID, reservationID uuid.UUID, err error) {
	inventoryID, err = parseID(req.GetInventoryId())
	if err != nil {
		return uuid.Nil(), uuid.Nil(), err
	}
	reservationID, err = parseID(req.GetReservationId())
	if err != nil {
		return uuid.Nil(), uuid.Nil(), err
	}
	return inventoryID, reservationID, nil
}

func (h handler) operationError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, domain.ErrInvalidReservation), errors.Is(err, domain.ErrInvalidStock):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrProductNotFound),
		errors.Is(err, domain.ErrInventoryNotFound),
		errors.Is(err, domain.ErrReservationNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrReservationConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInsufficientStock),
		errors.Is(err, domain.ErrReservationCancelled),
		errors.Is(err, domain.ErrReservationConsumed):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		h.logger.ErrorContext(ctx, "inventory operation failed", logging.Error(err))
		return status.Error(codes.Internal, "inventory operation failed")
	}
}
