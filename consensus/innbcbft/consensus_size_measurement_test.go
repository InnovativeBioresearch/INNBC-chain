/* START ALPHA CHANGE 2892 -- measure signed consensus evidence without changing production limits -- */

package innbcbft

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftcore"
	ibftmessages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	ibftproto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	chaincrypto "github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	libcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	ethwallet "github.com/umbracle/ethgo/wallet"
	protobuf "google.golang.org/protobuf/proto"
)

const (
	sizeWatchdog        = 30 * time.Second
	sizeMaxMessage      = 4 << 20
	sizeMaxReprepares   = 3
	sizeFixtureGasLimit = uint64(5_242_880)
)

type sizeFixture struct {
	accounts   []*wallet.Account
	runtimes   []*consensusRuntime
	validators validator.AccountSet
	block      *types.Block
	raw        []byte
}

func newSizeFixture(
	t *testing.T,
	validatorCount, transactionCount, dataPerTransaction int,
) *sizeFixture {
	t.Helper()

	/* Bound allocation before constructing transactions or evidence. */
	require.Contains(t, []int{4, 7}, validatorCount)
	require.Greater(t, transactionCount, 0)
	require.LessOrEqual(t, transactionCount, 9)
	require.GreaterOrEqual(t, dataPerTransaction, 0)
	require.LessOrEqual(t, dataPerTransaction, 120_000)

	logger := hclog.NewNullLogger()
	fixture := &sizeFixture{}

	for i := 0; i < validatorCount; i++ {
		rawKey := make([]byte, 32)
		rawKey[31] = byte(i + 1)

		ecdsaKey, err := ethwallet.NewWalletFromPrivKey(rawKey)
		require.NoError(t, err)

		blsKey, err := bls.UnmarshalPrivateKey([]byte(strconv.Itoa(i + 1)))
		require.NoError(t, err)

		account := &wallet.Account{Ecdsa: ecdsaKey, Bls: blsKey}
		fixture.accounts = append(fixture.accounts, account)
		fixture.validators = append(fixture.validators, &validator.ValidatorMetadata{
			Address:     types.Address(ecdsaKey.Address()),
			BlsKey:      blsKey.PublicKey(),
			VotingPower: big.NewInt(1),
			IsActive:    true,
		})
	}

	for _, account := range fixture.accounts {
		fixture.runtimes = append(fixture.runtimes, &consensusRuntime{
			logger: logger,
			config: &runtimeConfig{
				Key:        wallet.NewKey(account),
				blockchain: new(blockchainMock),
			},
			fsm: &fsm{
				parent:           &types.Header{Number: 0},
				validators:       validator.NewValidatorSet(fixture.validators, logger),
				proposerSnapshot: NewProposerSnapshot(1, fixture.validators),
			},
		})
	}

	storage := itrie.NewMemoryStorage()
	t.Cleanup(func() {
		require.NoError(t, storage.Close())
	})

	forks := &chain.Forks{}
	executor := state.NewExecutor(
		&chain.Params{ChainID: 0, Forks: forks},
		itrie.NewState(storage),
		logger,
	)
	/* START ALPHA CHANGE 3810 -- adapt consensus size fixture to checked hash lookups -- */
	executor.GetHash = func(*types.Header) func(uint64) (types.Hash, error) {
		return func(uint64) (types.Hash, error) {
			return types.ZeroHash, nil
		}
	}
	/* FINISH ALPHA CHANGE 3810 -- adapt consensus size fixture to checked hash lookups -- */

	sender := types.Address(fixture.accounts[0].Ecdsa.Address())
	root, err := executor.WriteGenesis(
		map[types.Address]*chain.GenesisAccount{
			sender: {Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)},
		},
		types.ZeroHash,
	)
	require.NoError(t, err)

	parent := &types.Header{
		Number:    0,
		Timestamp: 1000,
		GasLimit:  sizeFixtureGasLimit,
		StateRoot: root,
	}
	parent.ComputeHash()

	proposer := fixture.proposer(t, 0)
	builder := NewBlockBuilder(&BlockBuilderParams{
		Parent:    parent,
		Executor:  executor,
		Coinbase:  types.BytesToAddress(proposer.ID()),
		GasLimit:  sizeFixtureGasLimit,
		BlockTime: 2 * time.Second,
		Logger:    logger,
	})
	require.NoError(t, builder.Reset())

	privateKey, err := fixture.accounts[0].GetEcdsaPrivateKey()
	require.NoError(t, err)

	txSigner := chaincrypto.NewSigner(forks.At(1), 0)
	to := types.StringToAddress("0x0000000000000000000000000000000000009700")
	gasPerTransaction := uint64(21_000 + 4*dataPerTransaction)

	for i := 0; i < transactionCount; i++ {
		tx, signErr := txSigner.SignTx(&types.Transaction{
			Nonce:    uint64(i),
			To:       &to,
			Value:    big.NewInt(0),
			GasPrice: big.NewInt(2),
			Gas:      gasPerTransaction,
			Input:    make([]byte, dataPerTransaction),
		}, privateKey)
		require.NoError(t, signErr)

		recovered, recoverErr := txSigner.Sender(tx)
		require.NoError(t, recoverErr)
		require.Equal(t, sender, recovered)
		tx.From = recovered

		require.LessOrEqual(t, len(tx.MarshalRLP()), 128*1024)
		require.NoError(t, builder.WriteTx(tx))
	}

	validatorsHash, err := fixture.validators.Hash()
	require.NoError(t, err)

	extra := (&Extra{
		Checkpoint: &CheckpointData{
			EpochNumber:           1,
			BlockRound:            0,
			CurrentValidatorsHash: validatorsHash,
			NextValidatorsHash:    validatorsHash,
		},
	}).MarshalRLPTo(nil)

	fullBlock, err := builder.Build(func(header *types.Header) {
		header.ExtraData = extra
		header.MixHash = INNBCBFTMixDigest
	})
	require.NoError(t, err)
	require.Len(t, fullBlock.Receipts, transactionCount)
	require.Equal(t, uint64(transactionCount)*gasPerTransaction, fullBlock.Block.Header.GasUsed)
	require.LessOrEqual(t, fullBlock.Block.Header.GasUsed, sizeFixtureGasLimit)

	fixture.block = fullBlock.Block
	fixture.raw = fullBlock.Block.MarshalRLP()
	require.LessOrEqual(t, len(fixture.raw), sizeMaxMessage)

	return fixture
}

