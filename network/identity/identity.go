package identity

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/network/event"

	/* START ALPHA CHANGE 2689 -- import identity RPC lifetime support -- */
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	/* FINISH ALPHA CHANGE 2689 -- import identity RPC lifetime support -- */

	"github.com/innovativebioresearch/innbc-chain/network/proto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const PeerID = "peerID"

var (
	ErrInvalidChainID   = errors.New("invalid chain ID")
	ErrNoAvailableSlots = errors.New("no available Slots")
	/* START ALPHA CHANGE 681 -- reject non-allowed validator peers during identity handshake -- */
	ErrPeerNotAllowed = errors.New("peer is not allowed")
	/* FINISH ALPHA CHANGE 681 -- reject non-allowed validator peers during identity handshake -- */
)

// networkingServer defines the base communication interface between
// any networking server implementation and the IdentityService
type networkingServer interface {
	// PROTOCOL MANIPULATION //

	/* START ALPHA CHANGE 2690 -- require cancellable identity dialing and explicit cleanup -- */
	NewIdentityClient(
		context.Context,
		peer.ID,
	) (proto.IdentityClient, func(), error)
	/* FINISH ALPHA CHANGE 2690 -- require cancellable identity dialing and explicit cleanup -- */

	// PEER MANIPULATION //

	// DisconnectFromPeer attempts to disconnect from the specified peer
	DisconnectFromPeer(peerID peer.ID, reason string)

	// AddPeer adds a peer to the networking server's peer store
	AddPeer(id peer.ID, direction network.Direction)

	/* START ALPHA CHANGE 2745 -- bind live handshakes to their transport ownership -- */
	ConnectionContext(network.Conn) (context.Context, error)
	AddPeerConnection(network.Conn) error
	DisconnectConnection(network.Conn, string)
	EmitConnectionEvent(network.Conn, event.PeerEventType)
	/* FINISH ALPHA CHANGE 2745 -- bind live handshakes to their transport ownership -- */

	/* START ALPHA CHANGE 2864 -- require owned atomic capacity reservation for live handshakes -- */
	ReserveConnectionSlot(network.Conn) (func(), bool)
	/* FINISH ALPHA CHANGE 2864 -- require owned atomic capacity reservation for live handshakes -- */

	// EmitEvent emits the specified peer event on the base networking server
	EmitEvent(event *event.PeerEvent)

	// TEMPORARY DIALING //

	// IsTemporaryDial checks if the peer connection is a temporary dial [Thread safe]
	IsTemporaryDial(peerID peer.ID) bool

	// CONNECTION INFORMATION //

	/* START ALPHA CHANGE 682 -- expose validator peer allowlist to identity service -- */
	IsPeerAllowed(peerID peer.ID) bool
	/* FINISH ALPHA CHANGE 682 -- expose validator peer allowlist to identity service -- */
}

// IdentityService is a networking service used to handle peer handshaking.
// It acts as a gatekeeper to peer connectivity
type IdentityService struct {
	proto.UnimplementedIdentityServer

	/* START ALPHA CHANGE 2745 -- retain pending ownership by connection ID -- */
	pendingPeerConnections sync.Map /* Connection ID -> pendingHandshake. */
	/* START ALPHA CHANGE 2865 -- serialize pending registration and reservation cleanup -- */
	pendingAdmissionLock sync.Mutex
	/* FINISH ALPHA CHANGE 2865 -- serialize pending registration and reservation cleanup -- */
	/* FINISH ALPHA CHANGE 2745 -- retain pending ownership by connection ID -- */
	logger     hclog.Logger     // The IdentityService logger
	baseServer networkingServer // The interface towards the base networking server

	chainID int64   // The chain ID of the network
	hostID  peer.ID // The base networking server's host peer ID

	/* START ALPHA CHANGE 2691 -- track admitted identity handshakes -- */
	rpcLifetime netgrpc.RPCLifetime
	/* FINISH ALPHA CHANGE 2691 -- track admitted identity handshakes -- */
}

