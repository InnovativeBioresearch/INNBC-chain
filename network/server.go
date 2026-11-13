package network

import (
	"context"
	/* START ALPHA CHANGE 2678 -- import protocol lifecycle and cleanup support -- */
	"errors"
	/* FINISH ALPHA CHANGE 2678 -- import protocol lifecycle and cleanup support -- */
	"fmt"
	/* START ALPHA CHANGE 2678 -- import protocol lifecycle and cleanup support -- */
	"reflect"
	/* FINISH ALPHA CHANGE 2678 -- import protocol lifecycle and cleanup support -- */
	"sync"
	"time"

	"github.com/armon/go-metrics"
	"github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/network/dial"
	"github.com/innovativebioresearch/innbc-chain/network/discovery"

	/* START ALPHA CHANGE 2678 -- import protocol lifecycle and cleanup support -- */
	netgrpc "github.com/innovativebioresearch/innbc-chain/network/grpc"
	"github.com/innovativebioresearch/innbc-chain/network/identity"

	/* FINISH ALPHA CHANGE 2678 -- import protocol lifecycle and cleanup support -- */
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	rawGrpc "google.golang.org/grpc"

	"github.com/hashicorp/go-hclog"
	peerEvent "github.com/innovativebioresearch/innbc-chain/network/event"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

const (
	// peerOutboundBufferSize is the size of outbound messages to a peer buffers in go-libp2p-pubsub
	// we should have enough capacity of the queue
	// because we start dropping messages to a peer if the outbound queue is full
	peerOutboundBufferSize = 1024

	// validateBufferSize is the size of validate buffers in go-libp2p-pubsub
	// we should have enough capacity of the queue
	// because when queue is full, validation is throttled and new messages are dropped.
	validateBufferSize = 1024

	/* START ALPHA CHANGE 2895 -- define the interim complete GossipSub RPC limit -- */
	/* This limit applies to complete encoded GossipSub RPCs across all topics. */
	maxGossipMessageSize = 4 * 1024 * 1024
	/* FINISH ALPHA CHANGE 2895 -- define the interim complete GossipSub RPC limit -- */

	// networkMetrics is a prefix used for network-related metrics
	networkMetrics = "network"
)

const (
	defaultBucketSize = 20
	DefaultDialRatio  = 0.2

	DefaultLibp2pPort int = 1478

	/* START ALPHA CHANGE 1225 -- keep peer directory endpoint without background refresh interval -- */
	DefaultPeerDirectoryURL = "https://innbcchain.io/api/v1/peers/exchange"
	/* FINISH ALPHA CHANGE 1225 -- keep peer directory endpoint without background refresh interval -- */

	MinimumPeerConnections int64 = 1
)

/* START ALPHA CHANGE 650 -- remove legacy bootnode startup errors -- */
/* FINISH ALPHA CHANGE 650 -- remove legacy bootnode startup errors -- */

type Server struct {
	logger hclog.Logger // the logger
	config *Config      // the base networking server configuration

	closeCh chan struct{} // the channel used for closing the networking server

	/* START ALPHA CHANGE 2679 -- retain admitted networking work and identity service ownership -- */
	rpcLifetime     netgrpc.RPCLifetime
	identityService *identity.IdentityService
	/* FINISH ALPHA CHANGE 2679 -- retain admitted networking work and identity service ownership -- */

	/* START ALPHA CHANGE 1134 -- make network server close idempotent -- */
	closeOnce sync.Once
	closeErr  error
	/* FINISH ALPHA CHANGE 1134 -- make network server close idempotent -- */

	host  host.Host             // the libp2p host reference
	addrs []multiaddr.Multiaddr // the list of supported (bound) addresses

	peers     map[peer.ID]*PeerConnInfo // map of all peer connections
	peersLock sync.Mutex                // lock for the peer map

	/* START ALPHA CHANGE 677 -- add validator peer allowlist state -- */
	allowedPeers       map[peer.ID]struct{}
	allowedPeersLock   sync.RWMutex
	peerAllowlistReady bool
	/* FINISH ALPHA CHANGE 677 -- add validator peer allowlist state -- */

	/* START ALPHA CHANGE 1367 -- serialize peer directory client refreshes -- */
	peerDirectoryRefreshLock sync.Mutex
	/* FINISH ALPHA CHANGE 1367 -- serialize peer directory client refreshes -- */

	dialQueue *dial.DialQueue // queue used to asynchronously connect to peers

	discovery *discovery.DiscoveryService // service used for discovering other peers

	protocols     map[string]Protocol // supported protocols
	protocolsLock sync.Mutex          // lock for the supported protocols map

	secretsManager secrets.SecretsManager // secrets manager for networking keys

	/* START ALPHA CHANGE 694 -- retain libp2p key for peer directory signatures -- */
	networkKey crypto.PrivKey
	/* FINISH ALPHA CHANGE 694 -- retain libp2p key for peer directory signatures -- */

	ps *pubsub.PubSub // reference to the networking PubSub service

	/* START ALPHA CHANGE 2887 -- retain server-owned GossipSub cancellation -- */
	gossipCancel context.CancelFunc
	/* FINISH ALPHA CHANGE 2887 -- retain server-owned GossipSub cancellation -- */

	emitterPeerEvent event.Emitter // event emitter for listeners

	connectionCounts *ConnectionInfo

	/* START ALPHA CHANGE 2743 -- retain the policy created before the listening host -- */
	admission *peerAdmission
	/* FINISH ALPHA CHANGE 2743 -- retain the policy created before the listening host -- */

	temporaryDials sync.Map // map of temporary connections; peerID -> bool

	/* START ALPHA CHANGE 642 -- remove legacy bootnode wrapper from network server -- */
	/* FINISH ALPHA CHANGE 642 -- remove legacy bootnode wrapper from network server -- */
}

// NewServer returns a new instance of the networking server
/* START ALPHA CHANGE 2888 -- own GossipSub lifetime and release resources when construction fails -- */
func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	return newServerWithGossipFactories(
		logger,
		config,
		func(h host.Host) (event.Emitter, error) {
			return h.EventBus().Emitter(new(peerEvent.PeerEvent))
		},
		pubsub.NewGossipSub,
	)
}

