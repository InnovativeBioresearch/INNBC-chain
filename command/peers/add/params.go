package add

import (
	"context"
	"errors"

	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/command/helper"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
)

var (
	params = &addParams{
		addedPeers: make([]string, 0),
		addErrors:  make([]string, 0),
	}
)

var (
	errInvalidAddresses = errors.New("at least 1 peer address is required")
)

const (
	addrFlag = "addr"
)

type addParams struct {
	peerAddresses []string

	systemClient proto.SystemClient

	addedPeers []string
	addErrors  []string
}

func (p *addParams) getRequiredFlags() []string {
	return []string{
		addrFlag,
	}
}

func (p *addParams) validateFlags() error {
	if len(p.peerAddresses) < 1 {
		return errInvalidAddresses
	}

	return nil
}

/* START ALPHA CHANGE 3348 -- reset peer addition results and own the connection for each invocation -- */
func (p *addParams) addPeersAt(grpcAddress string) error {
	p.addedPeers = make([]string, 0)
	p.addErrors = make([]string, 0)
	p.systemClient = nil

	conn, err := helper.GetGRPCConnection(grpcAddress)
	if err != nil {
		return err
	}
	defer func() {
		p.systemClient = nil
		_ = conn.Close()
	}()

	p.systemClient = proto.NewSystemClient(conn)
	p.addPeers()

	return nil
}

/* FINISH ALPHA CHANGE 3348 -- reset peer addition results and own the connection for each invocation -- */

func (p *addParams) addPeers() {
	for _, address := range p.peerAddresses {
		if addErr := p.addPeer(address); addErr != nil {
			p.addErrors = append(p.addErrors, addErr.Error())

			continue
		}

		p.addedPeers = append(p.addedPeers, address)
	}
}

func (p *addParams) addPeer(peerAddress string) error {
	if _, err := p.systemClient.PeersAdd(
		context.Background(),
		&proto.PeersAddRequest{
			Id: peerAddress,
		},
	); err != nil {
		return err
	}

	return nil
}

func (p *addParams) getResult() command.CommandResult {
	return &PeersAddResult{
		NumRequested: len(p.peerAddresses),
		NumAdded:     len(p.addedPeers),
		Peers:        p.addedPeers,
		Errors:       p.addErrors,
	}
}
