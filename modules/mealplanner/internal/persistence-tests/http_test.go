package persistence_tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
	public "mealplanner/internal/mealplanner/infra/handlers/public"
	sessionapi "session/api/server/internalapi"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeSessions struct {
	sessionapi.SessionServiceClient
	owner, stranger uuid.UUID
}

func (s fakeSessions) ResolveSession(
	_ context.Context,
	req *sessionapi.ResolveSessionRequest,
	_ ...grpc.CallOption,
) (*sessionapi.ResolveSessionResponse, error) {
	switch req.GetToken() {
	case "owner":
		return &sessionapi.ResolveSessionResponse{UserId: s.owner.String()}, nil
	case "stranger":
		return &sessionapi.ResolveSessionResponse{UserId: s.stranger.String()}, nil
	case "offline":
		return nil, status.Error(codes.Unavailable, "offline")
	default:
		return nil, status.Error(codes.Unauthenticated, "invalid")
	}
}
func testPlanHTTP(t *testing.T, service *app.Service) {
	t.Helper()
	sessions := fakeSessions{owner: uuid.New(), stranger: uuid.New()}
	recipe, product := uuid.New(), uuid.New()
	planning := app.NewPlanningService(service, app.PlanningDependencies{
		Inventory: fakeInventory{}, Shopping: fakeShopping{},
		Recipes: fakeRecipes{snapshot: domain.RecipeSnapshot{
			RecipeID:    recipe,
			Name:        testRecipeName,
			Servings:    2,
			Ingredients: []domain.Ingredient{{ProductID: product, Quantity: 3}},
		}},
	})
	register, err := public.Register(planning, slog.Default(), sessions)
	require.NoError(t, err)
	router := chi.NewRouter()
	require.NoError(t, register(router))
	request := func(method, path, token, body, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Cookie", "session="+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}
	base := "/api/public/v1/meal-plans"
	require.Equal(t, http.StatusUnauthorized, request("GET", base, "", "", "").Code)
	require.Equal(t, http.StatusServiceUnavailable, request("GET", base, "offline", "", "").Code)
	require.Equal(
		t,
		http.StatusForbidden,
		request("POST", base, "owner", `{"name":"Week"}`, "https://evil.example").Code,
	)
	created := request("POST", base, "owner", `{"name":"Week"}`, "")
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var plan struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &plan))
	require.NotEmpty(t, plan.ID)
	path := base + "/" + plan.ID
	require.Equal(t, http.StatusNotFound, request("GET", path, "stranger", "", "").Code)
	require.Equal(t, http.StatusNotFound, request("PATCH", path, "stranger", `{"name":"Bad"}`, "").Code)
	require.Equal(t, http.StatusNoContent, request("PATCH", path, "owner", `{"name":"New"}`, "").Code)
	got := request("GET", path, "owner", "", "")
	require.Equal(t, http.StatusOK, got.Code)
	require.Contains(t, got.Body.String(), "New")
	listed := request("GET", base, "owner", "", "")
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listed.Body.String(), plan.ID)
	require.Equal(t, http.StatusBadRequest, request("POST", base, "owner", `{"name":""}`, "").Code)
	schedule := request(
		"POST",
		path+"/items",
		"owner",
		`{"kind":"dish","recipeId":"`+recipe.String()+`","servings":2,"scheduledAt":"2026-12-01T12:00:00Z","inventoryIds":["`+uuid.New().
			String()+
			`"],"shoppingListId":"`+uuid.New().
			String()+
			`"}`,
		"",
	)
	require.Equal(t, http.StatusCreated, schedule.Code, schedule.Body.String())
	var item struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(schedule.Body.Bytes(), &item))
	require.NotEmpty(t, item.ID)
	require.Equal(t, http.StatusBadRequest, request("POST", path+"/items/"+item.ID+"/completion", "owner", "", "").Code)
	require.Equal(
		t,
		http.StatusNoContent,
		request("POST", path+"/items/"+item.ID+"/cancellation", "owner", "", "").Code,
	)
}