/*
Constructor-local factories permit controlled failure tests without changing
global constructors or the production peer-admission policy.
*/
func newServerWithGossipFactories(
	logger hclog.Logger,
	config *Config,
	newEmitter func(host.Host) (event.Emitter, error),
	newGossip func(context.Context, host.Host, ...pubsub.Option) (*pubsub.PubSub, error),
) (*Server, error) {
	/* START ALPHA CHANGE 2522 -- reject invalid peer limits before network construction -- */
	if err := config.ValidatePeerLimits(); err != nil {
		return nil, fmt.Errorf("invalid network configuration: %w", err)
	}
	/* FINISH ALPHA CHANGE 2522 -- reject invalid peer limits before network construction -- */

	logger = logger.Named("network")

	key, err := setupLibp2pKey(config.SecretsManager)
	if err != nil {
		return nil, err
	}

	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
	}

	addrsFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if config.NatAddr != nil {
			addr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.NatAddr.String(), config.Addr.Port))

			if addr != nil {
				addrs = []multiaddr.Multiaddr{addr}
			}
		} else if config.DNS != nil {
			addrs = []multiaddr.Multiaddr{config.DNS}
		}

		return addrs
	}

	/* START ALPHA CHANGE 2743 -- gate connections from host construction -- */
	admission := newPeerAdmission(peerDirectoryRestricted(config))

	host, err := libp2p.New(
		// Use noise as the encryption protocol
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrs(listenAddr),
		libp2p.AddrsFactory(addrsFactory),
		libp2p.Identity(key),
		libp2p.ConnectionGater(admissionGater{policy: admission}),
	)
	/* FINISH ALPHA CHANGE 2743 -- gate connections from host construction -- */
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p stack: %w", err)
	}

	emitter, err := newEmitter(host)
	if err != nil {
		return nil, errors.Join(err, host.Close())
	}

	srv := &Server{
		logger: logger,
		config: config,
		/* START ALPHA CHANGE 2743 -- share the host admission policy -- */
		admission: admission,
		/* FINISH ALPHA CHANGE 2743 -- share the host admission policy -- */
		host:  host,
		addrs: host.Addrs(),
		peers: make(map[peer.ID]*PeerConnInfo),
		/* START ALPHA CHANGE 678 -- initialize validator peer allowlist -- */
		allowedPeers: make(map[peer.ID]struct{}),
		/* FINISH ALPHA CHANGE 678 -- initialize validator peer allowlist -- */
		dialQueue:        dial.NewDialQueue(),
		closeCh:          make(chan struct{}),
		emitterPeerEvent: emitter,
		protocols:        map[string]Protocol{},
		secretsManager:   config.SecretsManager,
		/* START ALPHA CHANGE 695 -- store libp2p key for peer directory exchange -- */
		networkKey: key,
		/* FINISH ALPHA CHANGE 695 -- store libp2p key for peer directory exchange -- */
		/* START ALPHA CHANGE 643 -- stop initializing legacy bootnode wrapper -- */
		/* FINISH ALPHA CHANGE 643 -- stop initializing legacy bootnode wrapper -- */
		connectionCounts: NewBlankConnectionInfo(
			config.MaxInboundPeers,
			config.MaxOutboundPeers,
		),
	}

	gossipCtx, gossipCancel := context.WithCancel(context.Background())

	/* START ALPHA CHANGE 2743 -- enforce membership on pubsub routing ingress and signed messages -- */
	ps, err := newGossip(
		gossipCtx,
		host, pubsub.WithPeerOutboundQueueSize(peerOutboundBufferSize),
		pubsub.WithValidateQueueSize(validateBufferSize),
		/* START ALPHA CHANGE 2896 -- configure the interim GossipSub limit for every network server -- */
		pubsub.WithMaxMessageSize(maxGossipMessageSize),
		/* FINISH ALPHA CHANGE 2896 -- configure the interim GossipSub limit for every network server -- */
		pubsub.WithPeerFilter(func(id peer.ID, _ string) bool {
			return srv.IsPeerAllowed(id)
		}),
		pubsub.WithAppSpecificRpcInspector(func(id peer.ID, _ *pubsub.RPC) error {
			if !srv.IsPeerAllowed(id) {
				return errPeerAdmission
			}

			return nil
		}),
		pubsub.WithDefaultValidator(
			func(_ context.Context, forwarder peer.ID, msg *pubsub.Message) bool {
				return srv.gossipAllowed(forwarder, msg.GetFrom())
			},
			pubsub.WithValidatorInline(true),
		),
	)
	/* FINISH ALPHA CHANGE 2743 -- enforce membership on pubsub routing ingress and signed messages -- */
	if err != nil {
		gossipCancel()

		return nil, errors.Join(err, emitter.Close(), host.Close())
	}

	srv.ps = ps
	srv.gossipCancel = gossipCancel

	return srv, nil
}

