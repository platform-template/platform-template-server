package public

import (
	"context"

	publicapi "bff/api/server/publicapi"
	"bff/internal/bff/app"
)

func NewHandler(svc *app.Service, bootstrap *app.BootstrapService) publicapi.StrictServerInterface {
	return &handler{
		svc:       svc,
		bootstrap: bootstrap,
	}
}

type handler struct {
	svc       *app.Service
	bootstrap *app.BootstrapService
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