func (f *sizeFixture) proposer(t *testing.T, round uint64) *consensusRuntime {
	t.Helper()

	address, err := NewProposerSnapshot(1, f.validators).CalcProposer(round, 1)
	require.NoError(t, err)

	for _, runtime := range f.runtimes {
		if bytes.Equal(runtime.ID(), address.Bytes()) {
			return runtime
		}
	}

	t.Fatal("calculated proposer is absent from the fixture")
	return nil
}

func (f *sizeFixture) quorum() int {
	return 2*len(f.runtimes)/3 + 1
}

func sizeCheckMessage(t *testing.T, message *ibftproto.Message) {
	t.Helper()

	require.NotNil(t, message)
	require.LessOrEqual(t, protobuf.Size(message), sizeMaxMessage)
	require.True(t, isStructurallyValidIBFTMessage(message))
}

func (f *sizeFixture) proposal(
	t *testing.T,
	round uint64,
	pc *ibftproto.PreparedCertificate,
) *ibftproto.Message {
	t.Helper()

	var certificate *ibftproto.RoundChangeCertificate
	if round > 0 {
		require.NotNil(t, pc)
		require.NotNil(t, pc.ProposalMessage)

		/*
			Check an upper estimate before the production builders marshal
			and sign the repeated evidence. Fixture rounds remain small.
		*/
		estimate := len(f.raw) +
			f.quorum()*(len(f.raw)+protobuf.Size(pc)+2048) + 2048
		require.LessOrEqual(t, estimate, sizeMaxMessage)

		certificate = &ibftproto.RoundChangeCertificate{}
		for _, runtime := range f.runtimes[:f.quorum()] {
			message := runtime.BuildRoundChangeMessage(
				pc.ProposalMessage.GetPreprepareData().Proposal,
				pc,
				&ibftproto.View{Height: 1, Round: round},
			)
			sizeCheckMessage(t, message)
			require.True(t, f.runtimes[0].IsValidValidator(message))
			certificate.RoundChangeMessages = append(
				certificate.RoundChangeMessages,
				message,
			)
		}
	}

	message := f.proposer(t, round).BuildPrePrepareMessage(
		f.raw,
		certificate,
		&ibftproto.View{Height: 1, Round: round},
	)
	sizeCheckMessage(t, message)
	require.True(t, f.runtimes[0].IsValidValidator(message))

	return message
}

