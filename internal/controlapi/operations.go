package controlapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"open-mihomo-gateway/internal/gateway"
)

const operationIDHeader = "X-OpenSurge-Operation-ID"

var errOperationExists = errors.New("operation id is already in use")

func validOperationID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func newOperation(id, kind string) Operation {
	now := time.Now().UTC()
	return Operation{
		SchemaVersion: SchemaVersion, ID: id, Kind: kind, State: "running",
		Phase: "waiting_helper", PhaseStartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

// Request operations keep the existing synchronous configuration API contract.
// The optional correlation ID lets the initiating UI read real progress while
// its PUT/POST is still pending; it is not permission to replay a mutation.
func (s *Server) beginRequestOperation(w http.ResponseWriter, r *http.Request, kind string) (context.Context, *Operation, bool) {
	id := r.Header.Get(operationIDHeader)
	if id == "" {
		id = randomToken(12)
	}
	if !validOperationID(id) {
		writeError(w, http.StatusBadRequest, "invalid_operation_id", "invalid operation id")
		return nil, nil, false
	}
	op := newOperation(id, kind)
	if err := s.store.CreateOperation(op); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errOperationExists) {
			status = http.StatusConflict
		}
		writeError(w, status, "operation_failed", err.Error())
		return nil, nil, false
	}
	w.Header().Set(operationIDHeader, id)
	return s.observeOperation(r.Context(), &op), &op, true
}

func (s *Server) observeOperation(ctx context.Context, op *Operation) context.Context {
	return gateway.WithProgress(ctx, func(progress gateway.Progress) {
		if op.State != "running" {
			return
		}
		now := time.Now().UTC()
		changed := false
		if progress.Phase != "" && progress.Phase != op.Phase {
			op.Phase = progress.Phase
			op.PhaseStartedAt = now
			changed = true
		}
		if progress.Notice != "" && !slices.Contains(op.Notices, progress.Notice) {
			op.Notices = append(op.Notices, progress.Notice)
			changed = true
		}
		if changed {
			op.UpdatedAt = now
			// Progress is observability, not another lifecycle precondition.
			_ = s.store.SaveOperation(*op)
		}
	})
}

func (s *Server) finishOperation(op *Operation, err error) {
	op.UpdatedAt = time.Now().UTC()
	op.State = "succeeded"
	if err != nil {
		op.State = "failed"
		op.Error = err.Error()
	}
	_ = s.store.SaveOperation(*op)
}
