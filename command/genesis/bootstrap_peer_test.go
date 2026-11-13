/* START ALPHA CHANGE 3850 -- regress genesis bootstrap identity preservation and input validation -- */

package genesis

import (
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	networkcommon "github.com/innovativebioresearch/innbc-chain/network/common"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func genesisBootstrapSecrets(
	t *testing.T,
	directory string,
	withNetwork bool,
) (secrets.SecretsManager, *wallet.Account, peer.ID) {
	t.Helper()

	manager, err := secretshelper.SetupLocalSecretsManager(directory)
	require.NoError(t, err)

	// Match the current node command's BN256 account initialization.
	account, err := wallet.GenerateAccount()
	require.NoError(t, err)
	require.NoError(t, account.Save(manager))

	if !withNetwork {
		return manager, account, ""
	}

	networkKey, err := secretshelper.InitNetworkingPrivateKey(manager)
	require.NoError(t, err)

	id, err := peer.IDFromPrivateKey(networkKey)
	require.NoError(t, err)

	return manager, account, id
}

func TestGenesisBootstrap_PrefixPreservesNetworkIdentity(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "validator-1")
	manager, account, id := genesisBootstrapSecrets(t, directory, true)

	keyNames := []string{
		secrets.ValidatorKey,
		secrets.ValidatorBLSKey,
		secrets.NetworkKey,
	}
	before := make(map[string][]byte, len(keyNames))
	for _, name := range keyNames {
		value, err := manager.GetSecret(name)
		require.NoError(t, err)
		before[name] = value
	}

	loaders := []struct {
		name string
		load func() ([]*validator.GenesisValidator, error)
	}{
		{
			name: "direct_prefix_reader",
			load: func() ([]*validator.GenesisValidator, error) {
				return ReadValidatorsByPrefix(root, "validator-")
			},
		},
		{
			name: "selected_directory_workflow",
			load: func() ([]*validator.GenesisValidator, error) {
				p := &genesisParams{
					validatorsPath:       root,
					validatorsPrefixPath: "validator-",
				}

				return p.getValidatorAccounts()
			},
		},
	}

	for _, loader := range loaders {
		t.Run(loader.name, func(t *testing.T) {
			loaded, err := loader.load()
			require.NoError(t, err)
			require.Len(t, loaded, 1)

			encoded, err := json.Marshal(loaded)
			require.NoError(t, err)

			var decoded []*validator.GenesisValidator
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Len(t, decoded, 1)

			entry := decoded[0]
			require.Equal(t, "/p2p/"+id.String(), entry.MultiAddr)
			require.Equal(t, account.Address(), entry.Address)
			require.Equal(
				t,
				hex.EncodeToString(account.Bls.PublicKey().Marshal()),
				entry.BlsKey,
			)

			info, err := networkcommon.StringToAddrInfo(entry.MultiAddr)
			require.NoError(t, err)
			require.Equal(t, id, info.ID)
			require.Empty(t, info.Addrs)

			metadata, err := entry.ToValidatorMetadata()
			require.NoError(t, err)
			require.Equal(t, account.Address(), metadata.Address)

			for _, name := range keyNames {
				after, err := manager.GetSecret(name)
				require.NoError(t, err)
				require.Equal(t, before[name], after)
			}
		})
	}
}

func TestGenesisBootstrap_PrefixRejectsMissingNetworkKey(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "validator-1")
	manager, _, _ := genesisBootstrapSecrets(t, directory, false)

	before := make(map[string][]byte)
	for _, name := range []string{
		secrets.ValidatorKey,
		secrets.ValidatorBLSKey,
	} {
		value, err := manager.GetSecret(name)
		require.NoError(t, err)
		before[name] = value
	}

	require.False(t, manager.HasSecret(secrets.NetworkKey))

	validators, err := ReadValidatorsByPrefix(root, "validator-")
	require.ErrorContains(t, err, "missing libp2p identity")
	require.ErrorContains(t, err, directory)
	require.Nil(t, validators)

	require.False(t, manager.HasSecret(secrets.NetworkKey))
	for name, expected := range before {
		after, err := manager.GetSecret(name)
		require.NoError(t, err)
		require.Equal(t, expected, after)
	}
}

func TestGenesisBootstrap_ExplicitPeerIdentity(t *testing.T) {
	const id = "16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR"

	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)

	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	tests := []struct {
		name  string
		addr  string
		valid bool
	}{
		{
			name:  "complete_ipv4",
			addr:  "/ip4/127.0.0.1/tcp/30301/p2p/" + id,
			valid: true,
		},
		{
			name:  "complete_ipv6",
			addr:  "/ip6/::1/tcp/30301/p2p/" + id,
			valid: true,
		},
		{
			name:  "complete_dns",
			addr:  "/dns4/validator.example.com/tcp/30301/p2p/" + id,
			valid: true,
		},
		{
			name:  "identity_only",
			addr:  "/p2p/" + id,
			valid: true,
		},
		{
			name: "empty",
		},
		{
			name: "ipv4_without_peer_identity",
			addr: "/ip4/127.0.0.1/tcp/30301",
		},
		{
			name: "ipv6_without_peer_identity",
			addr: "/ip6/::1/tcp/30301",
		},
		{
			name: "malformed_peer_identity",
			addr: "/ip4/127.0.0.1/tcp/30301/p2p/not-a-peer",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := strings.Join([]string{
				test.addr,
				operator.String(),
				"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
				"0x" + hex.EncodeToString(proofBytes),
			}, ":")

			p := &genesisParams{validators: []string{entry}}
			validators, err := p.getValidatorAccounts()

			if !test.valid {
				require.ErrorContains(t, err, "invalid P2P multi address")
				require.Nil(t, validators)

				return
			}

			require.NoError(t, err)
			require.Len(t, validators, 1)
			require.Equal(t, test.addr, validators[0].MultiAddr)
			require.Equal(t, operator, validators[0].Address)

			info, err := networkcommon.StringToAddrInfo(validators[0].MultiAddr)
			require.NoError(t, err)
			require.Equal(t, id, info.ID.String())

			_, err = validators[0].ToValidatorMetadata()
			require.NoError(t, err)
		})
	}
}

/* FINISH ALPHA CHANGE 3850 -- regress genesis bootstrap identity preservation and input validation -- */
