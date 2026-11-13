package peerdirectory

/* START ALPHA CHANGE 3517 -- support bootstrap validator structure regressions -- */
import (
	"crypto/rand"
	"net/http"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

/* FINISH ALPHA CHANGE 3517 -- support bootstrap validator structure regressions -- */

/* START ALPHA CHANGE 1922 -- test peer directory HTTP resource boundaries -- */

func TestNewPeerDirectoryHTTPServerUsesBoundedTimeouts(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newPeerDirectoryHTTPServer(handler)

	require.NotNil(t, server)
	require.NotNil(t, server.Handler)
	require.Equal(t, peerDirectoryReadHeaderTimeout, server.ReadHeaderTimeout)
	require.Equal(t, peerDirectoryReadTimeout, server.ReadTimeout)
	require.Equal(t, peerDirectoryWriteTimeout, server.WriteTimeout)
	require.Equal(t, peerDirectoryIdleTimeout, server.IdleTimeout)
	require.Equal(t, peerDirectoryMaxHeaderBytes, server.MaxHeaderBytes)

	require.Positive(t, server.ReadHeaderTimeout)
	require.Positive(t, server.ReadTimeout)
	require.Positive(t, server.WriteTimeout)
	require.Positive(t, server.IdleTimeout)
	require.Positive(t, server.MaxHeaderBytes)
	require.LessOrEqual(t, server.ReadHeaderTimeout, server.ReadTimeout)
}

/* FINISH ALPHA CHANGE 1922 -- test peer directory HTTP resource boundaries -- */

/* START ALPHA CHANGE 3517 -- reject nil bootstrap entries without returning partial allowlists -- */
func TestPeerDirectoryGenesisValidators_BootstrapPeerIDs(t *testing.T) {
	first, firstID := peerDirectoryGenesisValidatorFixture(t)
	second, secondID := peerDirectoryGenesisValidatorFixture(t)
	malformed := &validator.GenesisValidator{
		MultiAddr: "not-a-multiaddr",
	}

	for _, tc := range []struct {
		name       string
		validators []*validator.GenesisValidator
		wantIDs    []peer.ID
		wantError  string
	}{
		{
			name: "empty_set_preserves_existing_behavior",
		},
		{
			name:       "null_first_entry",
			validators: []*validator.GenesisValidator{nil},
			wantError:  "nil genesis validator at index 0",
		},
		{
			name:       "null_after_valid_entry",
			validators: []*validator.GenesisValidator{first, nil},
			wantError:  "nil genesis validator at index 1",
		},
		{
			name:       "valid_entries",
			validators: []*validator.GenesisValidator{first, second},
			wantIDs:    []peer.ID{firstID, secondID},
		},
		{
			name:       "repeated_peer_preserves_existing_deduplication",
			validators: []*validator.GenesisValidator{first, first},
			wantIDs:    []peer.ID{firstID},
		},
		{
			name:       "malformed_first_multiaddr",
			validators: []*validator.GenesisValidator{malformed},
			wantError:  "failed to parse genesis validator multiaddr",
		},
		{
			name:       "malformed_multiaddr_after_valid_entry",
			validators: []*validator.GenesisValidator{first, malformed},
			wantError:  "failed to parse genesis validator multiaddr",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				actual map[peer.ID]struct{}
				err    error
			)

			require.NotPanics(t, func() {
				actual, err = buildBootstrapPeerIDs(innbcbft.INNBCBFTConfig{
					InitialValidatorSet: tc.validators,
				})
			})

			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, actual, "failure must not return a partial allowlist")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, actual)
			require.Len(t, actual, len(tc.wantIDs))
			for _, id := range tc.wantIDs {
				require.Contains(t, actual, id)
			}
		})
	}
}

func peerDirectoryGenesisValidatorFixture(t *testing.T) (*validator.GenesisValidator, peer.ID) {
	t.Helper()

	privateKey, _, err := libp2pCrypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)

	id, err := peer.IDFromPrivateKey(privateKey)
	require.NoError(t, err)

	return &validator.GenesisValidator{
		MultiAddr: "/ip4/127.0.0.1/tcp/30301/p2p/" + id.String(),
	}, id
}

/* FINISH ALPHA CHANGE 3517 -- reject nil bootstrap entries without returning partial allowlists -- */
