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
