package app

import (
	"context"
	"fmt"
	"uuid"

	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

// The same context must be used by nested units of work so Inbox, business data
// and Outbox can participate in one transaction.
func (s *Service) CreateInventory(ctx context.Context, name string) (*domain.Inventory, error) {
	inventory, err := domain.NewInventory(uuid.New(), name)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := p.Inventory(ctx).Create(inventory); err != nil {
			return err
		}
		return s.dispatch(ctx, &inventoryevents.InventoryCreated{
			InventoryId: inventory.ID().String(), Name: inventory.Name(), ActorId: actorID(ctx),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create inventory: %w", err)
	}
	return inventory, nil
}

func (s *Service) GetInventory(ctx context.Context, id uuid.UUID) (*domain.Inventory, error) {
	var inventory *domain.Inventory
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		inventory, err = p.Inventory(ctx).GetByID(id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get inventory: %w", err)
	}
	if inventory == nil {
		return nil, domain.ErrInventoryNotFound
	}
	return inventory, nil
}

func (s *Service) ListInventories(ctx context.Context) ([]*domain.Inventory, error) {
	var inventories []*domain.Inventory
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		inventories, err = p.Inventory(ctx).List()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list inventories: %w", err)
	}
	if inventories == nil {
		inventories = []*domain.Inventory{}
	}
	return inventories, nil
}

func (s *Service) RenameInventory(ctx context.Context, id uuid.UUID, name string) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		inventory, err := p.Inventory(ctx).GetByID(id)
		if err != nil {
			return err
		}
		if inventory == nil {
			return domain.ErrInventoryNotFound
		}
		if inventory.Name() == name {
			return nil
		}
		if err := inventory.Rename(name); err != nil {
			return err
		}
		if err := p.Inventory(ctx).Update(inventory); err != nil {
			return err
		}
		return s.dispatch(
			ctx,
			&inventoryevents.InventoryRenamed{InventoryId: id.String(), Name: name, ActorId: actorID(ctx)},
		)
	})
	if err != nil {
		return fmt.Errorf("rename inventory: %w", err)
	}
	return nil
}

func (s *Service) CreateProduct(ctx context.Context, name string, unit domain.Unit) (*domain.Product, error) {
	product, err := domain.NewProduct(uuid.New(), name, unit)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := p.Product(ctx).Create(&product); err != nil {
			return err
		}
		return s.dispatch(ctx, &inventoryevents.ProductCreated{
			ProductId: product.ID().String(), Name: product.Name(), Unit: string(product.Unit()), ActorId: actorID(ctx),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}
	return &product, nil
}

func (s *Service) ListProducts(ctx context.Context, filter domain.ProductFilter) ([]*domain.Product, error) {
	if filter.Limit < 0 || filter.Limit > 100 || filter.Offset < 0 {
		return nil, domain.ErrInvalidProductFilter
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	var products []*domain.Product
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		products, err = p.Product(ctx).List(filter)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	if products == nil {
		products = []*domain.Product{}
	}
	return products, nil
}

func (s *Service) GetProduct(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	var product *domain.Product
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		product, err = p.Product(ctx).GetByID(id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if product == nil {
		return nil, domain.ErrProductNotFound
	}
	return product, nil
}

func (s *Service) RenameProduct(ctx context.Context, id uuid.UUID, name string) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		return s.renameProduct(ctx, p, id, name)
	})
	if err != nil {
		return fmt.Errorf("rename product: %w", err)
	}
	return nil
}

func (s *Service) renameProduct(ctx context.Context, p *mysql.RepositoryProvider, id uuid.UUID, name string) error {
	product, err := p.Product(ctx).GetByID(id)
	if err != nil {
		return err
	}
	if product == nil {
		return domain.ErrProductNotFound
	}
	if product.Name() == name {
		return nil
	}
	if err := product.Rename(name); err != nil {
		return err
	}
	if err := p.Product(ctx).Update(product); err != nil {
		return err
	}
	return s.dispatch(ctx, &inventoryevents.ProductRenamed{ProductId: id.String(), Name: name, ActorId: actorID(ctx)})
}

func requireResources(ctx context.Context, p *mysql.RepositoryProvider, inventoryID, productID uuid.UUID) error {
	inventory, err := p.Inventory(ctx).GetByID(inventoryID)
	if err != nil {
		return err
	}
	if inventory == nil {
		return domain.ErrInventoryNotFound
	}
	product, err := p.Product(ctx).GetByID(productID)
	if err != nil {
		return err
	}
	if product == nil {
		return domain.ErrProductNotFound
	}
	return nil
}

// GetStock returns nil for an absent stock, distinct from a persisted zero stock.
func (s *Service) GetStock(ctx context.Context, inventoryID, productID uuid.UUID) (*domain.Stock, error) {
	var stock *domain.Stock
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireResources(ctx, p, inventoryID, productID); err != nil {
			return err
		}
		var err error
		stock, err = p.Stock(ctx).GetByProductID(inventoryID, productID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get stock: %w", err)
	}
	return stock, nil
}

