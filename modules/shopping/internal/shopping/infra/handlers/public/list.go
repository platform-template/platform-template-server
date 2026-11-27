package public

import (
	"context"
	"errors"
	"uuid"

	publicapi "shopping/api/server/publicapi"
	"shopping/internal/shopping/domain"
)

func authenticatedUser(ctx context.Context) (uuid.UUID, error) {
	user, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok || user == uuid.Nil() {
		return uuid.Nil(), errors.New("missing authenticated user")
	}
	return user, nil
}
func listOutput(list *domain.List) publicapi.ShoppingList {
	return publicapi.ShoppingList{Id: list.ID().String(), Name: list.Name()}
}

func demandOutput(demand *domain.Demand) publicapi.ShoppingDemand {
	data := demand.Data()
	return publicapi.ShoppingDemand{
		Id:        data.ID.String(),
		ProductId: data.ProductID.String(),
		Required:  data.Required,
		Purchased: data.Purchased,
		Remaining: demand.Remaining(),
		Automatic: data.Source != nil,
	}
}

func (h handler) ListLists(
	ctx context.Context,
	_ publicapi.ListListsRequestObject,
) (publicapi.ListListsResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	lists, err := h.svc.ListLists(ctx, owner)
	if err != nil {
		return nil, err
	}
	result := make([]publicapi.ShoppingList, 0, len(lists))
	for _, list := range lists {
		result = append(result, listOutput(list))
	}
	return publicapi.ListLists200JSONResponse{Lists: result}, nil
}

func (h handler) CreateList(
	ctx context.Context,
	req publicapi.CreateListRequestObject,
) (publicapi.CreateListResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return publicapi.CreateList400Response{}, nil
	}
	list, err := h.svc.CreateList(ctx, owner, req.Body.Name)
	if errors.Is(err, domain.ErrInvalidList) {
		return publicapi.CreateList400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreateList201JSONResponse(listOutput(list)), nil
}

func (h handler) GetList(
	ctx context.Context,
	req publicapi.GetListRequestObject,
) (publicapi.GetListResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() {
		return publicapi.GetList400Response{}, nil
	}
	list, err := h.svc.GetList(ctx, owner, id)
	if errors.Is(err, domain.ErrListNotFound) {
		return publicapi.GetList404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetList200JSONResponse(listOutput(list)), nil
}

func (h handler) RenameList(
	ctx context.Context,
	req publicapi.RenameListRequestObject,
) (publicapi.RenameListResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() || req.Body == nil {
		return publicapi.RenameList400Response{}, nil
	}
	err = h.svc.RenameList(ctx, owner, id, req.Body.Name)
	if errors.Is(err, domain.ErrListNotFound) {
		return publicapi.RenameList404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidList) {
		return publicapi.RenameList400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.RenameList204Response{}, nil
}

func (h handler) CreateManualDemand(
	ctx context.Context,
	req publicapi.CreateManualDemandRequestObject,
) (publicapi.CreateManualDemandResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	listID, err := uuid.Parse(req.Id)
	if err != nil || listID == uuid.Nil() || req.Body == nil {
		return publicapi.CreateManualDemand400Response{}, nil
	}
	productID, err := uuid.Parse(req.Body.ProductId)
	if err != nil || productID == uuid.Nil() || req.Body.Quantity <= 0 {
		return publicapi.CreateManualDemand400Response{}, nil
	}
	demand, err := h.svc.CreateManualDemand(ctx, owner, listID, productID, req.Body.Quantity)
	if errors.Is(err, domain.ErrListNotFound) {
		return publicapi.CreateManualDemand404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidDemand) {
		return publicapi.CreateManualDemand400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreateManualDemand201JSONResponse(demandOutput(demand)), nil
}

func (h handler) ListDemands(
	ctx context.Context,
	req publicapi.ListDemandsRequestObject,
) (publicapi.ListDemandsResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	listID, err := uuid.Parse(req.Id)
	if err != nil || listID == uuid.Nil() {
		return publicapi.ListDemands400Response{}, nil
	}
	demands, err := h.svc.ListDemands(ctx, owner, listID)
	if errors.Is(err, domain.ErrListNotFound) {
		return publicapi.ListDemands404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]publicapi.ShoppingDemand, 0, len(demands))
	for _, demand := range demands {
		result = append(result, demandOutput(demand))
	}
	return publicapi.ListDemands200JSONResponse{Demands: result}, nil
}

func (h handler) ConfirmPurchase(
	ctx context.Context,
	req publicapi.ConfirmPurchaseRequestObject,
) (publicapi.ConfirmPurchaseResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	listID, err := uuid.Parse(req.Id)
	if err != nil || listID == uuid.Nil() || req.Body == nil {
		return publicapi.ConfirmPurchase400Response{}, nil
	}
	demandID, err := uuid.Parse(req.DemandId)
	if err != nil || demandID == uuid.Nil() {
		return publicapi.ConfirmPurchase400Response{}, nil
	}
	operationID, err := uuid.Parse(req.Body.OperationId)
	if err != nil || operationID == uuid.Nil() {
		return publicapi.ConfirmPurchase400Response{}, nil
	}
	inventoryID, err := uuid.Parse(req.Body.InventoryId)
	if err != nil || inventoryID == uuid.Nil() || req.Body.Quantity <= 0 {
		return publicapi.ConfirmPurchase400Response{}, nil
	}
	err = h.svc.ConfirmPurchase(ctx, owner, listID, operationID, demandID, inventoryID, req.Body.Quantity)
	if errors.Is(err, domain.ErrListNotFound) || errors.Is(err, domain.ErrInvalidDemand) {
		return publicapi.ConfirmPurchase404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidPurchase) || errors.Is(err, domain.ErrDemandConflict) {
		return publicapi.ConfirmPurchase400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.ConfirmPurchase202Response{}, nil
}
