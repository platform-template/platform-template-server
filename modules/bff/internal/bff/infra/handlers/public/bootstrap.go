package public

import (
	"context"

	publicapi "bff/api/server/publicapi"
)

func (h handler) GetBootstrap(
	ctx context.Context,
	_ publicapi.GetBootstrapRequestObject,
) (publicapi.GetBootstrapResponseObject, error) {
	token, err := sessionToken(ctx)
	if err != nil {
		return publicapi.GetBootstrap401Response{}, nil
	}
	bootstrap, err := h.bootstrap.GetBootstrap(ctx, token)
	if err != nil {
		if bootstrapError(err) == 401 {
			return publicapi.GetBootstrap401Response{}, nil
		}
		return publicapi.GetBootstrap503Response{}, nil
	}
	return publicapi.GetBootstrap200JSONResponse{
		UserId: bootstrap.UserID.String(), Authenticated: bootstrap.Authenticated,
		Modules: bootstrap.Modules, Superuser: bootstrap.Superuser,
	}, nil
}

func (h handler) GetNavigation(
	ctx context.Context,
	_ publicapi.GetNavigationRequestObject,
) (publicapi.GetNavigationResponseObject, error) {
	token, err := sessionToken(ctx)
	if err != nil {
		return publicapi.GetNavigation401Response{}, nil
	}
	items, err := h.bootstrap.GetNavigation(ctx, token)
	if err != nil {
		if bootstrapError(err) == 401 {
			return publicapi.GetNavigation401Response{}, nil
		}
		return publicapi.GetNavigation503Response{}, nil
	}
	response := make([]publicapi.NavigationItem, 0, len(items))
	for _, item := range items {
		response = append(response, publicapi.NavigationItem{Id: item.ID, Label: item.Label, Path: item.Path})
	}
	return publicapi.GetNavigation200JSONResponse{Items: response}, nil
}
