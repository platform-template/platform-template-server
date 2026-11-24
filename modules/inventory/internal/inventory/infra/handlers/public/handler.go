package public

import (
	"context"
	"errors"
	"uuid"

	openapitypes "github.com/oapi-codegen/runtime/types"

	publicapi "inventory/api/server/publicapi"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
)

func NewHandler(svc *app.Service) publicapi.StrictServerInterface {
	return &handler{
		svc: svc,
	}
}

type handler struct {
	svc *app.Service
}

func (h handler) ListProducts(
	ctx context.Context,
	req publicapi.ListProductsRequestObject,
) (publicapi.ListProductsResponseObject, error) {
	var filter domain.ProductFilter
	if req.Params.Search != nil {
		filter.Search = *req.Params.Search
	}
	if req.Params.Limit != nil {
		filter.Limit = *req.Params.Limit
	}
	if req.Params.Offset != nil {
		filter.Offset = *req.Params.Offset
	}
	products, err := h.svc.ListProducts(ctx, filter)
	if errors.Is(err, domain.ErrInvalidProductFilter) {
		return publicapi.ListProducts400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	response := publicapi.ListProducts200JSONResponse{Products: make([]publicapi.Product, 0, len(products))}
	for _, product := range products {
		response.Products = append(response.Products, productResponse(product))
	}
	return response, nil
}

func (h handler) CreateProduct(
	ctx context.Context,
	req publicapi.CreateProductRequestObject,
) (publicapi.CreateProductResponseObject, error) {
	if req.Body == nil {
		return publicapi.CreateProduct400Response{}, nil
	}
	product, err := h.svc.CreateProduct(ctx, req.Body.Name, domain.Unit(req.Body.Unit))
	if errors.Is(err, domain.ErrInvalidProduct) {
		return publicapi.CreateProduct400Response{}, nil
	}
	if errors.Is(err, domain.ErrProductExists) {
		return publicapi.CreateProduct409Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreateProduct201JSONResponse(productResponse(*product)), nil
}

func (h handler) GetProduct(
	ctx context.Context,
	req publicapi.GetProductRequestObject,
) (publicapi.GetProductResponseObject, error) {
	product, err := h.svc.GetProduct(ctx, uuid.UUID(req.Id))
	if errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.GetProduct404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetProduct200JSONResponse(productResponse(*product)), nil
}

func (h handler) GetStock(
	ctx context.Context,
	req publicapi.GetStockRequestObject,
) (publicapi.GetStockResponseObject, error) {
	stock, err := h.svc.GetStock(ctx, uuid.UUID(req.Id))
	if errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.GetStock404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetStock200JSONResponse{
		ProductId:        openapitypes.UUID(stock.ProductID),
		Quantity:         stock.Quantity,
		ReservedQuantity: stock.ReservedQuantity,
		Available:        stock.Available(),
	}, nil
}

func (h handler) AddStock(
	ctx context.Context,
	req publicapi.AddStockRequestObject,
) (publicapi.AddStockResponseObject, error) {
	if req.Body == nil {
		return publicapi.AddStock400Response{}, nil
	}
	err := h.svc.AddStock(ctx, uuid.UUID(req.Id), req.Body.Quantity)
	if errors.Is(err, domain.ErrInvalidStock) {
		return publicapi.AddStock400Response{}, nil
	}
	if errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.AddStock404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.AddStock204Response{}, nil
}

func productResponse(product domain.Product) publicapi.Product {
	return publicapi.Product{
		Id:   openapitypes.UUID(product.ID),
		Name: product.Name,
		Unit: string(product.Unit),
	}
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