type sizeBackend struct {
	*consensusRuntime
	ready             chan struct{}
	readyOnce         sync.Once
	rejected          chan struct{}
	rejectedOnce      sync.Once
	observeBadPrepare bool
	executions        atomic.Int32
	insertions        atomic.Int32
}

var _ ibftcore.Backend = (*sizeBackend)(nil)

func (b *sizeBackend) GetVotingPowers(height uint64) (map[string]*big.Int, error) {
	powers, err := b.consensusRuntime.GetVotingPowers(height)
	b.readyOnce.Do(func() { close(b.ready) })
	return powers, err
}

func (b *sizeBackend) IsValidValidator(message *ibftproto.Message) bool {
	valid := b.consensusRuntime.IsValidValidator(message)
	if b.observeBadPrepare && !valid && message != nil &&
		message.Type == ibftproto.MessageType_PREPARE &&
		bytes.Equal(message.Signature, []byte{1}) {
		b.rejectedOnce.Do(func() { close(b.rejected) })
	}
	return valid
}

func (b *sizeBackend) ExecuteProposal(raw []byte) *ibftcore.ProposalExecution {
	b.executions.Add(1)
	return &ibftcore.ProposalExecution{Value: append([]byte(nil), raw...)}
}

func (*sizeBackend) IsCurrentExecution(
	raw []byte,
	execution *ibftcore.ProposalExecution,
) bool {
	if execution == nil {
		return false
	}
	value, ok := execution.Value.([]byte)
	return ok && bytes.Equal(raw, value)
}

func (*sizeBackend) BuildExecutedProposal(
	*ibftproto.View,
) ([]byte, *ibftcore.ProposalExecution) {
	return nil, nil
}

func (b *sizeBackend) InsertProposal(
	*ibftproto.Proposal,
	[]*ibftmessages.CommittedSeal,
	*ibftcore.ProposalExecution,
) error {
	b.insertions.Add(1)
	return errors.New("measurement fixture must not finalize a block")
}

type sizeCaptureTransport struct {
	ctx      context.Context
	messages chan *ibftproto.Message
}

func (transport *sizeCaptureTransport) Multicast(message *ibftproto.Message) {
	select {
	case transport.messages <- message:
	case <-transport.ctx.Done():
	}
}

type sizeHarness struct {
	engine    *ibftcore.IBFT
	backend   *sizeBackend
	transport *sizeCaptureTransport
	stop      func()
}

func sizeWaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()

	timer := time.NewTimer(sizeWatchdog)
	defer timer.Stop()

	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(description)
	}
}

func newSizeHarness(
	t *testing.T,
	fixture *sizeFixture,
	candidate *ibftproto.Message,
	observeBadPrepare bool,
) *sizeHarness {
	t.Helper()

	var observer *consensusRuntime
	proposer0 := fixture.proposer(t, 0)
	for _, runtime := range fixture.runtimes {
		if !bytes.Equal(runtime.ID(), proposer0.ID()) &&
			!bytes.Equal(runtime.ID(), candidate.From) {
			observer = runtime
			break
		}
	}
	require.NotNil(t, observer)

	ctx, cancel := context.WithCancel(context.Background())
	backend := &sizeBackend{
		consensusRuntime:  observer,
		ready:             make(chan struct{}),
		rejected:          make(chan struct{}),
		observeBadPrepare: observeBadPrepare,
	}
	transport := &sizeCaptureTransport{
		ctx:      ctx,
		messages: make(chan *ibftproto.Message, 32),
	}
	engine := ibftcore.NewIBFT(hclog.NewNullLogger(), backend, transport)

	/* Drive round changes explicitly; do not modify global timeout values. */
	engine.ExtendRoundTimeout(time.Hour)

	done := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			sizeWaitSignal(t, done, "measurement consensus sequence did not stop")
		})
	}
	t.Cleanup(stop)

	go func() {
		defer close(done)
		engine.RunSequence(ctx, 1)
	}()

	sizeWaitSignal(t, backend.ready, "consensus validator initialization did not run")
	engine.AddMessage(candidate)

	return &sizeHarness{
		engine:    engine,
		backend:   backend,
		transport: transport,
		stop:      stop,
	}
}

func (h *sizeHarness) waitMessage(
	t *testing.T,
	messageType ibftproto.MessageType,
	round uint64,
) *ibftproto.Message {
	t.Helper()

	timer := time.NewTimer(sizeWatchdog)
	defer timer.Stop()

	for {
		select {
		case message := <-h.transport.messages:
			require.NotNil(t, message)
			require.True(t, h.backend.consensusRuntime.IsValidValidator(message))
			if message.Type == messageType && message.View.Round == round {
				return message
			}
		case <-timer.C:
			t.Fatalf("did not observe %s at round %d", messageType, round)
		}
	}
}

