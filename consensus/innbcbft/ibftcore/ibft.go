/* START ALPHA CHANGE 1851 -- move IBFT consensus into INNBC ownership -- */
package ibftcore

/* FINISH ALPHA CHANGE 1851 -- move IBFT consensus into INNBC ownership -- */

import (
	/* START ALPHA CHANGE 2566 -- compare prepared hashes without repeated block decoding -- */
	"bytes"
	/* FINISH ALPHA CHANGE 2566 -- compare prepared hashes without repeated block decoding -- */
	"context"
	"errors"
	"sync"
	"time"

	"github.com/armon/go-metrics"

	/* START ALPHA CHANGE 1861 -- use INNBC-owned IBFT message packages -- */
	messages "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftmessages"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	/* FINISH ALPHA CHANGE 1861 -- use INNBC-owned IBFT message packages -- */)

// Logger represents the logger behaviour
type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Error(msg string, args ...any)
}

// Messages represents the message managing behaviour
type Messages interface {
	// Messages modifiers //
	AddMessage(message *proto.Message)
	PruneByHeight(height uint64)
	/* START ALPHA CHANGE 1886 -- expose round-aware consensus message pruning -- */
	PruneByView(view *proto.View)
	/* FINISH ALPHA CHANGE 1886 -- expose round-aware consensus message pruning -- */

	SignalEvent(messageType proto.MessageType, view *proto.View)

	// Messages fetchers //
	GetValidMessages(
		view *proto.View,
		messageType proto.MessageType,
		isValid func(*proto.Message) bool,
	) []*proto.Message
	GetExtendedRCC(
		height uint64,
		isValidMessage func(message *proto.Message) bool,
		isValidRCC func(round uint64, msgs []*proto.Message) bool,
	) []*proto.Message
	/* START ALPHA CHANGE 1904 -- remove unused inherited round-count selector -- */
	/* FINISH ALPHA CHANGE 1904 -- remove unused inherited round-count selector -- */

	// Messages subscription handlers //
	Subscribe(details messages.SubscriptionDetails) *messages.Subscription
	Unsubscribe(id messages.SubscriptionID)
}

const (
	round0Timeout = 10 * time.Second

	/* START ALPHA CHANGE 1887 -- bound admitted consensus heights and rounds -- */
	maxFutureHeightDistance = uint64(1)
	maxFutureRoundDistance  = uint64(8)
	maxRoundTimeout         = time.Duration(1<<63 - 1)
	/* FINISH ALPHA CHANGE 1887 -- bound admitted consensus heights and rounds -- */
)

var (
	errTimeoutExpired = errors.New("round timeout expired")
)

// IBFT represents a single instance of the IBFT state machine
type IBFT struct {
	// log is the logger instance
	log Logger

	// state is the current IBFT node state
	state *state

	// messages is the message storage layer
	messages Messages

	// backend is the reference to the
	// Backend implementation
	backend Backend

	// transport is the reference to the
	// Transport implementation
	transport Transport

	// roundDone is the channel used for signalizing
	// consensus finalization upon a certain sequence
	roundDone chan struct{}

	// roundExpired is the channel used for signalizing
	// round changing events
	roundExpired chan struct{}

	// newProposal is the channel used for signalizing
	// when new proposals for a view greater than the current
	// one arrive
	newProposal chan newProposalEvent

	// roundCertificate is the channel used for signalizing
	// when sufficient authenticated evidence for a greater round is present
	roundCertificate chan roundChangeEvent

	//	User configured additional timeout for each round of consensus
	additionalTimeout time.Duration

	// baseRoundTimeout is the base round timeout for each round of consensus
	baseRoundTimeout time.Duration

	// wg is a simple barrier used for synchronizing
	// state modification routines
	wg sync.WaitGroup

	// validatorManager keeps quorumSize and voting power information
	validatorManager *ValidatorManager

	/* START ALPHA CHANGE 2584 -- retain terminal committed-proposal failures -- */
	proposalFailure chan struct{}
	failureMutex    sync.Mutex
	failureErr      error
	/* FINISH ALPHA CHANGE 2584 -- retain terminal committed-proposal failures -- */
}

// NewIBFT creates a new instance of the IBFT consensus protocol
func NewIBFT(
	log Logger,
	backend Backend,
	transport Transport,
) *IBFT {
	return &IBFT{
		log:              log,
		backend:          backend,
		transport:        transport,
		messages:         messages.NewMessages(),
		roundDone:        make(chan struct{}),
		roundExpired:     make(chan struct{}),
		newProposal:      make(chan newProposalEvent),
		roundCertificate: make(chan roundChangeEvent),
		/* START ALPHA CHANGE 2584 -- initialize committed-proposal failure notification -- */
		proposalFailure: make(chan struct{}),
		/* FINISH ALPHA CHANGE 2584 -- initialize committed-proposal failure notification -- */
		state: &state{
			view: &proto.View{
				Height: 0,
				Round:  0,
			},
			seals:        make([]*messages.CommittedSeal, 0),
			roundStarted: false,
			name:         newRound,
		},
		baseRoundTimeout: round0Timeout,
		validatorManager: NewValidatorManager(backend, log),
	}
}

/* START ALPHA CHANGE 1905 -- use the INNBCBFT metric namespace -- */
// SetMeasurementTime records an INNBCBFT operation duration.
func SetMeasurementTime(prefix string, startTime time.Time) {
	metrics.SetGauge([]string{"innbcbft", prefix, "duration"}, float32(time.Since(startTime).Seconds()))
}

/* FINISH ALPHA CHANGE 1905 -- use the INNBCBFT metric namespace -- */