/* FINISH ALPHA CHANGE 2888 -- own GossipSub lifetime and release resources when construction fails -- */

// HasFreeConnectionSlot checks if there are free connection slots in the specified direction [Thread safe]
func (s *Server) HasFreeConnectionSlot(direction network.Direction) bool {
	return s.connectionCounts.HasFreeConnectionSlot(direction)
}

/* START ALPHA CHANGE 679 -- add validator peer allowlist helpers -- */
/* START ALPHA CHANGE 2743 -- publish membership through connection-aware admission -- */
func (s *Server) SetAllowedPeers(peerIDs []peer.ID) {
	s.replaceAllowedPeers(peerIDs)
}

/* FINISH ALPHA CHANGE 2743 -- publish membership through connection-aware admission -- */

/* START ALPHA CHANGE 2743 -- consult startup directory and shutdown state together -- */
func (s *Server) IsPeerAllowed(peerID peer.ID) bool {
	if s.admission != nil {
		return s.admission.allows(peerID)
	}

	/* Compatibility for explicitly constructed non-listening test servers. */
	s.allowedPeersLock.RLock()
	defer s.allowedPeersLock.RUnlock()

	if !s.peerAllowlistReady {
		return true
	}

	_, ok := s.allowedPeers[peerID]

	return ok
}

/* FINISH ALPHA CHANGE 2743 -- consult startup directory and shutdown state together -- */

/* FINISH ALPHA CHANGE 679 -- add validator peer allowlist helpers -- */

// PeerConnInfo holds the connection information about the peer
type PeerConnInfo struct {
	Info peer.AddrInfo

	connDirections  map[network.Direction]bool
	protocolStreams map[string]*rawGrpc.ClientConn
	/* START ALPHA CHANGE 2743 -- retain individual registered transport identities -- */
	connectionRefs map[string]network.Direction
	/* FINISH ALPHA CHANGE 2743 -- retain individual registered transport identities -- */
}

/* START ALPHA CHANGE 2680 -- detach protocol connections for cleanup outside peer locks -- */
func (pci *PeerConnInfo) addProtocolStream(
	protocol string,
	stream *rawGrpc.ClientConn,
) *rawGrpc.ClientConn {
	previous := pci.protocolStreams[protocol]
	pci.protocolStreams[protocol] = stream

	return previous
}

func (pci *PeerConnInfo) removeProtocolStream(
	protocol string,
) *rawGrpc.ClientConn {
	stream := pci.protocolStreams[protocol]
	delete(pci.protocolStreams, protocol)

	return stream
}