func sizeAcceptProposal(
	t *testing.T,
	fixture *sizeFixture,
	candidate *ibftproto.Message,
) {
	t.Helper()

	harness := newSizeHarness(t, fixture, candidate, false)
	harness.waitMessage(t, ibftproto.MessageType_PREPARE, candidate.View.Round)
	harness.stop()

	require.Positive(t, harness.backend.executions.Load())
	require.Zero(t, harness.backend.insertions.Load())
	require.Nil(t, harness.engine.SequenceFailure())
}

func sizePrepareAndAdvance(
	t *testing.T,
	fixture *sizeFixture,
	candidate *ibftproto.Message,
	advances int,
) []*ibftproto.Message {
	t.Helper()

	require.Greater(t, advances, 0)
	require.LessOrEqual(t, advances, sizeMaxReprepares)

	harness := newSizeHarness(t, fixture, candidate, false)
	round := candidate.View.Round
	harness.waitMessage(t, ibftproto.MessageType_PREPARE, round)

	data := candidate.GetPreprepareData()
	prepares := make([]*ibftproto.Message, 0, fixture.quorum()-1)
	for _, runtime := range fixture.runtimes {
		if bytes.Equal(runtime.ID(), candidate.From) {
			continue
		}
		message := runtime.BuildPrepareMessage(
			data.ProposalHash,
			&ibftproto.View{Height: 1, Round: round},
		)
		require.NotNil(t, message)
		prepares = append(prepares, message)
		harness.engine.AddMessage(message)
		if len(prepares) == fixture.quorum()-1 {
			break
		}
	}
	harness.waitMessage(t, ibftproto.MessageType_COMMIT, round)

	/*
		This equivalent signed quorum is used only to announce the first
		future round. Capture the core's own retained PC from its response.
	*/
	pc := &ibftproto.PreparedCertificate{
		ProposalMessage: candidate,
		PrepareMessages: prepares,
	}
	prepared := data.Proposal
	results := make([]*ibftproto.Message, 0, advances)

	for step := 1; step <= advances; step++ {
		nextRound := round + uint64(step)
		require.LessOrEqual(t,
			len(fixture.raw)+protobuf.Size(pc)+2048,
			sizeMaxMessage,
		)

		/* A weak quorum advances the round without forming another proposal. */
		needed := len(fixture.runtimes)/3 + 1
		sent := 0
		for _, runtime := range fixture.runtimes {
			if bytes.Equal(runtime.ID(), harness.backend.ID()) {
				continue
			}
			message := runtime.BuildRoundChangeMessage(
				prepared,
				pc,
				&ibftproto.View{Height: 1, Round: nextRound},
			)
			sizeCheckMessage(t, message)
			harness.engine.AddMessage(message)
			sent++
			if sent == needed {
				break
			}
		}

		message := harness.waitMessage(t, ibftproto.MessageType_ROUND_CHANGE, nextRound)
		rc := message.GetRoundChangeData()
		require.NotNil(t, rc)
		require.NotNil(t, rc.LatestPreparedCertificate)
		require.Same(t, candidate, rc.LatestPreparedCertificate.ProposalMessage)
		require.Len(t, rc.LatestPreparedCertificate.PrepareMessages, fixture.quorum()-1)
		require.True(t, fixture.runtimes[0].IsValidProposalHash(
			rc.LastPreparedProposal,
			data.ProposalHash,
		))

		results = append(results, message)
		pc = rc.LatestPreparedCertificate
		prepared = rc.LastPreparedProposal
	}

	harness.stop()
	require.Positive(t, harness.backend.executions.Load())
	require.Zero(t, harness.backend.insertions.Load())
	require.Nil(t, harness.engine.SequenceFailure())

	return results
}

