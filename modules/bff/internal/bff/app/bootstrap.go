package app

import (
	"context"
	"errors"
	"sort"
	"uuid"
)

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrUnavailable     = errors.New("authentication dependency unavailable")
)

type SessionResolver interface {
	Resolve(context.Context, string) (uuid.UUID, error)
}

type ModuleAccess interface {
	List(context.Context, string, uuid.UUID) ([]string, bool, error)
}

type Bootstrap struct {
	UserID        uuid.UUID
	Authenticated bool
	Modules       []string
	Superuser     bool
}

type NavigationItem struct {
	ID    string
	Label string
	Path  string
}

type BootstrapService struct {
	sessions SessionResolver
	access   ModuleAccess
}

func NewBootstrapService(sessions SessionResolver, access ModuleAccess) *BootstrapService {
	return &BootstrapService{sessions: sessions, access: access}
}

func (s *BootstrapService) GetBootstrap(ctx context.Context, token string) (Bootstrap, error) {
	if token == "" {
		return Bootstrap{}, ErrUnauthenticated
	}
	userID, err := s.sessions.Resolve(ctx, token)
	if err != nil {
		return Bootstrap{}, err
	}
	if userID == uuid.Nil() {
		return Bootstrap{}, ErrUnauthenticated
	}
	modules, superuser, err := s.access.List(ctx, token, userID)
	if err != nil {
		return Bootstrap{}, err
	}
	sort.Strings(modules)
	return Bootstrap{UserID: userID, Authenticated: true, Modules: modules, Superuser: superuser}, nil
}

func (s *BootstrapService) GetNavigation(ctx context.Context, token string) ([]NavigationItem, error) {
	bootstrap, err := s.GetBootstrap(ctx, token)
	if err != nil {
		return nil, err
	}
	items := make([]NavigationItem, 0, len(bootstrap.Modules)+1)
	if bootstrap.Superuser {
		bootstrap.Modules = make([]string, 0, len(moduleNavigation))
		for module := range moduleNavigation {
			bootstrap.Modules = append(bootstrap.Modules, module)
		}
		sort.Strings(bootstrap.Modules)
	}
	for _, module := range bootstrap.Modules {
		if item, ok := moduleNavigation[module]; ok {
			items = append(items, item)
		}
	}
	if bootstrap.Superuser {
		items = append(items, NavigationItem{ID: "admin", Label: "Администрирование", Path: "/admin"})
	}
	return items, nil
}

var moduleNavigation = map[string]NavigationItem{
	"inventory":   {ID: "inventory", Label: "Инвентари", Path: "/inventory"},
	"mealplanner": {ID: "mealplanner", Label: "План питания", Path: "/meal-planner"},
	"recipes":     {ID: "recipes", Label: "Рецепты", Path: "/recipes"},
	"shopping":    {ID: "shopping", Label: "Покупки", Path: "/shopping"},
}