/* FINISH ALPHA CHANGE 2680 -- detach protocol connections for cleanup outside peer locks -- */

// setupLibp2pKey is a helper method for setting up the networking private key
func setupLibp2pKey(secretsManager secrets.SecretsManager) (crypto.PrivKey, error) {
	var key crypto.PrivKey

	if secretsManager.HasSecret(secrets.NetworkKey) {
		// The key is present in the secrets manager, read it
		networkingKey, readErr := ReadLibp2pKey(secretsManager)
		if readErr != nil {
			return nil, fmt.Errorf("unable to read networking private key from Secrets Manager, %w", readErr)
		}

		key = networkingKey
	} else {
		// The key is not present in the secrets manager, generate it
		libp2pKey, libp2pKeyEncoded, keyErr := GenerateAndEncodeLibp2pKey()
		if keyErr != nil {
			return nil, fmt.Errorf("unable to generate networking private key for Secrets Manager, %w", keyErr)
		}

		// Write the networking private key to disk
		if setErr := secretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
			return nil, fmt.Errorf("unable to store networking private key to Secrets Manager, %w", setErr)
		}

		key = libp2pKey
	}

	return key, nil
}

// Start starts the networking services
func (s *Server) Start() error {
	/* START ALPHA CHANGE 2681 -- admit network startup before installing owned services -- */
	_, finish, err := s.rpcLifetime.Begin(context.Background())
	if err != nil {
		return err
	}
	defer finish()
	/* FINISH ALPHA CHANGE 2681 -- admit network startup before installing owned services -- */

	addr, err := common.AddrInfoToString(s.AddrInfo())
	if err != nil {
		return err
	}

	s.logger.Info("LibP2P server running", "addr", addr)

	/* START ALPHA CHANGE 696 -- start authenticated validator peer directory refresh -- */
	s.startPeerDirectoryRefresh()
	/* FINISH ALPHA CHANGE 696 -- start authenticated validator peer directory refresh -- */

	if setupErr := s.setupIdentity(); setupErr != nil {
		return fmt.Errorf("unable to setup identity, %w", setupErr)
	}

	// Set up the peer discovery mechanism if needed
	if !s.config.NoDiscover {
		/* START ALPHA CHANGE 644 -- remove legacy bootnode parsing from discovery startup -- */
		/* FINISH ALPHA CHANGE 644 -- remove legacy bootnode parsing from discovery startup -- */

		// Setup and start the discovery service
		if setupErr := s.setupDiscovery(); setupErr != nil {
			return fmt.Errorf("unable to setup discovery, %w", setupErr)
		}
	}

	/* START ALPHA CHANGE 2743 -- install owned cleanup before opening startup admission -- */
	s.host.Network().Notify(&network.NotifyBundle{
		DisconnectedF: func(_ network.Network, conn network.Conn) {
			s.removePeerConnection(conn)
		},
	})

	if !s.startPeerAdmission() {
		return netgrpc.ErrRPCClosed
	}

	go s.runDial()
	go s.keepAliveMinimumPeerConnections()
	/* FINISH ALPHA CHANGE 2743 -- install owned cleanup before opening startup admission -- */

	return nil
}

/* START ALPHA CHANGE 645 -- remove legacy bootnode parser from network server -- */
/* FINISH ALPHA CHANGE 645 -- remove legacy bootnode parser from network server -- */

// keepAliveMinimumPeerConnections will attempt to make new connections
// if the active peer count is lesser than the specified limit.
func (s *Server) keepAliveMinimumPeerConnections() {
	for {
		select {
		case <-time.After(10 * time.Second):
		case <-s.closeCh:
			return
		}

		if s.numPeers() < MinimumPeerConnections {
			/* START ALPHA CHANGE 646 -- keep peer reconnects limited to known non-bootnode peers -- */
			randPeer := s.GetRandomPeer()
			if randPeer != nil && !s.IsConnected(*randPeer) {
				s.addToDialQueue(s.GetPeerInfo(*randPeer), common.PriorityRandomDial)
			}
			/* FINISH ALPHA CHANGE 646 -- keep peer reconnects limited to known non-bootnode peers -- */
		}
	}
}

