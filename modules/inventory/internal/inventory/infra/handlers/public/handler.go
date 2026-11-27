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
		response.Products = append(response.Products, productResponse(*product))
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

func productResponse(product domain.Product) publicapi.Product {
	return publicapi.Product{
		Id:   openapitypes.UUID(product.ID()),
		Name: product.Name(),
		Unit: string(product.Unit()),
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

func userID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(userIDKey{}).(uuid.UUID)
	return id
}

func (h handler) owns(ctx context.Context, id uuid.UUID) (bool, error) {
	return h.svc.IsInventoryOwner(ctx, userID(ctx), id)
}

func inventoryResponse(inventory *domain.Inventory) publicapi.Inventory {
	return publicapi.Inventory{Id: openapitypes.UUID(inventory.ID()), Name: inventory.Name()}
}

func stockResponse(stock domain.Stock) publicapi.Stock {
	return publicapi.Stock{ProductId: openapitypes.UUID(stock.ProductID), Exists: true,
		Quantity: stock.Quantity, ReservedQuantity: stock.ReservedQuantity, Available: stock.Available()}
}

func (h handler) ListInventories(
	ctx context.Context,
	_ publicapi.ListInventoriesRequestObject,
) (publicapi.ListInventoriesResponseObject, error) {
	if userID(ctx) == uuid.Nil() {
		return publicapi.ListInventories401Response{}, nil
	}
	inventories, err := h.svc.ListOwnedInventories(ctx, userID(ctx))
	if err != nil {
		return nil, err
	}
	result := publicapi.ListInventories200JSONResponse{Inventories: make([]publicapi.Inventory, 0, len(inventories))}
	for _, inventory := range inventories {
		result.Inventories = append(result.Inventories, inventoryResponse(inventory))
	}
	return result, nil
}

func (h handler) CreateInventory(
	ctx context.Context,
	req publicapi.CreateInventoryRequestObject,
) (publicapi.CreateInventoryResponseObject, error) {
	if userID(ctx) == uuid.Nil() {
		return publicapi.CreateInventory401Response{}, nil
	}
	if req.Body == nil {
		return publicapi.CreateInventory400Response{}, nil
	}
	inventory, err := h.svc.CreateOwnedInventory(ctx, userID(ctx), req.Body.Name)
	if errors.Is(err, domain.ErrInvalidInventory) {
		return publicapi.CreateInventory400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreateInventory201JSONResponse(inventoryResponse(inventory)), nil
}

func (h handler) GetInventory(
	ctx context.Context,
	req publicapi.GetInventoryRequestObject,
) (publicapi.GetInventoryResponseObject, error) {
	id := uuid.UUID(req.InventoryId)
	owned, err := h.owns(ctx, id)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.GetInventory404Response{}, nil
	}
	inventory, err := h.svc.GetInventory(ctx, id)
	if errors.Is(err, domain.ErrInventoryNotFound) {
		return publicapi.GetInventory404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetInventory200JSONResponse(inventoryResponse(inventory)), nil
}

func (h handler) RenameInventory(
	ctx context.Context,
	req publicapi.RenameInventoryRequestObject,
) (publicapi.RenameInventoryResponseObject, error) {
	if req.Body == nil {
		return publicapi.RenameInventory400Response{}, nil
	}
	id := uuid.UUID(req.InventoryId)
	owned, err := h.owns(ctx, id)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.RenameInventory404Response{}, nil
	}
	err = h.svc.RenameInventory(ctx, id, req.Body.Name)
	if errors.Is(err, domain.ErrInvalidInventoryName) {
		return publicapi.RenameInventory400Response{}, nil
	}
	if errors.Is(err, domain.ErrInventoryNotFound) {
		return publicapi.RenameInventory404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.RenameInventory204Response{}, nil
}

func (h handler) ListStocks(
	ctx context.Context,
	req publicapi.ListStocksRequestObject,
) (publicapi.ListStocksResponseObject, error) {
	id := uuid.UUID(req.InventoryId)
	owned, err := h.owns(ctx, id)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.ListStocks404Response{}, nil
	}
	stocks, err := h.svc.ListStocks(ctx, id)
	if errors.Is(err, domain.ErrInventoryNotFound) {
		return publicapi.ListStocks404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := publicapi.ListStocks200JSONResponse{Stocks: make([]publicapi.Stock, 0, len(stocks))}
	for _, stock := range stocks {
		result.Stocks = append(result.Stocks, stockResponse(stock))
	}
	return result, nil
}

func (h handler) GetStock(
	ctx context.Context,
	req publicapi.GetStockRequestObject,
) (publicapi.GetStockResponseObject, error) {
	inventoryID, productID := uuid.UUID(req.InventoryId), uuid.UUID(req.ProductId)
	owned, err := h.owns(ctx, inventoryID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.GetStock404Response{}, nil
	}
	stock, err := h.svc.GetStock(ctx, inventoryID, productID)
	if errors.Is(err, domain.ErrInventoryNotFound) || errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.GetStock404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	if stock == nil {
		return publicapi.GetStock200JSONResponse{ProductId: req.ProductId}, nil
	}
	return publicapi.GetStock200JSONResponse(stockResponse(*stock)), nil
}

func (h handler) CreateStock(
	ctx context.Context,
	req publicapi.CreateStockRequestObject,
) (publicapi.CreateStockResponseObject, error) {
	inventoryID := uuid.UUID(req.InventoryId)
	owned, err := h.owns(ctx, inventoryID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.CreateStock404Response{}, nil
	}
	err = h.svc.CreateStock(ctx, inventoryID, uuid.UUID(req.ProductId))
	if errors.Is(err, domain.ErrStockExists) {
		return publicapi.CreateStock409Response{}, nil
	}
	if errors.Is(err, domain.ErrInventoryNotFound) || errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.CreateStock404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidStock) {
		return publicapi.CreateStock400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreateStock201Response{}, nil
}

func (h handler) AddStock(
	ctx context.Context,
	req publicapi.AddStockRequestObject,
) (publicapi.AddStockResponseObject, error) {
	if req.Body == nil {
		return publicapi.AddStock400Response{}, nil
	}
	inventoryID := uuid.UUID(req.InventoryId)
	owned, err := h.owns(ctx, inventoryID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.AddStock404Response{}, nil
	}
	err = h.svc.AddStock(ctx, inventoryID, uuid.UUID(req.ProductId), req.Body.Quantity)
	if errors.Is(err, domain.ErrInvalidStock) {
		return publicapi.AddStock400Response{}, nil
	}
	if errors.Is(err, domain.ErrInventoryNotFound) || errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.AddStock404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.AddStock204Response{}, nil
}

func (h handler) RemoveStock(
	ctx context.Context,
	req publicapi.RemoveStockRequestObject,
) (publicapi.RemoveStockResponseObject, error) {
	if req.Body == nil {
		return publicapi.RemoveStock400Response{}, nil
	}
	inventoryID := uuid.UUID(req.InventoryId)
	owned, err := h.owns(ctx, inventoryID)
	if err != nil {
		return nil, err
	}
	if !owned {
		return publicapi.RemoveStock404Response{}, nil
	}
	err = h.svc.RemoveStock(ctx, inventoryID, uuid.UUID(req.ProductId), req.Body.Quantity)
	if errors.Is(err, domain.ErrInvalidQuantityToRemove) {
		return publicapi.RemoveStock400Response{}, nil
	}
	if errors.Is(err, domain.ErrInsufficientStock) {
		return publicapi.RemoveStock409Response{}, nil
	}
	if errors.Is(err, domain.ErrInventoryNotFound) || errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.RemoveStock404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.RemoveStock204Response{}, nil
}

func (h handler) RenameProduct(
	ctx context.Context,
	req publicapi.RenameProductRequestObject,
) (publicapi.RenameProductResponseObject, error) {
	if req.Body == nil {
		return publicapi.RenameProduct400Response{}, nil
	}
	err := h.svc.RenameProduct(ctx, uuid.UUID(req.Id), req.Body.Name)
	if errors.Is(err, domain.ErrInvalidProduct) {
		return publicapi.RenameProduct400Response{}, nil
	}
	if errors.Is(err, domain.ErrProductNotFound) {
		return publicapi.RenameProduct404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.RenameProduct204Response{}, nil
}
