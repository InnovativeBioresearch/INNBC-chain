/* START ALPHA CHANGE 3831 -- verify startup peer requirements follow committee changes -- */

package innbcbft

import (
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/network"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type startupPeerWaitObservation struct {
	connected int
	required  int
}

type startupPeerWaitLogger struct {
	hclog.Logger

	observations chan startupPeerWaitObservation
	stopped      <-chan struct{}
	onWait       func()
}

// The existing wait log provides a synchronization point after an
// unsuccessful poll. No production test hook or timing sleep is needed.
func (l *startupPeerWaitLogger) Debug(message string, args ...any) {
	if message != "waiting for validator peers before starting consensus" {
		return
	}

	var observation startupPeerWaitObservation

	for i := 0; i+1 < len(args); i += 2 {
		value, ok := args[i+1].(int)
		if !ok {
			continue
		}

		switch args[i] {
		case "connectedPeers":
			observation.connected = value
		case "requiredPeers":
			observation.required = value
		}
	}

	if l.onWait != nil {
		l.onWait()
	}

	// An unbuffered observation prevents the waiter from advancing past
	// this synchronization point before the test receives it.
	select {
	case l.observations <- observation:
	case <-l.stopped:
	}
}

func startupPeerWaitSetCommittee(p *INNBCBFT, count int) {
	p.runtime.lock.Lock()
	defer p.runtime.lock.Unlock()

	epochNumber := uint64(1)
	if p.runtime.epoch != nil {
		epochNumber = p.runtime.epoch.Number + 1
	}

	// This startup check consumes only AccountSet.Len(); signing
	// identities and a complete execution runtime are unnecessary.
	p.runtime.epoch = &epochMetadata{
		Number:     epochNumber,
		Validators: make(validator.AccountSet, count),
	}
}

func startupPeerWaitNode(
	t *testing.T,
	committeeSize int,
) (*INNBCBFT, *startupPeerWaitLogger) {
	t.Helper()

	config := network.DefaultConfig()
	config.Addr.Port = 0
	config.NoDiscover = true
	config.PeerDirectoryEnabled = false
	config.DataDir = t.TempDir()
	config.Chain = &chain.Chain{
		Params: &chain.Params{ChainID: 1982},
	}

	manager, err := secretshelper.SetupLocalSecretsManager(config.DataDir)
	require.NoError(t, err)
	config.SecretsManager = manager

	networkServer, err := network.NewServer(hclog.NewNullLogger(), config)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, networkServer.Close())
	})

	// Do not call networkServer.Start(). The tests control registered
	// peer records directly rather than starting discovery or dialing.
	p := &INNBCBFT{
		config:      &consensus.Params{Network: networkServer},
		runtime:     &consensusRuntime{},
		closeCh:     make(chan struct{}),
		fatalStopCh: make(chan struct{}),
	}
	startupPeerWaitSetCommittee(p, committeeSize)

	logger := &startupPeerWaitLogger{
		Logger:       hclog.NewNullLogger(),
		observations: make(chan startupPeerWaitObservation),
		stopped:      p.closeCh,
	}
	p.logger = logger

	return p, logger
}

func startupPeerWaitPeerID(t *testing.T) peer.ID {
	t.Helper()

	_, publicKey, err := p2pcrypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)

	id, err := peer.IDFromPublicKey(publicKey)
	require.NoError(t, err)

	return id
}

type startupPeerWaitRun struct {
	done   chan struct{}
	result bool
	stop   func()
}

func startupPeerWaitStart(t *testing.T, p *INNBCBFT) *startupPeerWaitRun {
	t.Helper()

	run := &startupPeerWaitRun{done: make(chan struct{})}

	var stopOnce sync.Once
	run.stop = func() {
		stopOnce.Do(func() {
			close(p.closeCh)
		})
	}

	// Registered after network cleanup, so this joins the waiting
	// goroutine before its networking fixture is closed.
	t.Cleanup(func() {
		run.stop()

		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()

		select {
		case <-run.done:
		case <-timer.C:
			t.Error("startup peer waiter did not stop during cleanup")
		}
	})

	go func() {
		run.result = p.waitForValidatorPeers()
		close(run.done)
	}()

	return run
}

func startupPeerWaitNextObservation(
	t *testing.T,
	run *startupPeerWaitRun,
	logger *startupPeerWaitLogger,
) startupPeerWaitObservation {
	t.Helper()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	select {
	case observation := <-logger.observations:
		return observation
	case <-run.done:
		t.Fatalf("startup wait returned %t before the expected waiting poll", run.result)
	case <-timer.C:
		t.Fatal("startup wait did not produce the expected waiting poll")
	}

	return startupPeerWaitObservation{}
}