// runDial starts the networking server's dial loop.
// Essentially, the networking server monitors for any open connection slots
// and attempts to fill them as soon as they open up
/* START ALPHA CHANGE 2760 -- own dial slots through establishment and outbound transport removal -- */
/* START ALPHA CHANGE 2777 -- release queue ownership after returning the worker slot -- */
func (s *Server) runDial() {
	ctx, finish, err := s.rpcLifetime.Begin(context.Background())
	if err != nil {
		return
	}
	defer finish()

	slots := NewSlots(s.connectionCounts.maxOutboundConnectionCount)

	for {
		if closed := s.dialQueue.Wait(ctx); closed {
			return
		}

		for {
			tt := s.dialQueue.PopDialTask()
			if tt == nil {
				break
			}

			peerInfo := tt.GetAddrInfo()

			if !s.IsPeerAllowed(peerInfo.ID) {
				id := peerInfo.ID
				s.emitOwnedPeerEvent(id, peerEvent.PeerFailedToConnect, func() bool {
					return !s.hasPeer(id)
				})
				s.dialQueue.FinishDial(tt)

				continue
			}

			if s.IsConnected(peerInfo.ID) {
				s.dialQueue.FinishDial(tt)

				continue
			}

			s.logger.Debug("Waiting for a dialing slot", "addr", peerInfo, "local", s.host.ID())

			if closed := slots.Take(ctx); closed {
				s.dialQueue.FinishDial(tt)

				return
			}

			workerCtx, workerFinish, err := s.rpcLifetime.Begin(ctx)
			if err != nil {
				slots.Release()
				s.dialQueue.FinishDial(tt)

				return
			}

			go func(
				ctx context.Context,
				info peer.AddrInfo,
				task *dial.DialTask,
				finish func(),
			) {
				defer finish()
				defer func() {
					/* Make the slot available before publishing a pending replacement. */
					slots.Release()
					s.dialQueue.FinishDial(task)
				}()

				s.runOwnedDial(ctx, info)
			}(workerCtx, *peerInfo, tt, workerFinish)
		}
	}
}

/* FINISH ALPHA CHANGE 2777 -- release queue ownership after returning the worker slot -- */

func (s *Server) runOwnedDial(ctx context.Context, info peer.AddrInfo) {
	if ctx.Err() != nil || !s.IsPeerAllowed(info.ID) {
		return
	}

	s.logger.Debug("Dialing peer", "addr", info, "local", s.host.ID())

	if err := s.host.Connect(ctx, info); err != nil {
		s.logger.Debug("failed to dial", "addr", info, "err", err.Error())
		s.emitOwnedPeerEvent(info.ID, peerEvent.PeerFailedToConnect, func() bool {
			return !s.hasPeer(info.ID)
		})

		return
	}

	p := s.admission
	if p == nil {
		return
	}

	for ctx.Err() == nil {
		var removed <-chan struct{}

		/* Inspect only this peer's tracked outbound transports. */
		p.lock.RLock()
		for _, ownership := range p.outbound[info.ID] {
			removed = ownership.removed

			break
		}
		p.lock.RUnlock()

		/* Includes successful transport establishment followed by early rejection. */
		if removed == nil {
			return
		}

		select {
		case <-removed:
			/* Another outbound transport to this peer may still be present. */
		case <-ctx.Done():
			return
		}
	}
}

/* FINISH ALPHA CHANGE 2760 -- own dial slots through establishment and outbound transport removal -- */

// numPeers returns the number of connected peers [Thread safe]
func (s *Server) numPeers() int64 {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	return int64(len(s.peers))
}

// Peers returns a copy of the networking server's peer connection info set.
// Only one (initial) connection (inbound OR outbound) per peer is contained [Thread safe]
func (s *Server) Peers() []*PeerConnInfo {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	peers := make([]*PeerConnInfo, 0)
	for _, connectionInfo := range s.peers {
		peers = append(peers, connectionInfo)
	}

	return peers
}

// hasPeer checks if the peer is present in the peers list [Thread safe]
func (s *Server) hasPeer(peerID peer.ID) bool {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	_, ok := s.peers[peerID]

	return ok
}

// IsConnected checks if the networking server is connected to a peer
func (s *Server) IsConnected(peerID peer.ID) bool {
	return s.host.Network().Connectedness(peerID) == network.Connected
}

// GetProtocols fetches the list of node-supported protocols
func (s *Server) GetProtocols(peerID peer.ID) ([]string, error) {
	protocols, err := s.host.Peerstore().GetProtocols(peerID)
	if err != nil {
		return nil, err
	}

	return protocol.ConvertToStrings(protocols), nil
}

