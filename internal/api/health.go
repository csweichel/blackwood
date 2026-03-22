package api

import (
	"context"

	"connectrpc.com/connect"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
)

// Version is set via build flags (-ldflags).
var Version = "dev"

// HealthHandler implements the HealthService Connect handler.
type HealthHandler struct{}

// Check returns the current health status and build version.
func (h *HealthHandler) Check(_ context.Context, _ *connect.Request[blackwoodv1.HealthCheckRequest]) (*connect.Response[blackwoodv1.HealthCheckResponse], error) {
	return connect.NewResponse(&blackwoodv1.HealthCheckResponse{
		Status:  "ok",
		Version: Version,
	}), nil
}