// startRoundTimer starts the exponential round timer, based on the
// passed in round number
func (i *IBFT) startRoundTimer(ctx context.Context, round uint64) {
	defer i.wg.Done()

	startTime := time.Now()

	roundTimeout := getRoundTimeout(i.baseRoundTimeout, i.additionalTimeout, round)

	//	Create a new timer instance
	timer := time.NewTimer(roundTimeout)

	select {
	case <-ctx.Done():
		SetMeasurementTime("round", startTime)
		// Stop signal received, stop the timer
		timer.Stop()
	case <-timer.C:
		// Timer expired, alert the round change channel to move
		// to the next round
		i.signalRoundExpired(ctx)
	}
}

// signalRoundExpired notifies the sequence routine (RunSequence) that it
// should move to a new round. The quit channel is used to abort this call
// if another routine has already signaled a round change request.
func (i *IBFT) signalRoundExpired(ctx context.Context) {
	select {
	case i.roundExpired <- struct{}{}:
	case <-ctx.Done():
	}
}

// signalRoundDone notifies the sequence routine (RunSequence) that the
// consensus sequence is finished
func (i *IBFT) signalRoundDone(ctx context.Context) {
	select {
	case i.roundDone <- struct{}{}:
	case <-ctx.Done():
	}
}

/* START ALPHA CHANGE 1888 -- distinguish full certificates from weak catch-up evidence -- */
type roundChangeEvent struct {
	round      uint64
	weakQuorum bool
}

// signalNewRCC notifies the sequence routine that sufficient authenticated
// round-change evidence for a higher round has appeared.
func (i *IBFT) signalNewRCC(ctx context.Context, event roundChangeEvent) {
	select {
	case i.roundCertificate <- event:
	case <-ctx.Done():
	}
}

/* FINISH ALPHA CHANGE 1888 -- distinguish full certificates from weak catch-up evidence -- */

/* START ALPHA CHANGE 1902 -- centralize authenticated future-round adoption -- */
func (i *IBFT) applyRoundChangeEvent(height uint64, event roundChangeEvent) {
	i.moveToNewRound(event.round)

	if event.weakQuorum {
		i.sendRoundChangeMessage(height, event.round)
	}
}

/* FINISH ALPHA CHANGE 1902 -- centralize authenticated future-round adoption -- */

/* START ALPHA CHANGE 2584 -- carry future proposal execution ownership through adoption -- */
type newProposalEvent struct {
	proposalMessage *proto.Message
	execution       *ProposalExecution
	round           uint64
}

/* FINISH ALPHA CHANGE 2584 -- carry future proposal execution ownership through adoption -- */

// signalNewProposal notifies the sequence routine (RunSequence) that
// a valid proposal for a higher round appeared
func (i *IBFT) signalNewProposal(ctx context.Context, event newProposalEvent) {
	select {
	case i.newProposal <- event:
	case <-ctx.Done():
	}
}

/* START ALPHA CHANGE 1900 -- scan the complete bounded future-proposal window -- */
// highestValidFutureProposal returns the valid proposal at the highest admitted
// future round. The scan is strictly bounded by maxFutureRoundDistance.
/* START ALPHA CHANGE 2584 -- preserve the message-only inspection helper -- */
func (i *IBFT) highestValidFutureProposal(view *proto.View) (*proto.Message, uint64) {
	candidate, round := i.highestValidFutureCandidate(view)
	if candidate == nil {
		return nil, 0
	}

	return candidate.message, round
}

/* FINISH ALPHA CHANGE 2584 -- preserve the message-only inspection helper -- */

// watchForFutureProposal listens for valid proposal messages in the complete
// bounded future-round window. Subscribing before the first scan prevents a
// message from being missed between storage inspection and subscription.
func (i *IBFT) watchForFutureProposal(ctx context.Context) {
	defer i.wg.Done()

	view := i.state.getView()
	if view == nil || view.Round == ^uint64(0) {
		return
	}

	sub := i.messages.Subscribe(messages.SubscriptionDetails{
		MessageType: proto.MessageType_PREPREPARE,
		View: &proto.View{
			Height: view.Height,
			Round:  view.Round + 1,
		},
		HasMinRound: true,
	})
	defer i.messages.Unsubscribe(sub.ID)

	for {
		/* START ALPHA CHANGE 2584 -- publish the future candidate with its execution result -- */
		candidate, round := i.highestValidFutureCandidate(view)
		if candidate != nil {
			i.signalNewProposal(ctx, newProposalEvent{
				proposalMessage: candidate.message,
				execution:       candidate.execution,
				round:           round,
			})

			return
		}
		/* FINISH ALPHA CHANGE 2584 -- publish the future candidate with its execution result -- */

		select {
		case <-ctx.Done():
			return
		case _, open := <-sub.SubCh:
			if !open {
				return
			}
		}
	}
}

/* FINISH ALPHA CHANGE 1900 -- scan the complete bounded future-proposal window -- */

/* START ALPHA CHANGE 1889 -- catch up to bounded future rounds using authenticated evidence -- */
/* START ALPHA CHANGE 1901 -- select the highest qualifying future-round evidence -- */
func roundChangeCertificateRound(
	certificate *proto.RoundChangeCertificate,
) (uint64, bool) {
	if certificate == nil ||
		len(certificate.RoundChangeMessages) == 0 ||
		certificate.RoundChangeMessages[0] == nil ||
		certificate.RoundChangeMessages[0].View == nil {
		return 0, false
	}

	return certificate.RoundChangeMessages[0].View.Round, true
}

