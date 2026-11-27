package app

import (
	auditevents "audit/api/events"
	"encoding/json"
	"fmt"
	inventoryevents "inventory/api/events"

	"google.golang.org/protobuf/proto"
)

const (
	auditInventoryTarget   = "inventory"
	auditReservationTarget = "reservation"
)

// auditRecord is owned by Inventory: it chooses which domain operations to audit.
func auditRecord(message proto.Message) (*auditevents.AuditRecordRequested, error) {
	record := &auditevents.AuditRecordRequested{}
	metadata := map[string]any{}
	switch v := message.(type) {
	case *inventoryevents.InventoryCreated:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "inventory.created", auditInventoryTarget, v.InventoryId, v.ActorId
		metadata["name"] = v.Name
	case *inventoryevents.InventoryRenamed:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "inventory.renamed", auditInventoryTarget, v.InventoryId, v.ActorId
		metadata["name"] = v.Name
	case *inventoryevents.ProductCreated:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "product.created", "product", v.ProductId, v.ActorId
		metadata["name"], metadata["unit"] = v.Name, v.Unit
	case *inventoryevents.ProductRenamed:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "product.renamed", "product", v.ProductId, v.ActorId
		metadata["name"] = v.Name
	case *inventoryevents.StockCreated:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "stock.created", auditInventoryTarget, v.InventoryId, v.ActorId
		metadata["productId"] = v.ProductId
	case *inventoryevents.StockAdjusted:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "stock.adjusted", auditInventoryTarget, v.InventoryId, v.ActorId
		metadata["productId"], metadata["quantityDelta"] = v.ProductId, v.QuantityDelta
	case *inventoryevents.ReservationCreated:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "reservation.created", auditReservationTarget, v.ReservationId, v.ActorId
		metadata["inventoryId"], metadata["itemCount"] = v.InventoryId, len(v.Items)
	case *inventoryevents.ReservationCancelled:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "reservation.cancelled", auditReservationTarget, v.ReservationId, v.ActorId
		metadata["inventoryId"] = v.InventoryId
	case *inventoryevents.ReservationConsumed:
		record.Action, record.TargetType, record.TargetId, record.ActorId = "reservation.consumed", auditReservationTarget, v.ReservationId, v.ActorId
		metadata["inventoryId"] = v.InventoryId
	default:
		return nil, fmt.Errorf("no inventory audit policy for %T", message)
	}
	var err error
	record.MetadataJson, err = json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode inventory audit metadata: %w", err)
	}
	return record, nil
}
