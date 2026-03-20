package cli

import (
	"fmt"
	"net"

	"github.com/mdlayher/vsock"
)

// vsockListen creates a vsock listener on the given port.
// Returns a net.Listener for use with VsockListener.Start().
func vsockListen(port uint32) (net.Listener, error) {
	ln, err := vsock.Listen(port, nil)
	if err != nil {
		return nil, fmt.Errorf("vsock listen on port %d: %w", port, err)
	}
	return ln, nil
}