// highestFutureRoundChangeEvent compares the highest full-quorum certificate
// with the highest weak-quorum evidence. The highest qualifying round wins;
// full quorum wins when both forms of evidence identify the same round.
func (i *IBFT) highestFutureRoundChangeEvent(
	view *proto.View,
) (roundChangeEvent, bool) {
	fullCertificate := i.handleFutureRoundChangeMessages(
		view,
		i.validatorManager.HasQuorum,
	)
	weakCertificate := i.handleFutureRoundChangeMessages(
		view,
		i.validatorManager.HasWeakQuorum,
	)

	fullRound, hasFullCertificate := roundChangeCertificateRound(fullCertificate)
	weakRound, hasWeakCertificate := roundChangeCertificateRound(weakCertificate)

	switch {
	case !hasFullCertificate && !hasWeakCertificate:
		return roundChangeEvent{}, false
	case hasFullCertificate &&
		(!hasWeakCertificate || fullRound >= weakRound):
		return roundChangeEvent{
			round: fullRound,
		}, true
	default:
		return roundChangeEvent{
			round:      weakRound,
			weakQuorum: true,
		}, true
	}
}

// watchForRoundChangeCertificates waits for bounded future round-change
// evidence. Full quorum remains authoritative at an equal round; higher weak
// evidence advances the local round and causes this validator to contribute
// its own ROUND_CHANGE message.
func (i *IBFT) watchForRoundChangeCertificates(ctx context.Context) {
	defer i.wg.Done()

	view := i.state.getView()
	if view == nil || view.Round == ^uint64(0) {
		return
	}

	sub := i.messages.Subscribe(messages.SubscriptionDetails{
		MessageType: proto.MessageType_ROUND_CHANGE,
		View: &proto.View{
			Height: view.Height,
			Round:  view.Round + 1,
		},
		HasMinRound: true,
	})
	defer i.messages.Unsubscribe(sub.ID)

	for {
		if event, found := i.highestFutureRoundChangeEvent(view); found {
			i.signalNewRCC(ctx, event)

			return
		}

		select {
		case <-ctx.Done():
			return
		case _, open := <-sub.SubCh:
			if !open {
				return
			}
		}
	}
}

/* FINISH ALPHA CHANGE 1901 -- select the highest qualifying future-round evidence -- */
/* FINISH ALPHA CHANGE 1889 -- catch up to bounded future rounds using authenticated evidence -- */

// RunSequence runs the IBFT sequence for the specified height
func (i *IBFT) RunSequence(ctx context.Context, h uint64) {
	/* START ALPHA CHANGE 2584 -- preserve the committed decision after insertion failure -- */
	if i.SequenceFailure() != nil {
		return
	}
	/* FINISH ALPHA CHANGE 2584 -- preserve the committed decision after insertion failure -- */

	startTime := time.Now()

	// Set the starting state data
	i.state.reset(h)

	if err := i.validatorManager.Init(h); err != nil {
		i.log.Error("failed to run sequence - validator manager init", "height", h, "error", err)

		return
	}

	// Prune messages for older heights
	i.messages.PruneByHeight(h)

	i.log.Info("sequence started", "height", h)
	defer i.log.Info("sequence done", "height", h)
	defer SetMeasurementTime("sequence", startTime)

	for {
		view := i.state.getView()

		i.log.Info("round started", "round", view.Round)

		currentRound := view.Round
		ctxRound, cancelRound := context.WithCancel(ctx)

		i.wg.Add(4)

		// Start the round timer worker
		go i.startRoundTimer(ctxRound, currentRound)

		//	Jump round on proposals from higher rounds
		go i.watchForFutureProposal(ctxRound)

		//	Jump round on certificates
		go i.watchForRoundChangeCertificates(ctxRound)

		// Start the state machine worker
		go i.startRound(ctxRound)

		teardown := func() {
			cancelRound()
			i.wg.Wait()
		}

		/* START ALPHA CHANGE 2584 -- resolve worker completion before future proposal adoption -- */
		var (
			future      *newProposalEvent
			certificate *roundChangeEvent
			expired     bool
			stop        bool
		)

		select {
		case event := <-i.newProposal:
			future = &event
		case event := <-i.roundCertificate:
			certificate = &event
		case <-i.roundExpired:
			expired = true
		case <-i.roundDone:
			stop = true
		case <-i.proposalFailure:
			stop = true
		case <-ctxRound.Done():
			stop = true
		}

		teardown()

		/*
			The current worker may have inserted or failed while teardown
			waited for speculative execution. Neither outcome may be
			overwritten by an already received future-round event.
		*/
		if i.SequenceFailure() != nil || i.state.wasInserted() ||
			stop || ctx.Err() != nil {
			return
		}

		switch {
		case future != nil:
			candidate := &proposalCandidate{
				message:   future.proposalMessage,
				execution: future.execution,
			}
			proposal := candidateProposal(candidate)
			if proposal == nil ||
				!i.backend.IsCurrentExecution(proposal.RawProposal, candidate.execution) {
				return
			}

			i.log.Info("received future proposal", "round", future.round)
			i.moveToNewRound(future.round)

			if !i.acceptProposal(candidate) {
				return
			}

			i.state.setRoundStarted(true)
			i.sendPrepareMessage(i.state.getView())

		case certificate != nil:
			i.log.Info(
				"received future round-change evidence",
				"round", certificate.round,
				"weakQuorum", certificate.weakQuorum,
			)
			i.applyRoundChangeEvent(h, *certificate)

		case expired:
			i.log.Info("round timeout expired", "round", currentRound)

			nextRound := currentRound + 1
			i.moveToNewRound(nextRound)
			i.sendRoundChangeMessage(h, nextRound)
		}
		/* FINISH ALPHA CHANGE 2584 -- resolve worker completion before future proposal adoption -- */
	}
}

