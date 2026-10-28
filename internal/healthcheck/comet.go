// Package healthcheck typically enables readiness or liveness probes within kubernetes.
// IMPORTANT: If you update this behavior, be sure to update internal/fullnode/pod_builder.go with the new
// cosmos operator image in the "healthcheck" container.
package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/bryanlabs/cosmos-operator/internal/cosmos"
	"github.com/go-logr/logr"
)

// Statuser can query the Comet status endpoint.
type Statuser interface {
	Status(ctx context.Context, rpcHost string) (cosmos.CometStatus, error)
}

type healthResponse struct {
	Address string `json:"address"`
	InSync  bool   `json:"in_sync"`
	Error   string `json:"error,omitempty"`
}

// Comet checks the CometBFT status endpoint to determine if the node is in-sync or not.
type Comet struct {
	client      Statuser
	lastStatus  int32
	logger      logr.Logger
	maxBlockAge time.Duration
	now         func() time.Time
	rpcHost     string
	timeout     time.Duration
}

func NewComet(logger logr.Logger, client Statuser, rpcHost string, timeout time.Duration) *Comet {
	return &Comet{
		client:  client,
		logger:  logger,
		now:     time.Now,
		rpcHost: rpcHost,
		timeout: timeout,
	}
}

// WithMaxBlockAge makes the check fail once the latest block is older than maxAge. A node halted at an upgrade
// height still reports catching_up=false, so without this it stays ready while serving a frozen height.
// Zero disables the check.
func (h *Comet) WithMaxBlockAge(maxAge time.Duration) *Comet {
	h.maxBlockAge = maxAge
	return h
}

// ServeHTTP implements http.Handler.
func (h *Comet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp healthResponse
	resp.Address = h.rpcHost

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	status, err := h.client.Status(ctx, h.rpcHost)
	if err != nil {
		resp.Error = err.Error()
		h.writeResponse(http.StatusServiceUnavailable, w, resp)
		return
	}

	resp.InSync = !status.Result.SyncInfo.CatchingUp
	if !resp.InSync {
		h.writeResponse(http.StatusUnprocessableEntity, w, resp)
		return
	}

	if h.maxBlockAge > 0 {
		if age := h.now().Sub(status.Result.SyncInfo.LatestBlockTime); age > h.maxBlockAge {
			resp.InSync = false
			resp.Error = fmt.Sprintf("latest block is %s old, limit %s", age.Round(time.Second), h.maxBlockAge)
			h.writeResponse(http.StatusUnprocessableEntity, w, resp)
			return
		}
	}

	h.writeResponse(http.StatusOK, w, resp)
}

func (h *Comet) writeResponse(code int, w http.ResponseWriter, resp healthResponse) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	mustJSONEncode(resp, w)
	// Only log when status code changes, so we don't spam logs.
	if atomic.SwapInt32(&h.lastStatus, int32(code)) != int32(code) {
		h.logger.Info("Health state change", "statusCode", code, "response", resp)
	}
}
