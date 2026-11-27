package app

import (
	"context"
	"fmt"
	"uuid"

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
		return p.Inventory(ctx).Create(inventory)
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
		if err := inventory.Rename(name); err != nil {
			return err
		}
		return p.Inventory(ctx).Update(inventory)
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
		return p.Product(ctx).Create(&product)
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
		product, err := p.Product(ctx).GetByID(id)
		if err != nil {
			return err
		}
		if product == nil {
			return domain.ErrProductNotFound
		}
		if err := product.Rename(name); err != nil {
			return err
		}
		return p.Product(ctx).Update(product)
	})
	if err != nil {
		return fmt.Errorf("rename product: %w", err)
	}
	return nil
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
		return p.Stock(ctx).Add(inventoryID, productID, quantity)
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
		return p.Stock(ctx).Create(stock)
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
		return p.Stock(ctx).Remove(inventoryID, productID, quantity)
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
		return p.Reservation(ctx).Reserve(reservation)
	})
	if err != nil {
		return fmt.Errorf("reserve: %w", err)
	}
	return nil
}

func (s *Service) Release(ctx context.Context, inventoryID, id uuid.UUID) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		return p.Reservation(ctx).Release(inventoryID, id)
	})
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	return nil
}

func (s *Service) Consume(ctx context.Context, inventoryID, id uuid.UUID) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		return p.Reservation(ctx).Consume(inventoryID, id)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	return nil
}