// startRound runs the state machine loop for the current round
func (i *IBFT) startRound(ctx context.Context) {
	// Register this worker thread with the barrier
	defer i.wg.Done()

	i.state.newRound()

	var (
		id   = i.backend.ID()
		view = i.state.getView()
	)

	// Check if any block needs to be proposed
	if i.backend.IsProposer(id, view.Height, view.Round) {
		i.log.Info("we are the proposer")

		/* START ALPHA CHANGE 2584 -- adopt locally constructed execution results explicitly -- */
		candidate := i.buildCandidate(ctx, view)
		if candidate == nil {
			i.log.Error("unable to build proposal")

			return
		}

		if ctx.Err() != nil || !i.acceptProposal(candidate) {
			return
		}

		i.log.Debug("block proposal accepted")
		i.sendPreprepareMessage(candidate.message)
		i.log.Debug("pre-prepare message multicasted")
		/* FINISH ALPHA CHANGE 2584 -- adopt locally constructed execution results explicitly -- */
	}

	i.runStates(ctx)
}

// waitForRCC waits for valid RCC for the specified height and round
func (i *IBFT) waitForRCC(
	ctx context.Context,
	height,
	round uint64,
) *proto.RoundChangeCertificate {
	var (
		view = &proto.View{
			Height: height,
			Round:  round,
		}

		sub = i.subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_ROUND_CHANGE,
				View:        view,
			},
		)
	)

	defer i.messages.Unsubscribe(sub.ID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.SubCh:
			rcc := i.handleRoundChangeMessage(view)
			if rcc == nil {
				continue
			}

			return rcc
		}
	}
}

/* START ALPHA CHANGE 1891 -- validate exact and future round-change evidence separately -- */
type roundChangeQuorum func(map[string]struct{}) bool

// handleRoundChangeMessage returns a full certificate for exactly the
// requested view. It remains the path used by proposal construction.
func (i *IBFT) handleRoundChangeMessage(view *proto.View) *proto.RoundChangeCertificate {
	return i.handleRoundChangeMessages(view, false, i.validatorManager.HasQuorum)
}

// handleFutureRoundChangeMessages returns the highest qualifying future round
// within the admitted window.
func (i *IBFT) handleFutureRoundChangeMessages(
	view *proto.View,
	hasQuorum roundChangeQuorum,
) *proto.RoundChangeCertificate {
	return i.handleRoundChangeMessages(view, true, hasQuorum)
}

func (i *IBFT) handleRoundChangeMessages(
	view *proto.View,
	future bool,
	hasQuorum roundChangeQuorum,
) *proto.RoundChangeCertificate {
	if view == nil || hasQuorum == nil {
		return nil
	}

	height := view.Height
	hasAcceptedProposal := i.state.getProposal() != nil

	isValidMsgFn := func(msg *proto.Message) bool {
		if msg == nil ||
			msg.View == nil ||
			msg.Type != proto.MessageType_ROUND_CHANGE ||
			msg.GetRoundChangeData() == nil ||
			msg.View.Height != height {
			return false
		}

		// Revalidate the outer sender against the active validator set. This is
		// required for messages admitted at H+1 before that height became active.
		if !i.backend.IsValidValidator(msg) {
			return false
		}

		proposal := messages.ExtractLastPreparedProposal(msg)
		certificate := messages.ExtractLatestPC(msg)

		if !i.validPC(certificate, msg.View.Round, height) {
			return false
		}

		return i.proposalMatchesCertificate(proposal, certificate)
	}

	isValidRCCFn := func(round uint64, msgs []*proto.Message) bool {
		if future {
			if round <= view.Round ||
				round-view.Round > maxFutureRoundDistance {
				return false
			}
		} else if round != view.Round {
			return false
		}

		if round == view.Round && hasAcceptedProposal {
			return false
		}

		return hasQuorum(convertMessageToAddressSet(msgs))
	}

	extendedRCC := i.messages.GetExtendedRCC(
		height,
		isValidMsgFn,
		isValidRCCFn,
	)
	if extendedRCC == nil {
		return nil
	}

	return &proto.RoundChangeCertificate{
		RoundChangeMessages: extendedRCC,
	}
}

/* FINISH ALPHA CHANGE 1891 -- validate exact and future round-change evidence separately -- */

/* START ALPHA CHANGE 2566 -- bind prepared proposal rounds and avoid repeated block decoding -- */
func (i *IBFT) proposalMatchesCertificate(
	proposal *proto.Proposal,
	certificate *proto.PreparedCertificate,
) bool {
	if certificate == nil {
		return proposal == nil
	}

	if proposal == nil || certificate.ProposalMessage == nil {
		return false
	}

	message := certificate.ProposalMessage
	data := message.GetPreprepareData()
	if message.Type != proto.MessageType_PREPREPARE ||
		message.View == nil || data == nil || data.Proposal == nil {
		return false
	}

	if proposal.Round != message.View.Round ||
		data.Proposal.Round != message.View.Round {
		return false
	}

	/* Callers verify the certificate separately. Decode this proposal once. */
	if !i.backend.IsValidProposalHash(proposal, data.ProposalHash) {
		return false
	}

	for _, prepareMessage := range certificate.PrepareMessages {
		if prepareMessage == nil ||
			prepareMessage.Type != proto.MessageType_PREPARE ||
			prepareMessage.GetPrepareData() == nil {
			return false
		}

		if !bytes.Equal(
			data.ProposalHash,
			prepareMessage.GetPrepareData().ProposalHash,
		) {
			return false
		}
	}

	return true
}

/* FINISH ALPHA CHANGE 2566 -- bind prepared proposal rounds and avoid repeated block decoding -- */

