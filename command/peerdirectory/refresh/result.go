package refresh

import (
	"bytes"
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/command/helper"
)

/* START ALPHA CHANGE 1224 -- add manual peer directory refresh output -- */
type PeerDirectoryRefreshResult struct {
	Peers []string `json:"peers"`
}

func (r *PeerDirectoryRefreshResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEER DIRECTORY REFRESHED]\n")
	buffer.WriteString(fmt.Sprintf("Number of peers: %d\n\n", len(r.Peers)))

	if len(r.Peers) > 0 {
		rows := make([]string, len(r.Peers))
		for i, peer := range r.Peers {
			rows[i] = fmt.Sprintf("Peer %d|%s", i+1, peer)
		}

		buffer.WriteString(helper.FormatKV(rows))
		buffer.WriteString("\n")
	}

	return buffer.String()
}

/* FINISH ALPHA CHANGE 1224 -- add manual peer directory refresh output -- */
