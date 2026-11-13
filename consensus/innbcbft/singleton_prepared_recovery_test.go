// START ALPHA CHANGE 3793 -- reproduce singleton prepared recovery with serialized and real gossip delivery --
package innbcbft

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/bls"
	chainconfig "github.com/innovativebioresearch/innbc-chain/chain"
	core "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftcore"
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	ibftproto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/network"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	ethwallet "github.com/umbracle/ethgo/wallet"
	protobuf "google.golang.org/protobuf/proto"
)

type singletonRecoveryChain struct {
	*blockchainMock
}

func (*singletonRecoveryChain) GetChainID() uint64 {
	return 1982
}

type singletonRecoveryBackend struct {
	*consensusRuntime
	builds     atomic.Uint32
	replays    atomic.Uint32
	insertions atomic.Uint32
	inserted   chan *types.FullBlock
}

func (b *singletonRecoveryBackend) BuildExecutedProposal(
	view *ibftproto.View,
) ([]byte, *core.ProposalExecution) {
	b.builds.Add(1)
	return b.consensusRuntime.BuildExecutedProposal(view)
}

func (b *singletonRecoveryBackend) ExecuteProposal(raw []byte) *core.ProposalExecution {
	b.replays.Add(1)
	return b.consensusRuntime.ExecuteProposal(raw)
}

func (b *singletonRecoveryBackend) InsertProposal(
	proposal *ibftproto.Proposal,
	seals []*messages.CommittedSeal,
	execution *core.ProposalExecution,
) error {
	return b.consensusRuntime.insertExecutedProposal(
		proposal,
		seals,
		execution,
		func(block *types.FullBlock) error {
			b.lock.Lock()
			b.lastBuiltBlock = block.Block.Header.Copy()
			b.lock.Unlock()

			b.insertions.Add(1)
			b.inserted <- block
			return nil
		},
	)
}

type singletonRecoveryWire struct {
	data   []byte
	err    error
	author peer.ID
}

type singletonRecoveryTransport struct {
	ctx            context.Context
	outgoing       chan singletonRecoveryWire
	topic          *network.Topic
	expectedAuthor peer.ID
}

func (t *singletonRecoveryTransport) enqueue(
	data []byte,
	err error,
	author peer.ID,
) {
	select {
	case t.outgoing <- singletonRecoveryWire{
		data: data, err: err, author: author,
	}:
	case <-t.ctx.Done():
	}
}

func (t *singletonRecoveryTransport) Multicast(message *ibftproto.Message) {
	if t.topic != nil {
		if err := t.topic.Publish(message); err != nil {
			t.enqueue(nil, err, "")
		}
		return
	}

	data, err := protobuf.Marshal(message)
	t.enqueue(data, err, "")
}

func singletonRecoveryPendingGossip(
	t *testing.T,
	transport *singletonRecoveryTransport,
) *network.Server {
	t.Helper()

	entered := make(chan struct{}, 1)
	directory := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		select {
		case entered <- struct{}{}:
		default:
		}
		http.Error(w, "controlled unavailable directory",
			http.StatusInternalServerError)
	}))
	t.Cleanup(directory.Close)

	config := network.DefaultConfig()
	config.Addr.Port = 0
	config.NoDiscover = true
	config.PeerDirectoryEnabled = true
	config.PeerDirectoryURL = directory.URL + "/api/v1/peers/exchange"
	config.DataDir = t.TempDir()
	config.Chain = &chainconfig.Chain{
		Params: &chainconfig.Params{
			ChainID: 1982,
			Engine: map[string]interface{}{
				ConsensusName: struct{}{},
			},
		},
	}

	manager, err := secretshelper.SetupLocalSecretsManager(config.DataDir)
	require.NoError(t, err)
	config.SecretsManager = manager

	server, err := network.NewServer(hclog.NewNullLogger(), config)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	require.NoError(t, server.Start())

	select {
	case <-entered:
	case <-transport.ctx.Done():
		t.Fatal("network did not attempt its directory request")
	}

	localID := server.AddrInfo().ID
	require.False(t, server.IsPeerAllowed(localID),
		"directory membership must remain uninitialized")

	topic, err := server.NewTopic(pbftProto, &ibftproto.Message{})
	require.NoError(t, err)
	t.Cleanup(topic.Close)
	transport.topic = topic
	transport.expectedAuthor = localID

	require.NoError(t, topic.Subscribe(func(obj any, from peer.ID) {
		message, ok := obj.(*ibftproto.Message)
		if !ok {
			transport.enqueue(nil,
				fmt.Errorf("unexpected consensus object %T", obj), from)
			return
		}

		// This object already traversed real publication, subscription,
		// unmarshalling and delivery authorization.
		data, err := protobuf.Marshal(message)
		transport.enqueue(data, err, from)
	}))

	return server
}

