package status

import (
	"context"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
)

var (
	params = &statusParams{}
)

const (
	peerIDFlag = "peer-id"
)

type statusParams struct {
	peerID string

	peerStatus *proto.Peer
}

func (p *statusParams) getRequiredFlags() []string {
	return []string{
		peerIDFlag,
	}
}

func (p *statusParams) initPeerInfo(grpcAddress string) error {
	/* START ALPHA CHANGE 3346 -- own and close the peer status connection before final output -- */
	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return err
	}
	defer conn.Close()

	systemClient := proto.NewSystemClient(conn)

	peerStatus, err := systemClient.PeersStatus(
		context.Background(),
		&proto.PeersStatusRequest{
			Id: p.peerID,
		},
	)
	/* FINISH ALPHA CHANGE 3346 -- own and close the peer status connection before final output -- */
	if err != nil {
		return err
	}

	p.peerStatus = peerStatus

	return nil
}

func (p *statusParams) getResult() command.CommandResult {
	return &PeersStatusResult{
		ID:        p.peerStatus.Id,
		Protocols: p.peerStatus.Protocols,
		Addresses: p.peerStatus.Addrs,
	}
}