// removePeer removes a peer from the networking server's peer list,
// and updates relevant counters and metrics. It is called from the
// disconnection callback of the libp2p network bundle (when the connection is closed)
func (s *Server) removePeer(peerID peer.ID) {
	s.logger.Info("Peer disconnected", "id", peerID)

	// Remove the peer from the peers map
	connectionInfo := s.removePeerInfo(peerID)
	if connectionInfo == nil {
		// The peer wasn't present in the local peers info table
		// so no action should be taken further
		return
	}

	// Emit the event alerting listeners
	s.emitEvent(peerID, peerEvent.PeerDisconnected)
}

// removePeerInfo removes (pops) peer connection info from the networking
// server's peer map. Returns nil if no peer was removed
/* START ALPHA CHANGE 2682 -- release removed peer clients without holding the peer lock -- */
func (s *Server) removePeerInfo(peerID peer.ID) *PeerConnInfo {
	s.peersLock.Lock()

	connectionInfo, ok := s.peers[peerID]
	if !ok {
		s.peersLock.Unlock()
		s.logger.Warn(
			fmt.Sprintf("Attempted removing missing peer info %s", peerID),
		)

		return nil
	}

	delete(s.peers, peerID)

	streams := connectionInfo.protocolStreams
	connectionInfo.protocolStreams = make(map[string]*rawGrpc.ClientConn)

	for connDirection, active := range connectionInfo.connDirections {
		if active {
			s.connectionCounts.UpdateConnCountByDirection(-1, connDirection)
			s.updateConnCountMetrics(connDirection)
		}
	}

	metrics.SetGauge([]string{networkMetrics, "peers"}, float32(len(s.peers)))
	s.peersLock.Unlock()

	for _, conn := range streams {
		if conn != nil {
			_ = conn.Close()
		}
	}

	return connectionInfo
}

/* FINISH ALPHA CHANGE 2682 -- release removed peer clients without holding the peer lock -- */

/* START ALPHA CHANGE 647 -- remove legacy bootnode connection counter -- */
/* FINISH ALPHA CHANGE 647 -- remove legacy bootnode connection counter -- */

// DisconnectFromPeer disconnects the networking server from the specified peer
func (s *Server) DisconnectFromPeer(peer peer.ID, reason string) {
	if s.host.Network().Connectedness(peer) == network.Connected {
		s.logger.Info("Closing connection", "id", peer, "reason", reason)

		if err := s.host.Network().ClosePeer(peer); err != nil {
			s.logger.Error("Unable to gracefully close connection", "id", peer, "err", err)
		}
	}
}

var (
	// Anything below 35s is prone to false timeouts, as seen from empirical test data
	DefaultJoinTimeout   = 100 * time.Second
	DefaultBufferTimeout = DefaultJoinTimeout + time.Second*5
)

// JoinPeer attempts to add a new peer to the networking server
func (s *Server) JoinPeer(rawPeerMultiaddr string) error {
	// Parse the raw string to a MultiAddr format
	parsedMultiaddr, err := multiaddr.NewMultiaddr(rawPeerMultiaddr)
	if err != nil {
		return err
	}

	// Extract the peer info from the Multiaddr
	peerInfo, err := peer.AddrInfoFromP2pAddr(parsedMultiaddr)
	if err != nil {
		return err
	}

	// Mark the peer as ripe for dialing (async)
	s.joinPeer(peerInfo)

	return nil
}

// joinPeer creates a new dial task for the peer (for async joining)
func (s *Server) joinPeer(peerInfo *peer.AddrInfo) {
	s.logger.Info("Join request", "addr", peerInfo)

	// This method can be completely refactored to support some kind of active
	// feedback information on the dial status, and not just asynchronous updates.
	// For this feature to work, the networking server requires a flexible event subscription
	// manager that is configurable and cancelable at any point in time
	s.addToDialQueue(peerInfo, common.PriorityRequestedDial)
}