func sizeCanonicalPC(
	t *testing.T,
	message *ibftproto.Message,
) *ibftproto.PreparedCertificate {
	t.Helper()

	actual := message.GetRoundChangeData().LatestPreparedCertificate
	require.NotNil(t, actual)

	/*
		The PC container is unsigned. Copy its prepare slice to make the
		next fixture deterministic without altering any signed message.
		Do not mutate the certificate inside the captured ROUND_CHANGE.
	*/
	result := &ibftproto.PreparedCertificate{
		ProposalMessage: actual.ProposalMessage,
		PrepareMessages: append([]*ibftproto.Message(nil), actual.PrepareMessages...),
	}
	sort.Slice(result.PrepareMessages, func(i, j int) bool {
		return bytes.Compare(result.PrepareMessages[i].From, result.PrepareMessages[j].From) < 0
	})
	require.Equal(t, protobuf.Size(actual), protobuf.Size(result))
	return result
}

func sizeBlockCopies(
	t *testing.T,
	root *ibftproto.Message,
	raw []byte,
) int {
	t.Helper()

	pending := []*ibftproto.Message{root}
	copies, visited := 0, 0

	for len(pending) > 0 {
		visited++
		require.LessOrEqual(t, visited, 4096, "fixture evidence traversal exceeded its bound")

		message := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		require.NotNil(t, message)

		if pp := message.GetPreprepareData(); pp != nil {
			require.NotNil(t, pp.Proposal)
			require.Equal(t, raw, pp.Proposal.RawProposal)
			copies++
			if pp.Certificate != nil {
				pending = append(pending, pp.Certificate.RoundChangeMessages...)
			}
		}

		if rc := message.GetRoundChangeData(); rc != nil {
			if rc.LastPreparedProposal != nil {
				require.Equal(t, raw, rc.LastPreparedProposal.RawProposal)
				copies++
			}
			if rc.LatestPreparedCertificate != nil {
				pending = append(pending, rc.LatestPreparedCertificate.ProposalMessage)
			}
		}
	}

	return copies
}

func sizeLibp2pKey(t *testing.T, scalar byte) libcrypto.PrivKey {
	t.Helper()

	raw := make([]byte, 32)
	raw[31] = scalar
	key, err := libcrypto.UnmarshalSecp256k1PrivateKey(raw)
	require.NoError(t, err)
	return key
}

type sizeWireMeasurement struct {
	payload    []byte
	rpcBytes   int
	frameBytes int
}

func sizeMeasureWire(
	t *testing.T,
	message *ibftproto.Message,
) sizeWireMeasurement {
	t.Helper()

	sizeCheckMessage(t, message)
	payload, err := protobuf.Marshal(message)
	require.NoError(t, err)

	key := sizeLibp2pKey(t, 31)
	id, err := peer.IDFromPrivateKey(key)
	require.NoError(t, err)

	sequence := make([]byte, 8)
	binary.BigEndian.PutUint64(sequence, 1)
	topic := pbftProto
	envelope := &pubsubpb.Message{
		From:  []byte(id),
		Data:  payload,
		Seqno: sequence,
		Topic: &topic,
	}
	unsigned, err := envelope.Marshal()
	require.NoError(t, err)

	signedBytes := append([]byte(pubsub.SignPrefix), unsigned...)
	envelope.Signature, err = key.Sign(signedBytes)
	require.NoError(t, err)

	valid, err := key.GetPublic().Verify(signedBytes, envelope.Signature)
	require.NoError(t, err)
	require.True(t, valid)

	publicKey, _ := id.ExtractPublicKey()
	if publicKey == nil {
		envelope.Key, err = libcrypto.MarshalPublicKey(key.GetPublic())
		require.NoError(t, err)
	}

	rpc := &pubsubpb.RPC{Publish: []*pubsubpb.Message{envelope}}
	require.LessOrEqual(t, rpc.Size(), sizeMaxMessage+2048)
	encoded, err := rpc.Marshal()
	require.NoError(t, err)
	require.Equal(t, rpc.Size(), len(encoded))

	var prefix [binary.MaxVarintLen64]byte
	prefixBytes := binary.PutUvarint(prefix[:], uint64(len(encoded)))

	return sizeWireMeasurement{
		payload:    payload,
		rpcBytes:   len(encoded),
		frameBytes: prefixBytes + len(encoded),
	}
}

func sizeLogMessage(
	t *testing.T,
	label string,
	fixture *sizeFixture,
	message *ibftproto.Message,
) sizeWireMeasurement {
	t.Helper()

	wire := sizeMeasureWire(t, message)
	copies := sizeBlockCopies(t, message, fixture.raw)

	t.Logf(
		"%s validators=%d quorum=%d round=%d block=%d copies=%d "+
			"copied_block_bytes=%d consensus_payload=%d signed_gossip_rpc=%d framed_rpc=%d",
		label, len(fixture.runtimes), fixture.quorum(), message.View.Round,
		len(fixture.raw), copies, copies*len(fixture.raw),
		len(wire.payload), wire.rpcBytes, wire.frameBytes,
	)
	return wire
}