// runStates is the main loop which performs state transitions
func (i *IBFT) runStates(ctx context.Context) {
	var timeout error

	for {
		switch i.state.getStateName() {
		case newRound:
			timeout = i.runNewRound(ctx)
		case prepare:
			timeout = i.runPrepare(ctx)
		case commit:
			timeout = i.runCommit(ctx)
		case fin:
			/* START ALPHA CHANGE 2584 -- retain committed decisions when insertion fails -- */
			if err := i.runFin(); err != nil {
				i.log.Error("committed proposal insertion failed", "error", err)
				i.recordSequenceFailure(err)

				return
			}

			i.signalRoundDone(ctx)

			return
			/* FINISH ALPHA CHANGE 2584 -- retain committed decisions when insertion fails -- */
		}

		if timeout != nil {
			// Timeout received
			return
		}
	}
}

// runNewRound runs the New Round IBFT state
func (i *IBFT) runNewRound(ctx context.Context) error {
	i.log.Debug("enter: new round state")
	defer i.log.Debug("exit: new round state")

	var (
		// Grab the current view
		view = i.state.getView()

		// Subscribe for PREPREPARE messages
		sub = i.subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_PREPREPARE,
				View:        view,
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.ID)

	for {
		select {
		case <-ctx.Done():
			// Stop signal received, exit
			return errTimeoutExpired
		case <-sub.SubCh:
			/* START ALPHA CHANGE 2584 -- discard execution completed by a cancelled round worker -- */
			candidate := i.handlePrePrepareCandidate(view)

			if ctx.Err() != nil {
				return errTimeoutExpired
			}

			if candidate == nil || !i.acceptProposal(candidate) {
				continue
			}

			i.sendPrepareMessage(view)
			i.log.Debug("prepare message multicasted")

			return nil
			/* FINISH ALPHA CHANGE 2584 -- discard execution completed by a cancelled round worker -- */
		}
	}
}

/*
	START ALPHA CHANGE 2566 -- validate proposal context without executing the candidate --

Execution belongs after the round-specific certificate checks.
Payload guards precede use of message extraction helpers.
*/
func (i *IBFT) validateProposalCommon(msg *proto.Message, view *proto.View) bool {
	if msg == nil || view == nil || msg.View == nil ||
		msg.Type != proto.MessageType_PREPREPARE {
		return false
	}

	data := msg.GetPreprepareData()
	if data == nil || data.Proposal == nil {
		return false
	}

	if msg.View.Height != view.Height ||
		msg.View.Round != view.Round ||
		data.Proposal.Round != view.Round {
		return false
	}

	if !i.backend.IsProposer(msg.From, view.Height, view.Round) {
		return false
	}

	return i.backend.IsValidProposalHash(data.Proposal, data.ProposalHash)
}

/* FINISH ALPHA CHANGE 2566 -- validate proposal context without executing the candidate -- */

/* START ALPHA CHANGE 2566 -- execute round-zero proposals only after contextual validation -- */
func (i *IBFT) validateProposal0(msg *proto.Message, view *proto.View) bool {
	if view == nil || view.Round != 0 ||
		!i.validateProposalCommon(msg, view) {
		return false
	}

	if i.backend.IsProposer(i.backend.ID(), view.Height, view.Round) {
		return false
	}

	/* START ALPHA CHANGE 2584 -- execute only after all protocol checks pass -- */
	return true
	/* FINISH ALPHA CHANGE 2584 -- execute only after all protocol checks pass -- */
}

/* FINISH ALPHA CHANGE 2566 -- execute round-zero proposals only after contextual validation -- */

// validateProposal validates a proposal for round > 0
func (i *IBFT) validateProposal(msg *proto.Message, view *proto.View) bool {
	/* START ALPHA CHANGE 2566 -- check later-round proposal shape before extraction -- */
	if view == nil || view.Round == 0 ||
		!i.validateProposalCommon(msg, view) {
		return false
	}

	var (
		height = view.Height
		round  = view.Round

		proposal = messages.ExtractProposal(msg)
		rcc      = messages.ExtractRoundChangeCertificate(msg)
	)
	/* FINISH ALPHA CHANGE 2566 -- check later-round proposal shape before extraction -- */

	/* START ALPHA CHANGE 2566 -- reject malformed round-change evidence before inspection -- */
	if rcc == nil {
		return false
	}

	for _, rc := range rcc.RoundChangeMessages {
		if rc == nil || rc.View == nil ||
			rc.Type != proto.MessageType_ROUND_CHANGE ||
			rc.GetRoundChangeData() == nil {
			return false
		}
	}
	/* FINISH ALPHA CHANGE 2566 -- reject malformed round-change evidence before inspection -- */

	// Make sure all the messages have the unique sender
	if !messages.HasUniqueSenders(rcc.RoundChangeMessages) {
		return false
	}

	// Make sure there are Quorum RCC
	if !i.hasQuorumByMsgType(rcc.RoundChangeMessages, proto.MessageType_ROUND_CHANGE) {
		return false
	}

	// Make sure the current node is not the proposer for this round
	if i.backend.IsProposer(i.backend.ID(), height, round) {
		return false
	}

	// Make sure all messages in the RCC are valid Round Change messages
	for _, rc := range rcc.RoundChangeMessages {
		// Make sure the message is a Round Change message
		if rc.Type != proto.MessageType_ROUND_CHANGE {
			return false
		}

		// Height of the message matches height of the proposal
		if rc.View.Height != height {
			return false
		}

		// Round of the message matches round of the proposal
		if rc.View.Round != round {
			return false
		}

		// Sender of RCC is valid
		if !i.backend.IsValidValidator(rc) {
			return false
		}
	}

	// Extract possible rounds and their corresponding
	// block hashes
	type roundHashTuple struct {
		round uint64
		hash  []byte
	}

	roundsAndPreparedBlockHashes := make([]roundHashTuple, 0)

	/* START ALPHA CHANGE 2566 -- reject invalid supplied prepared evidence -- */
	for _, rcMessage := range rcc.RoundChangeMessages {
		cert := messages.ExtractLatestPC(rcMessage)
		lastPrepared := messages.ExtractLastPreparedProposal(rcMessage)

		if !i.validPC(cert, round, height) ||
			!i.proposalMatchesCertificate(lastPrepared, cert) {
			return false
		}

		if cert != nil {
			hash := messages.ExtractProposalHash(cert.ProposalMessage)

			roundsAndPreparedBlockHashes = append(roundsAndPreparedBlockHashes, roundHashTuple{
				round: cert.ProposalMessage.View.Round,
				hash:  hash,
			})
		}
	}

	/* START ALPHA CHANGE 2584 -- defer execution until candidate result construction -- */
	if len(roundsAndPreparedBlockHashes) == 0 {
		return true
	}
	/* FINISH ALPHA CHANGE 2584 -- defer execution until candidate result construction -- */
	/* FINISH ALPHA CHANGE 2566 -- reject invalid supplied prepared evidence -- */

	// Find the max round
	var (
		maxRound     uint64
		expectedHash []byte
	)

	for _, tuple := range roundsAndPreparedBlockHashes {
		if tuple.round >= maxRound {
			maxRound = tuple.round
			expectedHash = tuple.hash
		}
	}

	/* START ALPHA CHANGE 2566 -- complete prepared-block checks before execution -- */
	if !i.backend.IsValidProposalHash(
		&proto.Proposal{
			RawProposal: proposal.RawProposal,
			Round:       maxRound,
		},
		expectedHash,
	) {
		return false
	}

	/* START ALPHA CHANGE 2584 -- defer execution after prepared evidence validation -- */
	return true
	/* FINISH ALPHA CHANGE 2584 -- defer execution after prepared evidence validation -- */
	/* FINISH ALPHA CHANGE 2566 -- complete prepared-block checks before execution -- */
}

