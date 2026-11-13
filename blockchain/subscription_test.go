package blockchain

import (
	"sync"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/assert"
)

/* START ALPHA CHANGE 1423 -- test mock subscription shutdown-aware push -- */
func TestMockSubscriptionPushReturnsAfterClose(t *testing.T) {
	t.Parallel()

	subscription := NewMockSubscription()
	subscription.Close()

	pushDone := make(chan struct{})
	go func() {
		defer close(pushDone)

		subscription.Push(&Event{})
	}()

	select {
	case <-pushDone:
	case <-time.After(time.Second):
		t.Fatal("mock subscription push blocked after close")
	}

	assert.Nil(t, subscription.GetEvent())
}

/* FINISH ALPHA CHANGE 1423 -- test mock subscription shutdown-aware push -- */

func TestSubscription(t *testing.T) {
	t.Parallel()

	var (
		e              = newEventStream()
		sub            = e.subscribe()
		caughtEventNum = uint64(0)
		event          = &Event{
			NewChain: []*types.Header{
				{
					Number: 100,
				},
			},
		}

		wg sync.WaitGroup
	)

	defer e.unsubscribe(sub)

	updateCh := sub.GetEventCh()

	wg.Add(1)

	go func() {
		defer wg.Done()

		select {
		case ev := <-updateCh:
			caughtEventNum = ev.NewChain[0].Number
		case <-time.After(5 * time.Second):
		}
	}()

	// Send the event to the channel
	e.push(event)

	// Wait for the event to be parsed
	wg.Wait()

	assert.Equal(t, event.NewChain[0].Number, caughtEventNum)
}

/* START ALPHA CHANGE 1295 -- test best-effort non-blocking event delivery -- */
func TestSubscription_BufferedChannel_MultipleSubscriptions(t *testing.T) {
	t.Parallel()

	var (
		e                  = newEventStream()
		numOfSubscriptions = 10
		event              = &Event{
			NewChain: []*types.Header{
				{
					Number: 100,
				},
			},
		}
	)

	subscriptions := make([]*subscription, numOfSubscriptions)

	for i := 0; i < numOfSubscriptions; i++ {
		subscriptions[i] = e.subscribe()
	}

	for _, sub := range subscriptions {
		defer e.unsubscribe(sub)
	}

	e.push(event)

	for i, sub := range subscriptions {
		select {
		case ev := <-sub.GetEventCh():
			assert.Equal(t, event.NewChain[0].Number, ev.NewChain[0].Number)
		case <-time.After(time.Second):
			t.Fatalf("subscription %d did not receive event", i)
		}
	}
}

func TestSubscription_PushDoesNotBlockOnFullSubscriber(t *testing.T) {
	t.Parallel()

	e := newEventStream()

	slowSub := e.subscribe()
	fastSub := e.subscribe()

	defer e.unsubscribe(slowSub)
	defer e.unsubscribe(fastSub)

	for i := 0; i < cap(slowSub.updateCh); i++ {
		slowSub.updateCh <- &Event{}
	}

	doneCh := make(chan struct{})

	go func() {
		e.push(&Event{
			NewChain: []*types.Header{
				{
					Number: 100,
				},
			},
		})
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("event push blocked on a full subscriber")
	}

	select {
	case ev := <-fastSub.updateCh:
		assert.Equal(t, uint64(100), ev.NewChain[0].Number)
	case <-time.After(time.Second):
		t.Fatal("fast subscriber did not receive event")
	}

	select {
	case <-slowSub.closeCh:
		t.Fatal("full subscriber was unexpectedly unsubscribed")
	default:
	}

	for i := 0; i < cap(slowSub.updateCh); i++ {
		select {
		case <-slowSub.updateCh:
		case <-time.After(time.Second):
			t.Fatal("failed to drain slow subscriber buffer")
		}
	}

	e.push(&Event{
		NewChain: []*types.Header{
			{
				Number: 101,
			},
		},
	})

	select {
	case ev := <-slowSub.updateCh:
		assert.Equal(t, uint64(101), ev.NewChain[0].Number)
	case <-time.After(time.Second):
		t.Fatal("formerly full subscriber did not receive later event")
	}
}

/* FINISH ALPHA CHANGE 1295 -- test best-effort non-blocking event delivery -- */

/* START ALPHA CHANGE 1296 -- make unsubscribe test deterministic after best-effort delivery -- */
func TestSubscription_AfterOneUnsubscribe(t *testing.T) {
	t.Parallel()

	e := newEventStream()

	sub1 := e.subscribe()
	sub2 := e.subscribe()

	readEvent := func(sub *subscription, expectedNumber uint64) {
		t.Helper()

		select {
		case ev := <-sub.GetEventCh():
			assert.Equal(t, expectedNumber, ev.NewChain[0].Number)
		case <-time.After(time.Second):
			t.Fatalf("subscription did not receive event %d", expectedNumber)
		}
	}

	for i := uint64(1); i <= 10; i++ {
		e.push(&Event{
			NewChain: []*types.Header{
				{
					Number: i,
				},
			},
		})

		readEvent(sub1, i)
		readEvent(sub2, i)
	}

	e.unsubscribe(sub1)

	for i := uint64(11); i <= 20; i++ {
		e.push(&Event{
			NewChain: []*types.Header{
				{
					Number: i,
				},
			},
		})

		readEvent(sub2, i)
	}

	e.unsubscribe(sub2)
}

/* FINISH ALPHA CHANGE 1296 -- make unsubscribe test deterministic after best-effort delivery -- */

func TestSubscription_NilEventAfterClosingSubscription(t *testing.T) {
	t.Parallel()

	var (
		e = newEventStream()

		wg sync.WaitGroup
	)

	sub := e.subscribe()

	wg.Add(1)

	receivedEvtCount := 0

	worker := func(sub *subscription) {
		/* START ALPHA CHANGE 168 -- remove unused expected block count from subscription worker -- */
		defer wg.Done()
		/* FINISH ALPHA CHANGE 168 -- remove unused expected block count from subscription worker -- */

		startTime := time.Now().UTC()
		timeout := 5 * time.Second

		for {
			evt := sub.GetEvent()
			if evt != nil {
				receivedEvtCount++
			} else {
				return
			}

			// Check for timeout
			if time.Since(startTime) >= timeout {
				t.Errorf("subscription did not caught all events")

				return
			}
		}
	}

	/* START ALPHA CHANGE 169 -- call subscription worker without unused expected count -- */
	go worker(sub)
	/* FINISH ALPHA CHANGE 169 -- call subscription worker without unused expected count -- */

	// Send the events to the channels
	for i := 0; i < 2; i++ {
		e.push(&Event{})
		time.Sleep(time.Millisecond)
	}

	e.unsubscribe(sub)

	// Wait for the event to be parsed
	wg.Wait()

	assert.Equal(t, 2, receivedEvtCount)
}