// NewIdentityService returns a new instance of the IdentityService
func NewIdentityService(
	server networkingServer,
	logger hclog.Logger,
	chainID int64,
	hostID peer.ID,
) *IdentityService {
	return &IdentityService{
		logger:     logger.Named("identity"),
		baseServer: server,
		chainID:    chainID,
		hostID:     hostID,
	}
}

/* START ALPHA CHANGE 2745 -- own each handshake and its cleanup by connection ID -- */
/* START ALPHA CHANGE 2866 -- reserve capacity before handshake work and finish rejection cleanup before joining -- */
func (i *IdentityService) GetNotifyBundle() *network.NotifyBundle {
	return &network.NotifyBundle{
		ConnectedF: func(_ network.Network, conn network.Conn) {
			id := conn.ID()
			peerID := conn.RemotePeer()
			direction := conn.Stat().Direction

			if _, pending := i.pendingPeerConnections.Load(id); pending {
				return
			}

			parent, err := i.baseServer.ConnectionContext(conn)
			if err != nil {
				i.baseServer.DisconnectConnection(conn, err.Error())

				return
			}

			ctx, finish, err := i.rpcLifetime.Begin(parent)
			if err != nil {
				i.baseServer.DisconnectConnection(conn, err.Error())

				return
			}

			ctx, cancel := context.WithCancel(ctx)
			added, err := i.addPendingStatus(conn, pendingHandshake{
				peerID: peerID,
				cancel: cancel,
			})
			if !added {
				cancel()
				if err != nil {
					i.baseServer.DisconnectConnection(conn, err.Error())
				}
				finish()

				return
			}

			go func() {
				defer finish()
				defer cancel()

				eventType := event.PeerDialCompleted
				if err := i.handleConnectedContext(ctx, peerID, direction, conn); err != nil {
					i.baseServer.DisconnectConnection(conn, err.Error())
					eventType = event.PeerFailedToConnect
				}

				i.removePendingStatus(id)
				i.baseServer.EmitConnectionEvent(conn, eventType)
			}()
		},
		DisconnectedF: func(_ network.Network, conn network.Conn) {
			i.cancelPendingConnection(conn.ID())
		},
	}
}

/* FINISH ALPHA CHANGE 2866 -- reserve capacity before handshake work and finish rejection cleanup before joining -- */

/* FINISH ALPHA CHANGE 2745 -- own each handshake and its cleanup by connection ID -- */

/* START ALPHA CHANGE 2745 -- make pending cancellation and removal connection-specific -- */
/* START ALPHA CHANGE 2867 -- retain owned reservation cleanup with the pending handshake -- */
type pendingHandshake struct {
	peerID  peer.ID
	cancel  context.CancelFunc
	release func()
}

/* FINISH ALPHA CHANGE 2867 -- retain owned reservation cleanup with the pending handshake -- */

func (i *IdentityService) hasPendingStatus(id peer.ID) bool {
	found := false
	i.pendingPeerConnections.Range(func(_, value interface{}) bool {
		pending, ok := value.(pendingHandshake)
		if ok && pending.peerID == id {
			found = true

			return false
		}

		return true
	})

	return found
}

/* START ALPHA CHANGE 2868 -- coordinate duplicate suppression with owned reservation and idempotent release -- */
func (i *IdentityService) removePendingStatus(id string) {
	i.pendingAdmissionLock.Lock()
	defer i.pendingAdmissionLock.Unlock()

	if value, loaded := i.pendingPeerConnections.LoadAndDelete(id); loaded {
		if pending, ok := value.(pendingHandshake); ok {
			pending.cancel()
			if pending.release != nil {
				pending.release()
			}
		}
	}
}

func (i *IdentityService) addPendingStatus(
	conn network.Conn,
	pending pendingHandshake,
) (bool, error) {
	i.pendingAdmissionLock.Lock()
	defer i.pendingAdmissionLock.Unlock()

	id := conn.ID()
	if _, loaded := i.pendingPeerConnections.Load(id); loaded {
		return false, nil
	}

	release, reserved := i.baseServer.ReserveConnectionSlot(conn)
	if !reserved {
		return false, ErrNoAvailableSlots
	}

	pending.release = release
	i.pendingPeerConnections.Store(id, pending)

	return true, nil
}