/* START ALPHA CHANGE 2590 -- remove the unused message-only PREPREPARE wrapper -- */
/* FINISH ALPHA CHANGE 2590 -- remove the unused message-only PREPREPARE wrapper -- */

// runPrepare runs the Prepare IBFT state
func (i *IBFT) runPrepare(ctx context.Context) error {
	i.log.Debug("enter: prepare state")
	defer i.log.Debug("exit: prepare state")

	var (
		// Grab the current view
		view = i.state.getView()

		// Subscribe to PREPARE messages
		sub = i.subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_PREPARE,
				View:        view,
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.ID)

	for {
		select {
		case <-ctx.Done():
			// Stop signal received, exit
			return errTimeoutExpired
		case <-sub.SubCh:
			if !i.handlePrepare(view) {
				//	quorum of valid prepare messages not received, retry
				continue
			}

			return nil
		}
	}
}

// handlePrepare parses available prepare messages and performs
// a transition to COMMIT state, if quorum was reached
func (i *IBFT) handlePrepare(view *proto.View) bool {
	isValidPrepare := func(message *proto.Message) bool {
		// Verify that the proposal hash is valid
		return i.backend.IsValidProposalHash(
			i.state.getProposal(),
			messages.ExtractPrepareHash(message),
		)
	}

	prepareMessages := i.messages.GetValidMessages(
		view,
		proto.MessageType_PREPARE,
		isValidPrepare,
	)

	if !i.hasQuorumByMsgType(prepareMessages, proto.MessageType_PREPARE) {
		//	quorum not reached, keep polling
		return false
	}

	// Multicast the COMMIT message
	i.sendCommitMessage(view)

	i.log.Debug("commit message multicasted")

	i.state.finalizePrepare(
		&proto.PreparedCertificate{
			ProposalMessage: i.state.getProposalMessage(),
			PrepareMessages: prepareMessages,
		},
		i.state.getProposal(),
	)

	return true
}

// runCommit runs the Commit IBFT state
func (i *IBFT) runCommit(ctx context.Context) error {
	i.log.Debug("enter: commit state")
	defer i.log.Debug("exit: commit state")

	var (
		// Grab the current view
		view = i.state.getView()

		// Subscribe to COMMIT messages
		sub = i.subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_COMMIT,
				View:        view,
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.ID)

	for {
		select {
		case <-ctx.Done():
			// Stop signal received, exit
			return errTimeoutExpired
		case <-sub.SubCh:
			if !i.handleCommit(view) {
				//	quorum not reached, retry
				continue
			}

			return nil
		}
	}
}

// handleCommit parses available commit messages and performs
// a transition to FIN state, if quorum was reached
func (i *IBFT) handleCommit(view *proto.View) bool {
	isValidCommit := func(message *proto.Message) bool {
		var (
			proposalHash  = messages.ExtractCommitHash(message)
			committedSeal = messages.ExtractCommittedSeal(message)
		)
		//	Verify that the proposal hash is valid
		if !i.backend.IsValidProposalHash(i.state.getProposal(), proposalHash) {
			return false
		}

		//	Verify that the committed seal is valid
		return i.backend.IsValidCommittedSeal(proposalHash, committedSeal)
	}

	commitMessages := i.messages.GetValidMessages(view, proto.MessageType_COMMIT, isValidCommit)
	if !i.hasQuorumByMsgType(commitMessages, proto.MessageType_COMMIT) {
		//	quorum not reached, keep polling
		return false
	}

	commitSeals, err := messages.ExtractCommittedSeals(commitMessages)
	if err != nil {
		// safe check
		i.log.Error("failed to extract committed seals from commit messages: %+v", err)

		return false
	}

	// Set the committed seals
	i.state.setCommittedSeals(commitSeals)

	//	Move to the fin state
	i.state.changeState(fin)

	return true
}

