package public

import (
	"context"
	"errors"
	"uuid"

	publicapi "mealplanner/api/server/publicapi"
	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
)

func mealItemOutput(item *domain.Item) publicapi.MealItem {
	data := item.Data()
	result := publicapi.MealItem{
		Id: data.ID.String(), PlanId: data.PlanID.String(), Kind: string(data.Kind), ScheduledAt: data.ScheduledAt,
		State: string(data.State), Revision: data.Revision.String(),
	}
	if len(data.Shortages) > 0 {
		result.Shortages = &[]publicapi.Ingredient{}
		for _, shortage := range data.Shortages {
			*result.Shortages = append(
				*result.Shortages,
				publicapi.Ingredient{ProductId: shortage.ProductID.String(), Quantity: shortage.Quantity},
			)
		}
	}
	return result
}

func parsePublicID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	return id, err == nil && id != uuid.Nil()
}

func (h handler) ScheduleItem(
	ctx context.Context,
	req publicapi.ScheduleItemRequestObject,
) (publicapi.ScheduleItemResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	planID, ok := parsePublicID(req.PlanId)
	if !ok || req.Body == nil {
		return publicapi.ScheduleItem400Response{}, nil
	}
	body := *req.Body
	shoppingID, ok := parsePublicID(body.ShoppingListId)
	if !ok {
		return publicapi.ScheduleItem400Response{}, nil
	}
	input := app.ScheduleInput{
		PlanID:         planID,
		ShoppingListID: shoppingID,
		ScheduledAt:    body.ScheduledAt,
		Kind:           domain.ItemKind(body.Kind),
	}
	for _, raw := range body.InventoryIds {
		id, ok := parsePublicID(raw)
		if !ok {
			return publicapi.ScheduleItem400Response{}, nil
		}
		input.InventoryIDs = append(input.InventoryIDs, id)
	}
	if body.RecipeId != nil {
		input.RecipeID, ok = parsePublicID(*body.RecipeId)
		if !ok {
			return publicapi.ScheduleItem400Response{}, nil
		}
	}
	if body.ProductId != nil {
		input.ProductID, ok = parsePublicID(*body.ProductId)
		if !ok {
			return publicapi.ScheduleItem400Response{}, nil
		}
	}
	if body.Servings != nil {
		input.Servings = int64(*body.Servings)
	}
	if body.Quantity != nil {
		input.Quantity = *body.Quantity
	}
	item, err := h.svc.Schedule(ctx, owner, input)
	if errors.Is(err, domain.ErrPlanNotFound) {
		return publicapi.ScheduleItem404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidItem) {
		return publicapi.ScheduleItem400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.ScheduleItem201JSONResponse(mealItemOutput(item)), nil
}

func (h handler) GetItem(
	ctx context.Context,
	req publicapi.GetItemRequestObject,
) (publicapi.GetItemResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	planID, validPlan := parsePublicID(req.PlanId)
	itemID, validItem := parsePublicID(req.ItemId)
	if !validPlan || !validItem {
		return publicapi.GetItem400Response{}, nil
	}
	item, err := h.svc.GetItem(ctx, owner, planID, itemID)
	if errors.Is(err, domain.ErrPlanNotFound) || errors.Is(err, domain.ErrItemNotFound) {
		return publicapi.GetItem404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetItem200JSONResponse(mealItemOutput(item)), nil
}

func (h handler) ListCalendar(
	ctx context.Context,
	req publicapi.ListCalendarRequestObject,
) (publicapi.ListCalendarResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	planID, validPlan := parsePublicID(req.PlanId)
	if !validPlan || !req.Params.From.Before(req.Params.To) {
		return publicapi.ListCalendar400Response{}, nil
	}
	items, err := h.svc.ListCalendar(ctx, owner, planID, req.Params.From, req.Params.To)
	if errors.Is(err, domain.ErrPlanNotFound) {
		return publicapi.ListCalendar404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidItem) {
		return publicapi.ListCalendar400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]publicapi.MealItem, 0, len(items))
	for _, item := range items {
		result = append(result, mealItemOutput(item))
	}
	return publicapi.ListCalendar200JSONResponse{Items: result}, nil
}

func (h handler) CompleteItem(
	ctx context.Context,
	req publicapi.CompleteItemRequestObject,
) (publicapi.CompleteItemResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	planID, validPlan := parsePublicID(req.PlanId)
	itemID, validItem := parsePublicID(req.ItemId)
	if !validPlan || !validItem {
		return publicapi.CompleteItem400Response{}, nil
	}
	err = h.svc.CompleteItem(ctx, owner, planID, itemID)
	if errors.Is(err, domain.ErrPlanNotFound) {
		return publicapi.CompleteItem404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidItem) || errors.Is(err, domain.ErrInvalidTransition) {
		return publicapi.CompleteItem400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CompleteItem204Response{}, nil
}

func (h handler) CancelItem(
	ctx context.Context,
	req publicapi.CancelItemRequestObject,
) (publicapi.CancelItemResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	planID, validPlan := parsePublicID(req.PlanId)
	itemID, validItem := parsePublicID(req.ItemId)
	if !validPlan || !validItem {
		return publicapi.CancelItem400Response{}, nil
	}
	err = h.svc.CancelItem(ctx, owner, planID, itemID)
	if errors.Is(err, domain.ErrPlanNotFound) {
		return publicapi.CancelItem404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidItem) || errors.Is(err, domain.ErrInvalidTransition) {
		return publicapi.CancelItem400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CancelItem204Response{}, nil
}