func startupPeerWaitResult(
	t *testing.T,
	run *startupPeerWaitRun,
	logger *startupPeerWaitLogger,
	want bool,
) {
	t.Helper()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-run.done:
			require.Equal(t, want, run.result)
			return

		case observation := <-logger.observations:
			if want {
				t.Fatalf(
					"startup unexpectedly kept waiting: connected=%d required=%d",
					observation.connected,
					observation.required,
				)
			}

			// A timer and cancellation can become ready together.
			// Permit a final waiting log while requiring cancellation
			// to return false.

		case <-timer.C:
			t.Fatal("startup wait did not return")
		}
	}
}

func TestStartupPeerWait_Thresholds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		validators int
		required   int
	}{
		{validators: 1, required: 0},
		{validators: 2, required: 1},
		{validators: 3, required: 2},
		{validators: 4, required: 2},
		{validators: 6, required: 2},
	} {
		t.Run(fmt.Sprintf("validators_%d", tc.validators), func(t *testing.T) {
			p := &INNBCBFT{runtime: &consensusRuntime{}}
			startupPeerWaitSetCommittee(p, tc.validators)

			require.Equal(t, tc.required, p.requiredValidatorPeers())
		})
	}
}

func TestStartupPeerWait_CommitteeShrinks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		validators int
		peers      int
	}{
		{name: "six_to_two", validators: 2, peers: 1},
		{name: "six_to_one", validators: 1, peers: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, logger := startupPeerWaitNode(t, 6)

			for i := 0; i < tc.peers; i++ {
				p.config.Network.AddPeer(
					startupPeerWaitPeerID(t),
					libnetwork.DirInbound,
				)
			}

			changed := false
			logger.onWait = func() {
				if !changed {
					changed = true
					startupPeerWaitSetCommittee(p, tc.validators)
				}
			}

			run := startupPeerWaitStart(t, p)

			first := startupPeerWaitNextObservation(t, run, logger)
			require.Equal(t, tc.peers, first.connected)
			require.Equal(t, 2, first.required)

			require.Equal(t, tc.validators-1, p.requiredValidatorPeers())
			startupPeerWaitResult(t, run, logger, true)
		})
	}
}

func TestStartupPeerWait_CommitteeGrows(t *testing.T) {
	t.Parallel()

	p, logger := startupPeerWaitNode(t, 2)
	firstPeer := startupPeerWaitPeerID(t)
	secondPeer := startupPeerWaitPeerID(t)

	polls := 0
	logger.onWait = func() {
		polls++

		switch polls {
		case 1:
			startupPeerWaitSetCommittee(p, 6)
		case 2:
			p.config.Network.AddPeer(firstPeer, libnetwork.DirInbound)
		case 3:
			p.config.Network.AddPeer(secondPeer, libnetwork.DirInbound)
		}
	}

	run := startupPeerWaitStart(t, p)

	first := startupPeerWaitNextObservation(t, run, logger)
	require.Equal(t, startupPeerWaitObservation{connected: 0, required: 1}, first)

	second := startupPeerWaitNextObservation(t, run, logger)
	require.Equal(t, startupPeerWaitObservation{connected: 0, required: 2}, second)

	third := startupPeerWaitNextObservation(t, run, logger)
	require.Equal(t, startupPeerWaitObservation{connected: 1, required: 2}, third)

	startupPeerWaitResult(t, run, logger, true)
	require.Len(t, p.config.Network.Peers(), 2)
}

func TestStartupPeerWait_StableCommittees(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		validators int
		peers      int
	}{
		{validators: 1, peers: 0},
		{validators: 2, peers: 1},
		{validators: 6, peers: 2},
	} {
		t.Run(fmt.Sprintf("validators_%d", tc.validators), func(t *testing.T) {
			t.Parallel()

			p, logger := startupPeerWaitNode(t, tc.validators)

			for i := 0; i < tc.peers; i++ {
				p.config.Network.AddPeer(
					startupPeerWaitPeerID(t),
					libnetwork.DirInbound,
				)
			}

			run := startupPeerWaitStart(t, p)
			startupPeerWaitResult(t, run, logger, true)
		})
	}
}

func TestStartupPeerWait_Cancellation(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"shutdown", "fatal"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			p, logger := startupPeerWaitNode(t, 6)
			run := startupPeerWaitStart(t, p)

			first := startupPeerWaitNextObservation(t, run, logger)
			require.Equal(t, startupPeerWaitObservation{connected: 0, required: 2}, first)

			if mode == "shutdown" {
				run.stop()
			} else {
				close(p.fatalStopCh)
			}

			startupPeerWaitResult(t, run, logger, false)
		})
	}
}

/* FINISH ALPHA CHANGE 3831 -- verify startup peer requirements follow committee changes -- */