// runFin runs the fin state (block insertion)
/* START ALPHA CHANGE 2584 -- complete the sequence only after selected-result insertion succeeds -- */
func (i *IBFT) runFin() error {
	i.log.Debug("enter: fin state")
	defer i.log.Debug("exit: fin state")

	if err := i.backend.InsertProposal(
		&proto.Proposal{
			RawProposal: i.state.getRawDataFromProposal(),
			Round:       i.state.getRound(),
		},
		i.state.getCommittedSeals(),
		i.state.getExecution(),
	); err != nil {
		return err
	}

	i.state.markInserted()
	i.messages.PruneByHeight(i.state.getHeight())

	return nil
}

/* FINISH ALPHA CHANGE 2584 -- complete the sequence only after selected-result insertion succeeds -- */

// moveToNewRound moves the state to the new round
func (i *IBFT) moveToNewRound(round uint64) {
	view := &proto.View{
		Height: i.state.getHeight(),
		Round:  round,
	}

	i.state.setView(view)
	i.state.setRoundStarted(false)
	i.state.setProposalMessage(nil)
	i.state.changeState(newRound)

	/* START ALPHA CHANGE 1892 -- release message buckets below the adopted round -- */
	i.messages.PruneByView(view)
	/* FINISH ALPHA CHANGE 1892 -- release message buckets below the adopted round -- */
}

/* START ALPHA CHANGE 2584 -- preserve the message-only construction helper -- */
func (i *IBFT) buildProposal(ctx context.Context, view *proto.View) *proto.Message {
	candidate := i.buildCandidate(ctx, view)
	if candidate == nil {
		return nil
	}

	return candidate.message
}

/* FINISH ALPHA CHANGE 2584 -- preserve the message-only construction helper -- */

// acceptProposal accepts the proposal and moves the state
/* START ALPHA CHANGE 2584 -- atomically adopt a proposal and its matching execution result -- */
func (i *IBFT) acceptProposal(candidate *proposalCandidate) bool {
	proposal := candidateProposal(candidate)
	if proposal == nil ||
		!i.backend.IsCurrentExecution(proposal.RawProposal, candidate.execution) {
		return false
	}

	return i.state.adoptExecutedProposal(candidate)
}

/* FINISH ALPHA CHANGE 2584 -- atomically adopt a proposal and its matching execution result -- */

/* START ALPHA CHANGE 1893 -- bound consensus messages before signature verification and storage -- */
// AddMessage adds a new message to the IBFT message system.
func (i *IBFT) AddMessage(message *proto.Message) {
	if !i.isAcceptableMessage(message) {
		return
	}

	i.messages.AddMessage(message)

	if message.View.Height != i.state.getHeight() {
		return
	}

	// Future-round catch-up uses a weaker threshold than finality. Signal the
	// watcher for every admitted ROUND_CHANGE and let it validate/count them.
	if message.Type == proto.MessageType_ROUND_CHANGE {
		i.messages.SignalEvent(message.Type, message.View)

		return
	}

	msgs := i.messages.GetValidMessages(
		message.View,
		message.Type,
		func(_ *proto.Message) bool { return true },
	)
	if i.hasQuorumByMsgType(msgs, message.Type) {
		i.messages.SignalEvent(message.Type, message.View)
	}
}

// isAcceptableMessage performs cheap view checks before the comparatively
// expensive validator-signature verification.
func (i *IBFT) isAcceptableMessage(message *proto.Message) bool {
	if message == nil || message.View == nil {
		return false
	}

	switch message.Type {
	case proto.MessageType_PREPREPARE,
		proto.MessageType_PREPARE,
		proto.MessageType_COMMIT,
		proto.MessageType_ROUND_CHANGE:
	default:
		return false
	}

	if !isViewWithinBounds(i.state.getView(), message.View) {
		return false
	}

	return i.backend.IsValidValidator(message)
}

func isViewWithinBounds(current, candidate *proto.View) bool {
	if current == nil || candidate == nil || candidate.Height < current.Height {
		return false
	}

	heightDistance := candidate.Height - current.Height
	if heightDistance > maxFutureHeightDistance {
		return false
	}

	// Only round zero is meaningful before the next height becomes active.
	if heightDistance == maxFutureHeightDistance {
		return candidate.Round == 0
	}

	if candidate.Round < current.Round {
		return false
	}

	return candidate.Round-current.Round <= maxFutureRoundDistance
}

/* FINISH ALPHA CHANGE 1893 -- bound consensus messages before signature verification and storage -- */

// ExtendRoundTimeout extends each round's timer by the specified amount.
func (i *IBFT) ExtendRoundTimeout(amount time.Duration) {
	i.additionalTimeout = amount
}

