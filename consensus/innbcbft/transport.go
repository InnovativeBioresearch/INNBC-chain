package innbcbft

import (
	/* START ALPHA CHANGE 1875 -- use INNBC-owned IBFT wire types -- */
	ibftProto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	/* FINISH ALPHA CHANGE 1875 -- use INNBC-owned IBFT wire types -- */
	"github.com/libp2p/go-libp2p/core/peer"
)

/* START ALPHA CHANGE 314 -- remove bridge transport abstraction -- */
/* FINISH ALPHA CHANGE 314 -- remove bridge transport abstraction -- */

/* START ALPHA CHANGE 1840 -- reject structurally invalid consensus messages before IBFT -- */
func isStructurallyValidIBFTMessage(root *ibftProto.Message) bool {
	pending := []*ibftProto.Message{root}

	for len(pending) > 0 {
		lastIndex := len(pending) - 1
		message := pending[lastIndex]
		pending = pending[:lastIndex]

		if message == nil || message.View == nil {
			return false
		}

		switch message.Type {
		case ibftProto.MessageType_PREPREPARE:
			preprepareData := message.GetPreprepareData()
			if preprepareData == nil ||
				preprepareData.Proposal == nil ||
				len(preprepareData.Proposal.RawProposal) == 0 {
				return false
			}

			if preprepareData.Certificate != nil {
				for _, roundChangeMessage := range preprepareData.Certificate.RoundChangeMessages {
					if roundChangeMessage == nil ||
						roundChangeMessage.Type != ibftProto.MessageType_ROUND_CHANGE {
						return false
					}

					pending = append(pending, roundChangeMessage)
				}
			}

		case ibftProto.MessageType_PREPARE:
			if message.GetPrepareData() == nil {
				return false
			}

		case ibftProto.MessageType_COMMIT:
			if message.GetCommitData() == nil {
				return false
			}

		case ibftProto.MessageType_ROUND_CHANGE:
			roundChangeData := message.GetRoundChangeData()
			if roundChangeData == nil {
				return false
			}

			hasProposal := roundChangeData.LastPreparedProposal != nil
			hasCertificate := roundChangeData.LatestPreparedCertificate != nil
			if hasProposal != hasCertificate {
				return false
			}

			if !hasCertificate {
				continue
			}

			/* START ALPHA CHANGE 3797 -- accept empty prepared-certificate lists at the transport boundary -- */
			preparedCertificate := roundChangeData.LatestPreparedCertificate
			// Protobuf decodes an empty prepare list as nil.
			// The consensus core validates the certificate's voting power.
			if preparedCertificate.ProposalMessage == nil ||
				preparedCertificate.ProposalMessage.Type != ibftProto.MessageType_PREPREPARE {
				return false
			}
			/* FINISH ALPHA CHANGE 3797 -- accept empty prepared-certificate lists at the transport boundary -- */

			pending = append(pending, preparedCertificate.ProposalMessage)

			for _, prepareMessage := range preparedCertificate.PrepareMessages {
				if prepareMessage == nil ||
					prepareMessage.Type != ibftProto.MessageType_PREPARE {
					return false
				}

				pending = append(pending, prepareMessage)
			}

		default:
			return false
		}
	}

	return true
}

/* FINISH ALPHA CHANGE 1840 -- reject structurally invalid consensus messages before IBFT -- */

// subscribeToIbftTopic subscribes to ibft topic
func (p *INNBCBFT) subscribeToIbftTopic() error {
	return p.consensusTopic.Subscribe(func(obj interface{}, _ peer.ID) {
		/* START ALPHA CHANGE 1437 -- join admitted consensus topic callbacks during shutdown -- */
		if !p.enterCloseWork() {
			return
		}
		defer p.leaveCloseWork()
		/* FINISH ALPHA CHANGE 1437 -- join admitted consensus topic callbacks during shutdown -- */

		if !p.runtime.IsActiveValidator() {
			return
		}

		msg, ok := obj.(*ibftProto.Message)
		if !ok || !isStructurallyValidIBFTMessage(msg) {
			recordRejectedConsensusMessage()

			return
		}

		/* START ALPHA CHANGE 1847 -- avoid per-message consensus transport log amplification -- */
		p.ibft.AddMessage(msg)
		/* FINISH ALPHA CHANGE 1847 -- avoid per-message consensus transport log amplification -- */
	})
}

// createTopics create all topics for a PolyBft instance
func (p *INNBCBFT) createTopics() (err error) {
	/* START ALPHA CHANGE 315 -- create only INNBC consensus gossip topic -- */
	p.consensusTopic, err = p.config.Network.NewTopic(pbftProto, &ibftProto.Message{})
	if err != nil {
		return err
	}

	return nil
	/* FINISH ALPHA CHANGE 315 -- create only INNBC consensus gossip topic -- */
}

// Multicast is implementation of core.Transport interface
func (p *INNBCBFT) Multicast(msg *ibftProto.Message) {
	if err := p.consensusTopic.Publish(msg); err != nil {
		p.logger.Warn("failed to multicast consensus message", "error", err)
	}
}
