/* START ALPHA CHANGE 3226 -- verify server identity initialization reuses canonical local keys -- */
package server

import (
	"os"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/innovativebioresearch/innbc-chain/secrets/local"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestLocalIdentity_ServerReusesCanonicalKeys(t *testing.T) {
	dir := t.TempDir()

	manager, err := secretshelper.SetupLocalSecretsManager(dir)
	require.NoError(t, err)

	address, err := secretshelper.InitECDSAValidatorKey(manager)
	require.NoError(t, err)
	_, err = secretshelper.InitBLSValidatorKey(manager)
	require.NoError(t, err)
	_, err = secretshelper.InitNetworkingPrivateKey(manager)
	require.NoError(t, err)

	expectedNodeID, err := secretshelper.LoadNodeID(manager)
	require.NoError(t, err)

	paths := map[string]string{
		secrets.ValidatorKey:    datadir.ValidatorKey(dir),
		secrets.ValidatorBLSKey: datadir.ValidatorBLSKey(dir),
		secrets.NetworkKey:      datadir.LibP2PKey(dir),
	}
	original := make(map[string][]byte, len(paths))
	for name, path := range paths {
		original[name], err = os.ReadFile(path)
		require.NoError(t, err)
		require.NotEmpty(t, original[name])
	}

	instance := &Server{
		config: &Config{DataDir: dir},
		logger: hclog.NewNullLogger(),
	}

	for range 2 {
		require.NoError(t, instance.setupSecretsManager())
		require.IsType(t, &local.LocalSecretsManager{}, instance.secretsManager)

		account, err := wallet.NewAccountFromSecret(instance.secretsManager)
		require.NoError(t, err)
		require.Equal(t, address, types.Address(account.Ecdsa.Address()))

		_, err = network.ReadLibp2pKey(instance.secretsManager)
		require.NoError(t, err)

		nodeID, err := secretshelper.LoadNodeID(instance.secretsManager)
		require.NoError(t, err)
		require.Equal(t, expectedNodeID, nodeID)

		for name, path := range paths {
			stored, err := instance.secretsManager.GetSecret(name)
			require.NoError(t, err)
			require.Equal(t, original[name], stored)

			onDisk, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original[name], onDisk)
		}
	}

	require.NoDirExists(t, datadir.ChainData(dir))
}

/* FINISH ALPHA CHANGE 3226 -- verify server identity initialization reuses canonical local keys -- */
