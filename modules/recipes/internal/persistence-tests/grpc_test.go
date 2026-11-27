package persistence_tests

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"uuid"

	recipesapi "recipes/api/server/internalapi"
	"recipes/internal/recipes/app"
	"recipes/internal/recipes/domain"
	handler "recipes/internal/recipes/infra/handlers/internalapi"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func testRecipeSnapshot(t *testing.T, service *app.Service, owner, stranger, recipeID, product uuid.UUID) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	recipesapi.RegisterRecipesServiceServer(server, handler.NewHandler(service, slog.Default()))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///recipes",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := recipesapi.NewRecipesServiceClient(conn)
	request := &recipesapi.GetRecipeSnapshotRequest{UserId: owner.String(), RecipeId: recipeID.String(), Servings: 3}
	snapshot, err := client.GetRecipeSnapshot(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, recipeID.String(), snapshot.GetRecipeId())
	require.Equal(t, "Каша", snapshot.GetName())
	require.EqualValues(t, 3, snapshot.GetServings())
	require.Len(t, snapshot.GetIngredients(), 1)
	require.Equal(t, product.String(), snapshot.GetIngredients()[0].GetProductId())
	require.EqualValues(t, 11, snapshot.GetIngredients()[0].GetQuantity())
	request.UserId = stranger.String()
	_, err = client.GetRecipeSnapshot(t.Context(), request)
	require.Equal(t, codes.NotFound, status.Code(err))
	request.UserId = "bad"
	_, err = client.GetRecipeSnapshot(t.Context(), request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	request.UserId = owner.String()
	request.Servings = 0
	_, err = client.GetRecipeSnapshot(t.Context(), request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	request.Servings = 3
	revised := domain.RecipeData{
		Name:        "Другая",
		Servings:    1,
		Ingredients: []domain.Ingredient{{ProductID: product, Quantity: 2}},
	}
	require.NoError(t, service.ReviseRecipe(t.Context(), owner, recipeID, revised))
	require.Equal(t, "Каша", snapshot.GetName())
	require.EqualValues(t, 11, snapshot.GetIngredients()[0].GetQuantity())
	current, err := client.GetRecipeSnapshot(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "Другая", current.GetName())
	require.EqualValues(t, 6, current.GetIngredients()[0].GetQuantity())
}
