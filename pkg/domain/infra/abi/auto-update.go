package abi

import (
	"context"

	"github.com/containers/podman/v3/pkg/autoupdate"
	"github.com/containers/podman/v3/pkg/domain/entities"
)

func (ic *ContainerEngine) AutoUpdate(ctx context.Context, options entities.AutoUpdateOptions) ([]*entities.AutoUpdateReport, []error) {
	// Convert the entities options to the autoupdate ones.  We can't use
	// them in the entities package as low-level packages must not leak
	// into the remote client.
	autoOpts := autoupdate.Options{Authfile: options.Authfile}
	return autoupdate.AutoUpdate(ctx, ic.Libpod, autoOpts)
}