/* FINISH ALPHA CHANGE 2868 -- coordinate duplicate suppression with owned reservation and idempotent release -- */

func (i *IdentityService) cancelPendingConnection(id string) {
	if value, loaded := i.pendingPeerConnections.Load(id); loaded {
		if pending, ok := value.(pendingHandshake); ok {
			pending.cancel()
		}
	}
}

/* FINISH ALPHA CHANGE 2745 -- make pending cancellation and removal connection-specific -- */

/* START ALPHA CHANGE 2694 -- bound identity establishment and RPC while retaining cleanup ownership -- */
func (i *IdentityService) Close() {
	i.rpcLifetime.Close()
}

func (i *IdentityService) handleConnected(
	peerID peer.ID,
	direction network.Direction,
) error {
	ctx, finish, err := i.rpcLifetime.Begin(context.Background())
	if err != nil {
		return err
	}
	defer finish()

	return i.handleConnectedContext(ctx, peerID, direction)
}

/* START ALPHA CHANGE 2745 -- supply the owned transport on live notification paths -- */
func (i *IdentityService) handleConnectedContext(
	parent context.Context,
	peerID peer.ID,
	direction network.Direction,
	connection ...network.Conn,
) error {
	/* FINISH ALPHA CHANGE 2745 -- supply the owned transport on live notification paths -- */
	ctx, cancel := context.WithTimeout(parent, netgrpc.DefaultProtocolTimeout)
	defer cancel()

	clt, closeClient, err := i.baseServer.NewIdentityClient(ctx, peerID)
	if err != nil {
		return fmt.Errorf("unable to create new identity client connection, %w", err)
	}
	defer closeClient()

	status := i.constructStatus(peerID)
	resp, err := clt.Hello(ctx, status)
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if resp == nil {
		return errors.New("identity handshake returned a nil response")
	}

	if status.Chain != resp.Chain {
		return ErrInvalidChainID
	}

	if !i.baseServer.IsPeerAllowed(peerID) {
		return ErrPeerNotAllowed
	}

	/* START ALPHA CHANGE 2745 -- atomically authorize live registration against policy publication -- */
	if !resp.TemporaryDial && !status.TemporaryDial {
		if len(connection) != 0 {
			return i.baseServer.AddPeerConnection(connection[0])
		}

		/* Retain the existing direct helper used by isolated handshake tests. */
		i.baseServer.AddPeer(peerID, direction)
	}
	/* FINISH ALPHA CHANGE 2745 -- atomically authorize live registration against policy publication -- */

	return nil
}

/* FINISH ALPHA CHANGE 2694 -- bound identity establishment and RPC while retaining cleanup ownership -- */

// Hello is the initial message that bundles peer information
// on first contact
/* START ALPHA CHANGE 2745 -- bind identity metadata to the authenticated transport -- */
func (i *IdentityService) Hello(ctx context.Context, req *proto.Status) (*proto.Status, error) {
	authenticated, ok := ctx.(*netgrpc.Context)
	if !ok || req == nil {
		return nil, ErrPeerNotAllowed
	}

	claimed, err := peer.Decode(req.Metadata[PeerID])
	if err != nil {
		return nil, err
	}
	if claimed != authenticated.PeerID ||
		!i.baseServer.IsPeerAllowed(authenticated.PeerID) {
		return nil, ErrPeerNotAllowed
	}

	return i.constructStatus(authenticated.PeerID), nil
}

/* FINISH ALPHA CHANGE 2745 -- bind identity metadata to the authenticated transport -- */

// constructStatus constructs a status response of the current node
func (i *IdentityService) constructStatus(peerID peer.ID) *proto.Status {
	return &proto.Status{
		Metadata: map[string]string{
			PeerID: i.hostID.String(),
		},
		Chain:         i.chainID,
		TemporaryDial: i.baseServer.IsTemporaryDial(peerID),
	}
}
