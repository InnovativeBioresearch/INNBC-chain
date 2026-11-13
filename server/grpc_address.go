package server

import (
	"fmt"
	"net"
)

/* START ALPHA CHANGE 2561 -- require a resolved loopback operator listener --

Validate the resolved IP rather than its hostname or textual spelling.
This includes IPv4 loopback addresses and IPv6 ::1.

FINISH ALPHA CHANGE 2561 -- require a resolved loopback operator listener -- */

func validateOperatorGRPCAddress(addr *net.TCPAddr) error {
	if addr == nil {
		return fmt.Errorf("operator gRPC address is missing; a loopback address is required")
	}

	if !addr.IP.IsLoopback() {
		return fmt.Errorf(
			"operator gRPC address %q must resolve to a loopback IP; wildcard, LAN, and public addresses are not allowed",
			addr.String(),
		)
	}

	return nil
}
