package txpool

import (
	"context"
	"fmt"

	/* START ALPHA CHANGE 2544 -- map event subscription failures to gRPC status -- */
	"errors"

	"google.golang.org/grpc/codes"
	/* START ALPHA CHANGE 2550 -- avoid the existing status type name -- */
	grpcstatus "google.golang.org/grpc/status"
	/* FINISH ALPHA CHANGE 2550 -- avoid the existing status type name -- */

	/* FINISH ALPHA CHANGE 2544 -- map event subscription failures to gRPC status -- */

	"github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// Status implements the GRPC status endpoint. Returns the number of transactions in the pool
func (p *TxPool) Status(ctx context.Context, req *empty.Empty) (*proto.TxnPoolStatusResp, error) {
	resp := &proto.TxnPoolStatusResp{
		Length: p.accounts.promoted(),
	}

	return resp, nil
}

// AddTxn adds a local transaction to the pool
func (p *TxPool) AddTxn(ctx context.Context, raw *proto.AddTxnReq) (*proto.AddTxnResp, error) {
	/* START ALPHA CHANGE 3012 -- bound operator transaction decoding and reject missing requests -- */
	if raw == nil || raw.Raw == nil {
		return nil, fmt.Errorf("transaction's field raw is empty")
	}

	txn, err := DecodeRawTransaction(raw.Raw.Value)
	if err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 3012 -- bound operator transaction decoding and reject missing requests -- */

	if raw.From != "" {
		from := types.Address{}
		if err := from.UnmarshalText([]byte(raw.From)); err != nil {
			return nil, err
		}

		txn.From = from
	}

	if err := p.AddTx(txn); err != nil {
		return nil, err
	}

	return &proto.AddTxnResp{
		TxHash: txn.Hash.String(),
	}, nil
}

/* START ALPHA CHANGE 2544 -- report queue overflow and always release stream subscriptions -- */

/* START ALPHA CHANGE 2550 -- use the aliased gRPC status package -- */
func txEventStreamError(err error) error {
	switch {
	/* START ALPHA CHANGE 2558 -- report queue and admission exhaustion consistently -- */
	case errors.Is(err, errEventQueueOverflow),
		errors.Is(err, errEventSubscriptionLimit):
		return grpcstatus.Error(codes.ResourceExhausted, err.Error())
	/* FINISH ALPHA CHANGE 2558 -- report queue and admission exhaustion consistently -- */
	case errors.Is(err, errEventManagerClosed):
		return grpcstatus.Error(codes.Unavailable, err.Error())
	default:
		return err
	}
}

/* FINISH ALPHA CHANGE 2550 -- use the aliased gRPC status package -- */

func (p *TxPool) Subscribe(
	request *proto.SubscribeRequest,
	stream proto.TxnPoolOperator_SubscribeServer,
) error {
	if err := request.ValidateAll(); err != nil {
		return err
	}

	/* START ALPHA CHANGE 2558 -- retain operator capacity until the stream handler exits -- */
	release, err := p.eventManager.reserveOperatorStream()
	if err != nil {
		return txEventStreamError(err)
	}
	defer release()

	subscription := p.eventManager.subscribe(request.Types)
	defer p.eventManager.cancelSubscription(
		subscription.subscriptionID,
		subscription.subscription,
	)
	/* FINISH ALPHA CHANGE 2558 -- retain operator capacity until the stream handler exits -- */

	for {
		select {
		case <-subscription.subscription.doneCh:
			return txEventStreamError(subscription.subscription.err())
		default:
		}

		select {
		case <-subscription.subscription.doneCh:
			return txEventStreamError(subscription.subscription.err())

		case event, more := <-subscription.subscriptionChannel:
			if !more {
				return txEventStreamError(subscription.subscription.err())
			}

			if err := stream.Send(event); err != nil {
				return err
			}

		case <-stream.Context().Done():
			/* START ALPHA CHANGE 2550 -- use the aliased context-error conversion -- */
			return grpcstatus.FromContextError(stream.Context().Err()).Err()
			/* FINISH ALPHA CHANGE 2550 -- use the aliased context-error conversion -- */
		}
	}
}

/* FINISH ALPHA CHANGE 2544 -- report queue overflow and always release stream subscriptions -- */

// TxPoolSubscribe subscribes to new events in the tx pool and returns subscription channel and unsubscribe fn
func (p *TxPool) TxPoolSubscribe(request *proto.SubscribeRequest) (<-chan *proto.TxPoolEvent, func(), error) {
	if err := request.ValidateAll(); err != nil {
		return nil, nil, err
	}

	/* START ALPHA CHANGE 2558 -- reserve independent capacity for the internal RPC feed -- */
	subscription := p.eventManager.subscribeInternal(request.Types)
	/* FINISH ALPHA CHANGE 2558 -- reserve independent capacity for the internal RPC feed -- */

	/* START ALPHA CHANGE 2551 -- clean up only the failed internal subscription -- */
	if err := subscription.subscription.err(); err != nil {
		p.eventManager.cancelSubscription(
			subscription.subscriptionID,
			subscription.subscription,
		)
		return nil, nil, err
	}
	/* FINISH ALPHA CHANGE 2551 -- clean up only the failed internal subscription -- */

	/* START ALPHA CHANGE 2551 -- preserve ownership in delayed unsubscribe callbacks -- */
	cancelSubscription := func() {
		p.eventManager.cancelSubscription(
			subscription.subscriptionID,
			subscription.subscription,
		)
	}
	/* FINISH ALPHA CHANGE 2551 -- preserve ownership in delayed unsubscribe callbacks -- */

	return subscription.subscriptionChannel, cancelSubscription, nil
}