func TestConsensusWireSize_FreshGasValidProposal(t *testing.T) {
	fixture := newSizeFixture(t, 4, 9, 120_000)
	require.Equal(t, uint64(4_509_000), fixture.block.Header.GasUsed)
	require.Greater(t, len(fixture.raw), pubsub.DefaultMaxMessageSize)

	candidate := fixture.proposal(t, 0, nil)
	sizeAcceptProposal(t, fixture, candidate)
	wire := sizeLogMessage(t, "fresh_large_block", fixture, candidate)

	require.Greater(t, wire.rpcBytes, pubsub.DefaultMaxMessageSize)
	require.Equal(t, 1, sizeBlockCopies(t, candidate, fixture.raw))
}

func TestConsensusWireSize_TimeoutsWithoutNewPreparation(t *testing.T) {
	fixture := newSizeFixture(t, 4, 1, 32*1024)
	candidate := fixture.proposal(t, 0, nil)
	results := sizePrepareAndAdvance(t, fixture, candidate, 3)

	first := results[0].GetRoundChangeData()
	firstSize := protobuf.Size(results[0])

	for _, message := range results {
		rc := message.GetRoundChangeData()
		require.Same(t, first.LatestPreparedCertificate, rc.LatestPreparedCertificate)
		require.Same(t, first.LastPreparedProposal, rc.LastPreparedProposal)
		require.Equal(t, firstSize, protobuf.Size(message))
		require.Equal(t, 2, sizeBlockCopies(t, message, fixture.raw))
		sizeLogMessage(t, "timeout_without_new_prepare_quorum", fixture, message)
	}
}

func TestConsensusWireSize_SuccessivePreparedReproposals(t *testing.T) {
	for _, testCase := range []struct {
		validators int
		data       int
	}{
		{validators: 4, data: 32 * 1024},
		{validators: 7, data: 4 * 1024},
	} {
		t.Run(fmt.Sprintf("validators_%d", testCase.validators), func(t *testing.T) {
			fixture := newSizeFixture(t, testCase.validators, 1, testCase.data)
			var pc *ibftproto.PreparedCertificate
			expectedCopies := 1

			for round := 0; round <= sizeMaxReprepares; round++ {
				candidate := fixture.proposal(t, uint64(round), pc)
				require.Equal(t, expectedCopies, sizeBlockCopies(t, candidate, fixture.raw))
				sizeLogMessage(t, "successive_prepared_reproposal", fixture, candidate)

				results := sizePrepareAndAdvance(t, fixture, candidate, 1)
				sizeLogMessage(t, "retained_prepared_round_change", fixture, results[0])
				pc = sizeCanonicalPC(t, results[0])

				expectedCopies = 1 + fixture.quorum()*(1+expectedCopies)
			}
		})
	}
}

func TestConsensusWireSize_InvalidPreparedSignatureRejected(t *testing.T) {
	fixture := newSizeFixture(t, 4, 1, 4*1024)
	initial := fixture.proposal(t, 0, nil)
	results := sizePrepareAndAdvance(t, fixture, initial, 1)
	pc := sizeCanonicalPC(t, results[0])

	require.LessOrEqual(t, protobuf.Size(pc), sizeMaxMessage)
	badPC := protobuf.Clone(pc).(*ibftproto.PreparedCertificate)
	badPC.PrepareMessages[0].Signature = []byte{1}

	/* Sign valid outer messages around the deliberately invalid inner signature. */
	candidate := fixture.proposal(t, 1, badPC)
	harness := newSizeHarness(t, fixture, candidate, true)
	sizeWaitSignal(t, harness.backend.rejected,
		"production certificate validation did not inspect the invalid prepare signature")
	harness.stop()

	require.Zero(t, harness.backend.executions.Load())
	require.Zero(t, harness.backend.insertions.Load())
	require.Nil(t, harness.engine.SequenceFailure())

	for {
		select {
		case message := <-harness.transport.messages:
			require.NotNil(t, message)
			require.NotEqual(t, ibftproto.MessageType_PREPARE, message.Type)
			require.NotEqual(t, ibftproto.MessageType_COMMIT, message.Type)
		default:
			return
		}
	}
}

/* FINISH ALPHA CHANGE 2892 -- measure signed consensus evidence without changing production limits -- */
