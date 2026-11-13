/* START ALPHA CHANGE 3878 -- regress live subscription collision and cancellation ownership -- */

package ibftmessages

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"
	"github.com/stretchr/testify/require"
)

func requireSubscriptionIDRound(
	t *testing.T,
	subscription *Subscription,
	expected uint64,
) {
	t.Helper()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case round, open := <-subscription.SubCh:
		require.True(t, open, "subscription closed unexpectedly")
		require.Equal(t, expected, round)
	case <-timer.C:
		t.Fatal("subscription did not receive its notification")
	}
}

func waitSubscriptionIDClosed(t *testing.T, output <-chan uint64) {
	t.Helper()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case _, open := <-output:
			if !open {
				return
			}
		case <-timer.C:
			t.Fatal("subscription worker did not close its output")
		}
	}
}

func TestSubscriptionID_LiveCollisionOwnership(t *testing.T) {
	// Do not parallelize: these cases temporarily replace UUID randomness.
	cases := []struct {
		name      string
		candidate uint32
		expected  []SubscriptionID
	}{
		{
			name:      "repeated_candidate",
			candidate: 42,
			expected:  []SubscriptionID{42, 43, 44},
		},
		{
			name:      "cross_signed_boundary",
			candidate: 0x7fffffff,
			expected: []SubscriptionID{
				2147483647,
				-2147483648,
				-2147483647,
			},
		},
		{
			name:      "wrap_to_zero",
			candidate: 0xffffffff,
			expected:  []SubscriptionID{-1, 0, 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Every UUID supplies the same initial 32-bit candidate.
			// Exactly four allocations are made in each case.
			raw := make([]byte, 16)
			binary.BigEndian.PutUint32(raw[:4], tc.candidate)
			uuid.SetRand(bytes.NewReader(bytes.Repeat(raw, 4)))

			// Registered first so randomness is restored after worker cleanup.
			t.Cleanup(func() {
				uuid.SetRand(nil)
			})

			manager := newEventManager()
			owned := make([]*eventSubscription, 0, 4)

			t.Cleanup(func() {
				// Capture and stop displaced workers too, allowing this test
				// to fail against the old implementation without leaking them.
				manager.subscriptionsLock.Lock()
				for _, worker := range owned {
					select {
					case <-worker.doneCh:
					default:
						worker.close()
					}
				}
				manager.subscriptionsLock.Unlock()

				// Signal every worker before waiting for any worker.
				for _, worker := range owned {
					waitSubscriptionIDClosed(t, worker.outputCh)
				}
			})

			details := SubscriptionDetails{
				MessageType: proto.MessageType_PREPARE,
				View: &proto.View{
					Height: 10,
					Round:  3,
				},
			}

			subscribe := func() *Subscription {
				t.Helper()

				subscription := manager.subscribe(details)

				// Record the actual worker immediately, before another
				// allocation could overwrite it in the old implementation.
				manager.subscriptionsLock.RLock()
				worker := manager.subscriptions[subscription.ID]
				manager.subscriptionsLock.RUnlock()

				require.NotNil(t, worker)
				owned = append(owned, worker)

				return subscription
			}

			assertRegistered := func(expected int64) {
				t.Helper()

				manager.subscriptionsLock.RLock()
				defer manager.subscriptionsLock.RUnlock()

				require.Len(t, manager.subscriptions, int(expected))
				require.Equal(
					t,
					expected,
					atomic.LoadInt64(&manager.numSubscriptions),
				)
			}

			notify := func(subscriptions ...*Subscription) {
				t.Helper()

				manager.signalEvent(details.MessageType, details.View)
				for _, subscription := range subscriptions {
					requireSubscriptionIDRound(
						t,
						subscription,
						details.View.Round,
					)
				}
			}

			subscriptions := make([]*Subscription, 0, 3)
			for _, expectedID := range tc.expected {
				subscription := subscribe()
				subscriptions = append(subscriptions, subscription)
				require.Equal(t, expectedID, subscription.ID)
			}

			assertRegistered(3)
			notify(subscriptions...)

			// Cancelling the middle owner must leave both other
			// subscriptions registered and receiving notifications.
			manager.cancelSubscription(subscriptions[1].ID)
			waitSubscriptionIDClosed(t, subscriptions[1].SubCh)
			assertRegistered(2)
			notify(subscriptions[0], subscriptions[2])

			// The freed ID can be reused while occupied IDs remain intact.
			replacement := subscribe()
			require.Equal(t, subscriptions[1].ID, replacement.ID)
			assertRegistered(3)
			notify(subscriptions[0], subscriptions[2], replacement)

			// Each remaining owner must independently stop its worker.
			for _, subscription := range []*Subscription{
				subscriptions[0],
				subscriptions[2],
				replacement,
			} {
				manager.cancelSubscription(subscription.ID)
				waitSubscriptionIDClosed(t, subscription.SubCh)
			}

			assertRegistered(0)
		})
	}
}

/* FINISH ALPHA CHANGE 3878 -- regress live subscription collision and cancellation ownership -- */
