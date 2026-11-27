package public

import (
	"context"
	"errors"
	"uuid"

	publicapi "mealplanner/api/server/publicapi"
	"mealplanner/internal/mealplanner/domain"
)

func authenticatedUser(ctx context.Context) (uuid.UUID, error) {
	user, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok || user == uuid.Nil() {
		return uuid.Nil(), errors.New("missing authenticated user")
	}
	return user, nil
}
func planOutput(plan *domain.Plan) publicapi.Plan {
	return publicapi.Plan{Id: plan.ID().String(), Name: plan.Name()}
}

func (h handler) ListPlans(
	ctx context.Context,
	_ publicapi.ListPlansRequestObject,
) (publicapi.ListPlansResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	plans, err := h.svc.ListPlans(ctx, owner)
	if err != nil {
		return nil, err
	}
	result := make([]publicapi.Plan, 0, len(plans))
	for _, plan := range plans {
		result = append(result, planOutput(plan))
	}
	return publicapi.ListPlans200JSONResponse{Plans: result}, nil
}

func (h handler) CreatePlan(
	ctx context.Context,
	req publicapi.CreatePlanRequestObject,
) (publicapi.CreatePlanResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return publicapi.CreatePlan400Response{}, nil
	}
	plan, err := h.svc.CreatePlan(ctx, owner, req.Body.Name)
	if errors.Is(err, domain.ErrInvalidPlan) {
		return publicapi.CreatePlan400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreatePlan201JSONResponse(planOutput(plan)), nil
}

func (h handler) GetPlan(
	ctx context.Context,
	req publicapi.GetPlanRequestObject,
) (publicapi.GetPlanResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() {
		return publicapi.GetPlan400Response{}, nil
	}
	plan, err := h.svc.GetPlan(ctx, owner, id)
	if errors.Is(err, domain.ErrPlanNotFound) {
		return publicapi.GetPlan404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetPlan200JSONResponse(planOutput(plan)), nil
}

func (h handler) RenamePlan(
	ctx context.Context,
	req publicapi.RenamePlanRequestObject,
) (publicapi.RenamePlanResponseObject, error) {
	owner, err := authenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() || req.Body == nil {
		return publicapi.RenamePlan400Response{}, nil
	}
	err = h.svc.RenamePlan(ctx, owner, id, req.Body.Name)
	if errors.Is(err, domain.ErrPlanNotFound) {
		return publicapi.RenamePlan404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidPlan) {
		return publicapi.RenamePlan400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.RenamePlan204Response{}, nil
}