func TestSingleValidatorPreparedRecovery_SerializedRoundChange(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		realGossip bool
	}{
		{name: "serialized_loopback"},
		{name: "pending_directory_real_gossip", realGossip: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			singletonPreparedRecovery(t, scenario.realGossip)
		})
	}
}

func singletonPreparedRecovery(t *testing.T, realGossip bool) {
	t.Helper()

	// Exercise the production first-round timer once. Channels control
	// delivery; the context deadline is a watchdog.
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	logger := hclog.NewNullLogger()
	rawECDSAKey := make([]byte, 32)
	rawECDSAKey[31] = 1
	ecdsaKey, err := ethwallet.NewWalletFromPrivKey(rawECDSAKey)
	require.NoError(t, err)
	blsKey, err := bls.UnmarshalPrivateKey([]byte("1"))
	require.NoError(t, err)

	account := &wallet.Account{Ecdsa: ecdsaKey, Bls: blsKey}
	accounts := validator.AccountSet{
		{
			Address:     types.Address(ecdsaKey.Address()),
			BlsKey:      blsKey.PublicKey(),
			VotingPower: big.NewInt(1),
			IsActive:    true,
		},
	}
	parent := &types.Header{
		Number:    0,
		Timestamp: 1000,
		GasLimit:  5_242_880,
		BaseFee:   1,
		ExtraData: BuildGenesisExtraData(accounts),
	}
	parent.ComputeHash()

	chainMock := new(blockchainMock)
	chainMock.On("CommitBlock", mock.Anything).Return(nil).Once()
	chain := &singletonRecoveryChain{blockchainMock: chainMock}

	config := &runtimeConfig{
		INNBCBFTConfig:   testChainClockConfig(),
		Key:              wallet.NewKey(account),
		blockchain:       chain,
		reportFatalError: func(error) {},
	}
	snapshot := NewProposerSnapshot(1, accounts)
	runtime := &consensusRuntime{
		config:         config,
		logger:         logger,
		lastBuiltBlock: parent,
		epoch: &epochMetadata{
			Number:     1,
			Validators: accounts,
		},
		proposerCalculator: NewProposerCalculatorFromSnapshot(
			snapshot, config, logger),
		fsm: &fsm{
			parent:           parent,
			backend:          chain,
			validators:       validator.NewValidatorSet(accounts, logger),
			proposerSnapshot: snapshot.Copy(),
			epochNumber:      1,
			config:           config.INNBCBFTConfig,
			logger:           logger,
		},
	}

	block := createDummyStateBlock(1, parent.Hash, nil, parent.Timestamp)
	block.Block.Header.GasLimit = parent.GasLimit
	block.Block.Header.BaseFee = parent.BaseFee
	block.Block.Header.Miner = accounts[0].Address.Bytes()

	builder := new(blockBuilderMock)
	builder.On("Build", mock.Anything).Return(block).Once()
	builder.On("Fill").Return(error(nil)).Once()
	builder.On("Reset").Return(error(nil)).Once()
	runtime.fsm.blockBuilder = builder

	backend := &singletonRecoveryBackend{
		consensusRuntime: runtime,
		inserted:         make(chan *types.FullBlock, 2),
	}
	transport := &singletonRecoveryTransport{
		ctx:      ctx,
		outgoing: make(chan singletonRecoveryWire, 16),
	}
	var gossipServer *network.Server
	if realGossip {
		gossipServer = singletonRecoveryPendingGossip(t, transport)
	}
	engine := core.NewIBFT(logger, backend, transport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.RunSequence(ctx, 1)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("cancelled singleton sequence did not stop")
		}
	})

	var (
		initialRaw             []byte
		initialHash            types.Hash
		initialCheckpointHash  types.Hash
		droppedFirstCommit     bool
		sawPreparedRoundChange bool
		sawReproposal          bool
	)

	for {
		select {
		case wire := <-transport.outgoing:
			require.NoError(t, wire.err,
				"local consensus publication failed before directory initialization")
			if realGossip {
				require.Equal(t, transport.expectedAuthor, wire.author)
			}

			message := new(ibftproto.Message)
			require.NoError(t, protobuf.Unmarshal(wire.data, message))
			require.NotNil(t, message.View)
			require.Equal(t, uint64(1), message.View.Height)
			require.True(t, runtime.IsValidValidator(message),
				"serialized consensus message lost its valid ECDSA signature")

			switch message.Type {
			case ibftproto.MessageType_PREPREPARE:
				data := message.GetPreprepareData()
				require.NotNil(t, data)
				require.NotNil(t, data.Proposal)
				require.Len(t, data.ProposalHash, 32)
				require.True(t, runtime.IsValidProposalHash(
					data.Proposal, data.ProposalHash))

				if message.View.Round == 0 {
					require.Empty(t, initialRaw)
					require.Nil(t, data.Certificate)
					initialRaw = append([]byte(nil), data.Proposal.RawProposal...)
					initialCheckpointHash = types.BytesToHash(data.ProposalHash)

					var proposed types.Block
					require.NoError(t, proposed.UnmarshalRLP(initialRaw))
					initialHash = proposed.Hash()
				} else {
					require.Equal(t, uint64(1), message.View.Round)
					require.True(t, sawPreparedRoundChange)
					require.Equal(t, initialRaw, data.Proposal.RawProposal,
						"later round replaced the prepared block")
					require.NotNil(t, data.Certificate)
					require.Len(t, data.Certificate.RoundChangeMessages, 1)
					sawReproposal = true
				}

			case ibftproto.MessageType_COMMIT:
				data := message.GetCommitData()
				require.NotNil(t, data)
				require.NotEmpty(t, initialRaw)
				require.Equal(t, initialCheckpointHash.Bytes(), data.ProposalHash)
				require.NoError(t, runtime.fsm.ValidateCommit(
					message.From, data.CommittedSeal, data.ProposalHash),
					"serialized COMMIT lost its valid BLS seal")

				if !droppedFirstCommit {
					require.Equal(t, uint64(0), message.View.Round)
					droppedFirstCommit = true

					// Drop application delivery only. In the real-gossip
					// variant this COMMIT has reached the subscriber.
					continue
				}
				require.Equal(t, uint64(1), message.View.Round)
				require.True(t, sawReproposal)

			case ibftproto.MessageType_ROUND_CHANGE:
				require.True(t, droppedFirstCommit)
				require.Equal(t, uint64(1), message.View.Round)
				require.Zero(t, backend.insertions.Load())

				data := message.GetRoundChangeData()
				require.NotNil(t, data)
				require.NotNil(t, data.LastPreparedProposal)
				require.NotNil(t, data.LatestPreparedCertificate)
				require.Equal(t, initialRaw, data.LastPreparedProposal.RawProposal)

				certificate := data.LatestPreparedCertificate
				require.NotNil(t, certificate.ProposalMessage)
				require.Empty(t, certificate.PrepareMessages)
				require.Nil(t, certificate.PrepareMessages,
					"protobuf decoding must preserve the actual empty-list representation")
				require.Equal(t, initialRaw,
					certificate.ProposalMessage.GetPreprepareData().Proposal.RawProposal)
				sawPreparedRoundChange = true

			default:
				t.Fatalf("unexpected singleton message type: %s", message.Type)
			}

			// Do not normalize the nil prepare list to work around the bug.
			require.True(t, isStructurallyValidIBFTMessage(message),
				"valid serialized message rejected: type=%s round=%d",
				message.Type, message.View.Round)
			engine.AddMessage(message)

		case <-done:
			require.NoError(t, engine.SequenceFailure())
			require.True(t, droppedFirstCommit)
			require.True(t, sawPreparedRoundChange)
			require.True(t, sawReproposal)
			require.Equal(t, uint32(1), backend.builds.Load())
			require.Zero(t, backend.replays.Load(),
				"prepared block was unnecessarily re-executed")
			require.Equal(t, uint32(1), backend.insertions.Load())

			var committed *types.FullBlock
			select {
			case committed = <-backend.inserted:
			default:
				t.Fatal("sequence finished without inserting its selected block")
			}
			require.Equal(t, uint64(1), committed.Block.Number())
			require.Equal(t, initialHash, committed.Block.Hash())
			require.Equal(t, types.HeaderHash(committed.Block.Header),
				committed.Block.Header.Hash)

			extra, err := GetIbftExtra(committed.Block.Header.ExtraData)
			require.NoError(t, err)
			require.NotNil(t, extra.Committed)
			require.NoError(t, extra.Committed.Verify(
				1, accounts, initialCheckpointHash,
				signer.DomainCheckpointManager, logger))
			require.Equal(t, uint64(1), runtime.lastBuiltBlock.Number)

			if realGossip {
				require.NotNil(t, gossipServer)
				require.False(t, gossipServer.IsPeerAllowed(
					gossipServer.AddrInfo().ID),
					"test unexpectedly initialized directory membership")
				require.Empty(t, gossipServer.Peers())
			}

			chainMock.AssertNumberOfCalls(t, "CommitBlock", 1)
			chainMock.AssertNotCalled(t, "ProcessBlock", mock.Anything, mock.Anything)
			chainMock.AssertExpectations(t)
			builder.AssertExpectations(t)
			return

		case <-ctx.Done():
			t.Fatalf(
				"singleton did not recover: droppedCommit=%t roundChange=%t reproposal=%t builds=%d insertions=%d failure=%v",
				droppedFirstCommit, sawPreparedRoundChange, sawReproposal,
				backend.builds.Load(), backend.insertions.Load(),
				engine.SequenceFailure())
		}
	}
}

// FINISH ALPHA CHANGE 3793 -- reproduce singleton prepared recovery with serialized and real gossip delivery --