// validPC verifies that the prepared certificate is valid
func (i *IBFT) validPC(
	certificate *proto.PreparedCertificate,
	rLimit,
	height uint64,
) bool {
	if certificate == nil {
		// PCs that are not set are valid by default
		return true
	}

	/* START ALPHA CHANGE 2566 -- bind certified proposal metadata before certificate processing -- */
	/* START ALPHA CHANGE 3798 -- validate empty prepared certificates using the existing quorum rule -- */
	// An empty repeated prepare list may be nil after protobuf decoding.
	// The proposal and any prepare messages must satisfy the quorum below.
	if certificate.ProposalMessage == nil {
		return false
	}
	/* FINISH ALPHA CHANGE 3798 -- validate empty prepared certificates using the existing quorum rule -- */

	proposalMessage := certificate.ProposalMessage
	proposalData := proposalMessage.GetPreprepareData()
	if proposalMessage.Type != proto.MessageType_PREPREPARE ||
		proposalMessage.View == nil ||
		proposalData == nil || proposalData.Proposal == nil {
		return false
	}

	if proposalData.Proposal.Round != proposalMessage.View.Round {
		return false
	}

	for _, message := range certificate.PrepareMessages {
		if message == nil || message.View == nil ||
			message.Type != proto.MessageType_PREPARE ||
			message.GetPrepareData() == nil {
			return false
		}
	}
	/* FINISH ALPHA CHANGE 2566 -- bind certified proposal metadata before certificate processing -- */

	allMessages := append(
		[]*proto.Message{certificate.ProposalMessage},
		certificate.PrepareMessages...,
	)

	// Make sure the senders are unique
	if !messages.HasUniqueSenders(allMessages) {
		return false
	}

	// Make sure there are at least Quorum (PP + P) messages
	// hasQuorum directly since the messages are of different types
	if !i.validatorManager.HasQuorum(convertMessageToAddressSet(allMessages)) {
		return false
	}

	// Make sure the proposal message is a Preprepare message
	if certificate.ProposalMessage.Type != proto.MessageType_PREPREPARE {
		return false
	}

	// Make sure all messages in the PC are Prepare messages
	for _, message := range certificate.PrepareMessages {
		if message.Type != proto.MessageType_PREPARE {
			return false
		}
	}

	// Make sure the proposal hashes match
	if !messages.HaveSameProposalHash(allMessages) {
		return false
	}

	// Make sure all the messages have a round number lower than rLimit
	if !messages.AllHaveLowerRound(allMessages, rLimit) {
		return false
	}

	// Make sure all the messages have the same height
	if !messages.AllHaveSameHeight(allMessages, height) {
		return false
	}

	// Make sure all have the same round
	if !messages.AllHaveSameRound(allMessages) {
		return false
	}

	// Make sure the proposal message is sent by the proposer
	// for the round
	proposal := certificate.ProposalMessage
	if !i.backend.IsProposer(proposal.From, proposal.View.Height, proposal.View.Round) {
		return false
	}

	// Make sure that the proposal sender is valid
	if !i.backend.IsValidValidator(proposal) {
		return false
	}

	// Make sure the Prepare messages are validators, apart from the proposer
	for _, message := range certificate.PrepareMessages {
		// Make sure the sender is part of the validator set
		if !i.backend.IsValidValidator(message) {
			return false
		}

		// Make sure the current node is not the proposer
		if i.backend.IsProposer(message.From, message.View.Height, message.View.Round) {
			return false
		}
	}

	/* START ALPHA CHANGE 2566 -- validate checkpoint context against the certified round -- */
	return i.backend.IsValidProposalHash(
		proposalData.Proposal,
		proposalData.ProposalHash,
	)
	/* FINISH ALPHA CHANGE 2566 -- validate checkpoint context against the certified round -- */
}

// sendPreprepareMessage sends out the preprepare message
func (i *IBFT) sendPreprepareMessage(message *proto.Message) {
	i.transport.Multicast(message)
}

// sendRoundChangeMessage sends out the round change message
func (i *IBFT) sendRoundChangeMessage(height, newRound uint64) {
	i.transport.Multicast(
		i.backend.BuildRoundChangeMessage(
			i.state.getLatestPreparedProposal(),
			i.state.getLatestPC(),
			&proto.View{
				Height: height,
				Round:  newRound,
			},
		),
	)
}

// sendPrepareMessage sends out the prepare message
func (i *IBFT) sendPrepareMessage(view *proto.View) {
	i.transport.Multicast(
		i.backend.BuildPrepareMessage(
			i.state.getProposalHash(),
			view,
		),
	)
}

// sendCommitMessage sends out the commit message
func (i *IBFT) sendCommitMessage(view *proto.View) {
	i.transport.Multicast(
		i.backend.BuildCommitMessage(
			i.state.getProposalHash(),
			view,
		),
	)
}

// hasQuorumByMsgType provides information on whether messages of specific types have reached the quorum
func (i *IBFT) hasQuorumByMsgType(msgs []*proto.Message, msgType proto.MessageType) bool {
	switch msgType {
	case proto.MessageType_PREPREPARE:
		return len(msgs) >= 1
	case proto.MessageType_PREPARE:
		return i.validatorManager.HasPrepareQuorum(i.state.getStateName(), i.state.getProposalMessage(), msgs)
	case proto.MessageType_ROUND_CHANGE, proto.MessageType_COMMIT:
		return i.validatorManager.HasQuorum(convertMessageToAddressSet(msgs))
	default:
		return false
	}
}

func (i *IBFT) subscribe(details messages.SubscriptionDetails) *messages.Subscription {
	subscription := i.messages.Subscribe(details)
	msgs := i.messages.GetValidMessages(
		details.View,
		details.MessageType,
		func(_ *proto.Message) bool { return true })
	// Check if any condition is already met
	if i.hasQuorumByMsgType(msgs, details.MessageType) {
		i.messages.SignalEvent(details.MessageType, details.View)
	}

	return subscription
}

/* START ALPHA CHANGE 1894 -- preserve exponential timeouts without duration overflow -- */
// getRoundTimeout exponentially increases the timeout and saturates at the
// maximum representable duration instead of wrapping negative.
func getRoundTimeout(
	baseRoundTimeout,
	additionalTimeout time.Duration,
	round uint64,
) time.Duration {
	roundTimeout := baseRoundTimeout

	for remaining := round; remaining > 0; remaining-- {
		if roundTimeout > maxRoundTimeout/2 {
			return maxRoundTimeout
		}

		roundTimeout *= 2
	}

	if additionalTimeout > 0 &&
		roundTimeout > maxRoundTimeout-additionalTimeout {
		return maxRoundTimeout
	}

	return roundTimeout + additionalTimeout
}

/* FINISH ALPHA CHANGE 1894 -- preserve exponential timeouts without duration overflow -- */
