package center

import (
	"context"

	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

type statusServer struct {
	version string
}

func (s statusServer) GetSystemStatus(context.Context, api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
	return api.GetSystemStatus200JSONResponse{
		Service: api.IpchronicleCenter,
		Status:  api.Ok,
		Version: s.version,
	}, nil
}