/* START ALPHA CHANGE 2683 -- cancel admitted RPC work before joining and closing network resources -- */
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		/* START ALPHA CHANGE 2743 -- prevent late refreshes and new connections during shutdown -- */
		s.stopPeerAdmission()
		/* FINISH ALPHA CHANGE 2743 -- prevent late refreshes and new connections during shutdown -- */
		/* START ALPHA CHANGE 2889 -- signal GossipSub shutdown before joining networking work -- */
		if s.gossipCancel != nil {
			s.gossipCancel()
		}
		/* FINISH ALPHA CHANGE 2889 -- signal GossipSub shutdown before joining networking work -- */
		close(s.closeCh)
		s.dialQueue.Close()

		s.rpcLifetime.Close()

		if s.identityService != nil {
			s.identityService.Close()
		}

		if s.discovery != nil {
			s.discovery.Close()
		}

		s.protocolsLock.Lock()
		protocols := make([]Protocol, 0, len(s.protocols))
		for _, p := range s.protocols {
			protocols = append(protocols, p)
		}
		clear(s.protocols)
		s.protocolsLock.Unlock()

		for _, p := range protocols {
			p.Stop()
		}

		s.peersLock.Lock()
		var connections []*rawGrpc.ClientConn
		for _, info := range s.peers {
			for _, conn := range info.protocolStreams {
				if conn != nil {
					connections = append(connections, conn)
				}
			}
			clear(info.protocolStreams)
		}
		s.peersLock.Unlock()

		for _, conn := range connections {
			if err := conn.Close(); err != nil &&
				!errors.Is(err, rawGrpc.ErrClientConnClosing) {
				s.closeErr = errors.Join(s.closeErr, err)
			}
		}

		s.closeErr = errors.Join(s.closeErr, s.host.Close())
	})

	return s.closeErr
}

/* FINISH ALPHA CHANGE 2683 -- cancel admitted RPC work before joining and closing network resources -- */

/* START ALPHA CHANGE 2684 -- establish protocols outside the registry lock with cancellable transport readiness -- */
func (s *Server) NewProtoConnection(
	proto string,
	peerID peer.ID,
) (*rawGrpc.ClientConn, error) {
	return s.NewProtoConnectionContext(context.Background(), proto, peerID)
}

func (s *Server) NewProtoConnectionContext(
	parent context.Context,
	proto string,
	peerID peer.ID,
) (*rawGrpc.ClientConn, error) {
	ctx, finish, err := s.rpcLifetime.Begin(parent)
	if err != nil {
		return nil, err
	}
	defer finish()

	ctx, cancel := context.WithTimeout(ctx, netgrpc.DefaultProtocolTimeout)
	defer cancel()

	s.protocolsLock.Lock()
	p, ok := s.protocols[proto]
	s.protocolsLock.Unlock()

	if !ok || p == nil {
		return nil, fmt.Errorf("protocol not found: %s", proto)
	}

	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(proto))
	if err != nil {
		return nil, err
	}

	conn, err := p.ClientContext(ctx, stream)
	if err != nil {
		_ = stream.Reset()

		return nil, err
	}

	if conn == nil {
		_ = stream.Reset()

		return nil, errors.New("protocol returned a nil client connection")
	}

	if err := ctx.Err(); err != nil {
		_ = stream.Reset()
		_ = conn.Close()

		return nil, err
	}

	return conn, nil
}

func (s *Server) NewStream(proto string, id peer.ID) (network.Stream, error) {
	ctx, finish, err := s.rpcLifetime.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer finish()

	ctx, cancel := context.WithTimeout(ctx, netgrpc.DefaultProtocolTimeout)
	defer cancel()

	stream, err := s.host.NewStream(ctx, id, protocol.ID(proto))
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		_ = stream.Reset()

		return nil, err
	}

	return stream, nil
}

type Protocol interface {
	Client(network.Stream) (*rawGrpc.ClientConn, error)
	ClientContext(context.Context, network.Stream) (*rawGrpc.ClientConn, error)
	Handler() func(network.Stream)
	Stop()
}

func (s *Server) RegisterProtocol(id string, p Protocol) {
	if p == nil {
		return
	}

	s.protocolsLock.Lock()

	select {
	case <-s.closeCh:
		s.protocolsLock.Unlock()
		p.Stop()

		return
	default:
	}

	/* START ALPHA CHANGE 2743 -- authorize every RPC on registered P2P gRPC services -- */
	if service, ok := p.(*netgrpc.GrpcStream); ok {
		service.SetPeerAuthorizer(s.IsPeerAllowed)
	}
	/* FINISH ALPHA CHANGE 2743 -- authorize every RPC on registered P2P gRPC services -- */

	previous := s.protocols[id]

	/* Preserve idempotent registration without comparing uncomparable values. */
	if previous != nil &&
		reflect.TypeOf(previous).Comparable() &&
		previous == p {
		s.protocolsLock.Unlock()

		return
	}

	s.protocols[id] = p
	s.wrapStream(id, p.Handler())
	s.protocolsLock.Unlock()

	if previous != nil {
		previous.Stop()
	}
}

/* FINISH ALPHA CHANGE 2684 -- establish protocols outside the registry lock with cancellable transport readiness -- */