func (s *Service) ListStocks(ctx context.Context, inventoryID uuid.UUID) ([]domain.Stock, error) {
	var stocks []domain.Stock
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		inventory, err := p.Inventory(ctx).GetByID(inventoryID)
		if err != nil {
			return err
		}
		if inventory == nil {
			return domain.ErrInventoryNotFound
		}
		stocks, err = p.Stock(ctx).List(inventoryID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list stocks: %w", err)
	}
	if stocks == nil {
		stocks = []domain.Stock{}
	}
	return stocks, nil
}

func (s *Service) AddStock(ctx context.Context, inventoryID, productID uuid.UUID, quantity int64) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireResources(ctx, p, inventoryID, productID); err != nil {
			return err
		}
		if err := p.Stock(ctx).Add(inventoryID, productID, quantity); err != nil {
			return err
		}
		return s.dispatch(ctx, &inventoryevents.StockAdjusted{
			InventoryId:   inventoryID.String(),
			ProductId:     productID.String(),
			QuantityDelta: quantity,
			ActorId:       actorID(ctx),
		})
	})
	if err != nil {
		return fmt.Errorf("add stock: %w", err)
	}
	return nil
}

func (s *Service) CreateStock(ctx context.Context, inventoryID, productID uuid.UUID) error {
	stock, err := domain.NewStock(inventoryID, productID, 0, 0)
	if err != nil {
		return err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireResources(ctx, p, inventoryID, productID); err != nil {
			return err
		}
		if err := p.Stock(ctx).Create(stock); err != nil {
			return err
		}
		return s.dispatch(ctx, &inventoryevents.StockCreated{
			InventoryId: inventoryID.String(), ProductId: productID.String(), ActorId: actorID(ctx),
		})
	})
	if err != nil {
		return fmt.Errorf("create stock: %w", err)
	}
	return nil
}

func (s *Service) RemoveStock(ctx context.Context, inventoryID, productID uuid.UUID, quantity int64) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireResources(ctx, p, inventoryID, productID); err != nil {
			return err
		}
		if err := p.Stock(ctx).Remove(inventoryID, productID, quantity); err != nil {
			return err
		}
		return s.dispatch(ctx, &inventoryevents.StockAdjusted{
			InventoryId:   inventoryID.String(),
			ProductId:     productID.String(),
			QuantityDelta: -quantity,
			ActorId:       actorID(ctx),
		})
	})
	if err != nil {
		return fmt.Errorf("remove stock: %w", err)
	}
	return nil
}

func (s *Service) Reserve(ctx context.Context, inventoryID, id uuid.UUID, items []domain.ReservationItem) error {
	reservation, err := domain.NewReservation(id, inventoryID, items)
	if err != nil {
		return err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireInventory(ctx, p, inventoryID); err != nil {
			return err
		}
		return p.Reservation(ctx).Reserve(reservation)
	})
	if err != nil {
		return fmt.Errorf("reserve: %w", err)
	}
	return nil
}

func (s *Service) Release(ctx context.Context, inventoryID, id uuid.UUID) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireInventory(ctx, p, inventoryID); err != nil {
			return err
		}
		return p.Reservation(ctx).Release(inventoryID, id)
	})
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	return nil
}

func (s *Service) Consume(ctx context.Context, inventoryID, id uuid.UUID) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireInventory(ctx, p, inventoryID); err != nil {
			return err
		}
		return p.Reservation(ctx).Consume(inventoryID, id)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	return nil
}

func requireInventory(ctx context.Context, p *mysql.RepositoryProvider, inventoryID uuid.UUID) error {
	inventory, err := p.Inventory(ctx).GetByID(inventoryID)
	if err != nil {
		return err
	}
	if inventory == nil {
		return domain.ErrInventoryNotFound
	}
	return nil
}

// CreateOwnedInventory stores the resource and its creator in the same transaction.
func (s *Service) CreateOwnedInventory(ctx context.Context, userID uuid.UUID, name string) (*domain.Inventory, error) {
	if userID == uuid.Nil() {
		return nil, domain.ErrInvalidInventory
	}
	inventory, err := domain.NewInventory(uuid.New(), name)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := p.Inventory(ctx).Create(inventory); err != nil {
			return err
		}
		if err := p.InventoryOwnership(ctx).Create(inventory.ID(), userID); err != nil {
			return err
		}
		return s.dispatch(ctx, &inventoryevents.InventoryCreated{
			InventoryId: inventory.ID().String(), Name: inventory.Name(), ActorId: userID.String(),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create owned inventory: %w", err)
	}
	return inventory, nil
}

func (s *Service) ListOwnedInventories(ctx context.Context, userID uuid.UUID) ([]*domain.Inventory, error) {
	if userID == uuid.Nil() {
		return nil, domain.ErrInvalidInventory
	}
	var inventories []*domain.Inventory
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		inventories, err = p.InventoryOwnership(ctx).ListByUser(userID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list owned inventories: %w", err)
	}
	return inventories, nil
}

func (s *Service) IsInventoryOwner(ctx context.Context, userID, inventoryID uuid.UUID) (bool, error) {
	var owner bool
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		owner, err = p.InventoryOwnership(ctx).IsOwner(inventoryID, userID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("check inventory owner: %w", err)
	}
	return owner, nil
}
