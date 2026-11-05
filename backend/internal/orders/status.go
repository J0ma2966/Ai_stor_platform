package orders

import (
	"errors"

	"github.com/boutique-market/backend/internal/auth"
)

// Status is the order lifecycle state. Values match the order_status DB enum.
type Status string

const (
	StatusPending    Status = "pending"
	StatusConfirmed  Status = "confirmed"
	StatusRejected   Status = "rejected"
	StatusAssembling Status = "assembling"
	StatusInDelivery Status = "in_delivery"
	StatusDelivered  Status = "delivered"
	StatusCancelled  Status = "cancelled"
)

var (
	ErrInvalidTransition = errors.New("orders: invalid status transition")
	ErrForbidden         = errors.New("orders: forbidden")
	ErrReasonRequired    = errors.New("orders: reason required")
)

// transitions is the only source of truth for allowed moves.
var transitions = map[Status][]Status{
	StatusPending:    {StatusConfirmed, StatusRejected, StatusCancelled},
	StatusConfirmed:  {StatusAssembling, StatusCancelled},
	StatusAssembling: {StatusInDelivery, StatusCancelled},
	StatusInDelivery: {StatusDelivered},
}

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusConfirmed, StatusRejected, StatusAssembling,
		StatusInDelivery, StatusDelivered, StatusCancelled:
		return true
	}
	return false
}

// Terminal reports whether no further transitions are possible.
func (s Status) Terminal() bool { return len(transitions[s]) == 0 }

// NextStatuses returns the statuses reachable from s (for UI buttons).
func NextStatuses(s Status) []Status {
	out := make([]Status, len(transitions[s]))
	copy(out, transitions[s])
	return out
}

// CheckTransition validates a status change requested by role.
// Rejection and cancellation require a non-empty reason shown to the customer.
func CheckTransition(role auth.Role, from, to Status, reason string) error {
	if role != auth.RoleStore && role != auth.RoleAdmin {
		return ErrForbidden
	}
	if !from.Valid() || !to.Valid() {
		return ErrInvalidTransition
	}
	for _, n := range transitions[from] {
		if n == to {
			if (to == StatusRejected || to == StatusCancelled) && trimmedLen(reason) == 0 {
				return ErrReasonRequired
			}
			return nil
		}
	}
	return ErrInvalidTransition
}

// CheckClientCancel — клиент может только отменить заказ и только до начала сборки.
// Причина необязательна.
func CheckClientCancel(from, to Status) error {
	if to != StatusCancelled {
		return ErrForbidden
	}
	if from != StatusPending && from != StatusConfirmed {
		return ErrInvalidTransition
	}
	return nil
}

// ClientCanCancel — показывать ли клиенту кнопку «Отменить заказ».
func ClientCanCancel(s Status) bool { return CheckClientCancel(s, StatusCancelled) == nil }

// CustomerStep maps a status to the customer-facing progress step (1..4),
// 0 for rejected/cancelled/pending-before-confirmation.
func CustomerStep(s Status) int {
	switch s {
	case StatusConfirmed:
		return 1
	case StatusAssembling:
		return 2
	case StatusInDelivery:
		return 3
	case StatusDelivered:
		return 4
	}
	return 0
}
