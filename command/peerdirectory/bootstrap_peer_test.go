/* START ALPHA CHANGE 3851 -- verify identity-only genesis metadata in directory bootstrap -- */

package peerdirectory

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestPeerDirectoryGenesisBootstrap_IdentityOnlyMetadata(t *testing.T) {
	first, firstID := peerDirectoryGenesisValidatorFixture(t)
	_, secondID := peerDirectoryGenesisValidatorFixture(t)

	tests := []struct {
		name      string
		addresses []string
		wantIDs   []peer.ID
		wantError bool
	}{
		{
			name:      "identity_only",
			addresses: []string{"/p2p/" + firstID.String()},
			wantIDs:   []peer.ID{firstID},
		},
		{
			name: "mixed_complete_and_identity_only",
			addresses: []string{
				first.MultiAddr,
				"/p2p/" + secondID.String(),
			},
			wantIDs: []peer.ID{firstID, secondID},
		},
		{
			name: "same_identity_in_both_forms",
			addresses: []string{
				first.MultiAddr,
				"/p2p/" + firstID.String(),
			},
			wantIDs: []peer.ID{firstID},
		},
		{
			name:      "empty_address",
			addresses: []string{""},
			wantError: true,
		},
		{
			name: "empty_after_valid_entry",
			addresses: []string{
				first.MultiAddr,
				"",
			},
			wantError: true,
		},
		{
			name:      "transport_without_identity",
			addresses: []string{"/ip4/127.0.0.1/tcp/30301"},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validators := make([]*validator.GenesisValidator, len(test.addresses))
			for i, address := range test.addresses {
				validators[i] = &validator.GenesisValidator{
					MultiAddr: address,
				}
			}

			actual, err := buildBootstrapPeerIDs(innbcbft.INNBCBFTConfig{
				InitialValidatorSet: validators,
			})

			if test.wantError {
				require.ErrorContains(t, err, "failed to parse genesis validator multiaddr")
				require.Nil(t, actual)

				return
			}

			require.NoError(t, err)
			require.Len(t, actual, len(test.wantIDs))
			for _, id := range test.wantIDs {
				require.Contains(t, actual, id)
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3851 -- verify identity-only genesis metadata in directory bootstrap -- */
