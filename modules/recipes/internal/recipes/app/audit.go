package app

import (
	auditevents "audit/api/events"
	"context"
	"encoding/json"
	"fmt"
	recipesevents "recipes/api/events"

	"google.golang.org/protobuf/proto"
)

func (s *Service) dispatch(ctx context.Context, message proto.Message) error {
	record := &auditevents.AuditRecordRequested{TargetType: "recipe"}
	metadata := map[string]any{}
	switch v := message.(type) {
	case *recipesevents.RecipeCreated:
		record.Action, record.TargetId, record.ActorId = "recipe.created", v.RecipeId, v.ActorId
		metadata["name"] = v.Name
	case *recipesevents.RecipeRevised:
		record.Action, record.TargetId, record.ActorId = "recipe.revised", v.RecipeId, v.ActorId
		metadata["name"] = v.Name
	case *recipesevents.RecipeArchived:
		record.Action, record.TargetId, record.ActorId = "recipe.archived", v.RecipeId, v.ActorId
	default:
		return fmt.Errorf("no recipes audit policy for %T", message)
	}
	var err error
	record.MetadataJson, err = json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode recipe audit metadata: %w", err)
	}
	dispatcher := s.dispatcherFactory.NewDispatcher(ctx)
	if err := dispatcher.Dispatch(message); err != nil {
		return err
	}
	return dispatcher.Dispatch(record)
}