/* START ALPHA CHANGE 2743 -- authorize authenticated peers before protocol dispatch -- */
func (s *Server) wrapStream(id string, handle func(network.Stream)) {
	s.host.SetStreamHandler(protocol.ID(id), func(stream network.Stream) {
		peerID := stream.Conn().RemotePeer()
		if !s.IsPeerAllowed(peerID) {
			_ = stream.Reset()

			return
		}

		s.logger.Debug("open stream", "protocol", id, "peer", peerID)

		handle(stream)
	})
}

/* FINISH ALPHA CHANGE 2743 -- authorize authenticated peers before protocol dispatch -- */

func (s *Server) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.addrs,
	}
}

func (s *Server) addToDialQueue(addr *peer.AddrInfo, priority common.DialPriority) {
	s.dialQueue.AddTask(addr, priority)
	s.emitEvent(addr.ID, peerEvent.PeerAddedToDialQueue)
}

func (s *Server) emitEvent(peerID peer.ID, peerEventType peerEvent.PeerEventType) {
	// POTENTIALLY BLOCKING
	if err := s.emitterPeerEvent.Emit(peerEvent.PeerEvent{
		PeerID: peerID,
		Type:   peerEventType,
	}); err != nil {
		s.logger.Info("failed to emit event", "peer", peerID, "type", peerEventType, "err", err)
	}
}

// Subscribe is a helper method to run subscription of PeerEvents
func (s *Server) Subscribe(ctx context.Context, handler func(evnt *peerEvent.PeerEvent)) error {
	sub, err := s.host.EventBus().Subscribe(new(peerEvent.PeerEvent))
	if err != nil {
		return err
	}

	go func() {
		defer sub.Close()

		for {
			select {
			case <-ctx.Done():
				return

			case <-s.closeCh:
				return

			/* START ALPHA CHANGE 2092 -- stop subscription dispatch after observed cancellation or closure -- */
			case evnt, open := <-sub.Out():
				if !open {
					return
				}

				select {
				case <-ctx.Done():
					return
				case <-s.closeCh:
					return
				default:
				}

				if obj, ok := evnt.(peerEvent.PeerEvent); ok {
					handler(&obj)
				}
				/* FINISH ALPHA CHANGE 2092 -- stop subscription dispatch after observed cancellation or closure -- */
			}
		}
	}()

	return nil
}

// SubscribeCh returns an event of of subscription events
/* START ALPHA CHANGE 2685 -- give peer subscription delivery one joined channel owner -- */
func (s *Server) SubscribeCh(
	parent context.Context,
) (<-chan *peerEvent.PeerEvent, error) {
	ctx, finish, err := s.rpcLifetime.Begin(parent)
	if err != nil {
		return nil, err
	}

	sub, err := s.host.EventBus().Subscribe(new(peerEvent.PeerEvent))
	if err != nil {
		finish()

		return nil, err
	}

	ch := make(chan *peerEvent.PeerEvent)

	go func() {
		defer finish()
		defer sub.Close()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.closeCh:
				return
			case value, open := <-sub.Out():
				if !open {
					return
				}

				evnt, ok := value.(peerEvent.PeerEvent)
				if !ok {
					continue
				}

				select {
				case <-ctx.Done():
					return
				case <-s.closeCh:
					return
				case ch <- &evnt:
				}
			}
		}
	}()

	return ch, nil
}

/* FINISH ALPHA CHANGE 2685 -- give peer subscription delivery one joined channel owner -- */

// updateConnCountMetrics updates the connection count metrics
func (s *Server) updateConnCountMetrics(direction network.Direction) {
	switch direction {
	case network.DirInbound:
		metrics.SetGauge([]string{networkMetrics, "inbound_connections_count"},
			float32(s.connectionCounts.GetInboundConnCount()))

	case network.DirOutbound:
		metrics.SetGauge([]string{networkMetrics, "outbound_connections_count"},
			float32(s.connectionCounts.GetOutboundConnCount()))
	}
}

// updatePendingConnCountMetrics updates the pending connection count metrics
func (s *Server) updatePendingConnCountMetrics(direction network.Direction) {
	switch direction {
	case network.DirInbound:
		metrics.SetGauge([]string{networkMetrics, "pending_inbound_connections_count"},
			float32(s.connectionCounts.GetPendingInboundConnCount()))

	case network.DirOutbound:
		metrics.SetGauge([]string{networkMetrics, "pending_outbound_connections_count"},
			float32(s.connectionCounts.GetPendingOutboundConnCount()))
	}
}
