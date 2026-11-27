package events

import (
	"context"
	"log/slog"
	"testing"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"audit/internal/audit/app"
	"audit/internal/audit/domain"
	inventoryevents "inventory/api/events"
)

type captureRecorder struct{ input app.RecordAuditEntryInput }

func (r *captureRecorder) RecordAuditEntry(
	_ context.Context,
	input app.RecordAuditEntryInput,
) (*domain.AuditEntry, error) {
	r.input = input
	return nil, nil
}

func TestInventoryEvents(t *testing.T) {
	actor, inventoryID, productID := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		eventType, action, target string
		message                   proto.Message
	}{
		{
			"InventoryCreated",
			"inventory.created",
			inventoryID.String(),
			&inventoryevents.InventoryCreated{
				InventoryId: inventoryID.String(),
				ActorId:     actor.String(),
				Name:        "Холодильник",
			},
		},
		{
			"InventoryRenamed",
			"inventory.renamed",
			inventoryID.String(),
			&inventoryevents.InventoryRenamed{InventoryId: inventoryID.String(), ActorId: actor.String(), Name: "Шкаф"},
		},
		{
			"ProductCreated",
			"product.created",
			productID.String(),
			&inventoryevents.ProductCreated{
				ProductId: productID.String(),
				ActorId:   actor.String(),
				Name:      "Греча",
				Unit:      "g",
			},
		},
		{
			"ProductRenamed",
			"product.renamed",
			productID.String(),
			&inventoryevents.ProductRenamed{ProductId: productID.String(), ActorId: actor.String(), Name: "Крупа"},
		},
		{
			"StockCreated",
			"stock.created",
			inventoryID.String(),
			&inventoryevents.StockCreated{
				InventoryId: inventoryID.String(),
				ProductId:   productID.String(),
				ActorId:     actor.String(),
			},
		},
		{
			"StockAdjusted",
			"stock.adjusted",
			inventoryID.String(),
			&inventoryevents.StockAdjusted{
				InventoryId:   inventoryID.String(),
				ProductId:     productID.String(),
				ActorId:       actor.String(),
				QuantityDelta: -5,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.eventType, func(t *testing.T) {
			envelope, err := event.New(t.Context(), "inventory", test.eventType, test.message)
			require.NoError(t, err)
			recorder := &captureRecorder{}
			err = NewHandler(slog.Default(), recorder).Handle(t.Context(), envelope)
			require.NoError(t, err)
			require.Equal(t, test.action, recorder.input.Action)
			require.Equal(t, test.target, recorder.input.TargetID)
			require.Equal(t, actor, *recorder.input.ActorID)
			require.Equal(t, envelope.ID(), recorder.input.EventID)
			require.Equal(t, envelope.CorrelationID(), recorder.input.CorrelationID)
		})
	}
}
