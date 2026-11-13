package helper

import (
	/* START ALPHA CHANGE 3233 -- remove the error import used only by remote backend selection -- */
	/* FINISH ALPHA CHANGE 3233 -- remove the error import used only by remote backend selection -- */
	"fmt"

	"github.com/hashicorp/go-hclog"
	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/helper/hex"
	"github.com/innovativebioresearch/innbc-chain/network"
	"github.com/innovativebioresearch/innbc-chain/secrets"

	/* START ALPHA CHANGE 3233 -- remove remote identity backend imports from shared helpers -- */
	/* FINISH ALPHA CHANGE 3233 -- remove remote identity backend imports from shared helpers -- */
	"github.com/innovativebioresearch/innbc-chain/secrets/local"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 3233 -- initialize local identities without a backend configuration argument -- */
// SetupLocalSecretsManager initializes local identity storage.
func SetupLocalSecretsManager(dataDir string) (secrets.SecretsManager, error) {
	return local.SecretsManagerFactory(
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
			Extra: map[string]interface{}{
				secrets.Path: dataDir,
			},
		},
	)
}

/* FINISH ALPHA CHANGE 3233 -- initialize local identities without a backend configuration argument -- */

/* START ALPHA CHANGE 3233 -- remove Vault AWS and GCP identity factory helpers -- */
/* FINISH ALPHA CHANGE 3233 -- remove Vault AWS and GCP identity factory helpers -- */

// InitECDSAValidatorKey creates new ECDSA key and set as a validator key
func InitECDSAValidatorKey(secretsManager secrets.SecretsManager) (types.Address, error) {
	if secretsManager.HasSecret(secrets.ValidatorKey) {
		return types.ZeroAddress, fmt.Errorf(`secrets "%s" has been already initialized`, secrets.ValidatorKey)
	}

	validatorKey, validatorKeyEncoded, err := crypto.GenerateAndEncodeECDSAPrivateKey()
	if err != nil {
		return types.ZeroAddress, err
	}

	address := crypto.PubKeyToAddress(&validatorKey.PublicKey)

	// Write the validator private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(
		secrets.ValidatorKey,
		validatorKeyEncoded,
	); setErr != nil {
		return types.ZeroAddress, setErr
	}

	return address, nil
}

func InitBLSValidatorKey(secretsManager secrets.SecretsManager) ([]byte, error) {
	if secretsManager.HasSecret(secrets.ValidatorBLSKey) {
		return nil, fmt.Errorf(`secrets "%s" has been already initialized`, secrets.ValidatorBLSKey)
	}

	blsSecretKey, blsSecretKeyEncoded, err := crypto.GenerateAndEncodeBLSSecretKey()
	if err != nil {
		return nil, err
	}

	// Write the validator private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(
		secrets.ValidatorBLSKey,
		blsSecretKeyEncoded,
	); setErr != nil {
		return nil, setErr
	}

	pubkeyBytes, err := crypto.BLSSecretKeyToPubkeyBytes(blsSecretKey)
	if err != nil {
		return nil, err
	}

	return pubkeyBytes, nil
}

func InitNetworkingPrivateKey(secretsManager secrets.SecretsManager) (libp2pCrypto.PrivKey, error) {
	if secretsManager.HasSecret(secrets.NetworkKey) {
		return nil, fmt.Errorf(`secrets "%s" has been already initialized`, secrets.NetworkKey)
	}

	// Generate the libp2p private key
	libp2pKey, libp2pKeyEncoded, keyErr := network.GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		return nil, keyErr
	}

	// Write the networking private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(
		secrets.NetworkKey,
		libp2pKeyEncoded,
	); setErr != nil {
		return nil, setErr
	}

	return libp2pKey, keyErr
}

// LoadValidatorAddress loads ECDSA key by SecretsManager and returns validator address
func LoadValidatorAddress(secretsManager secrets.SecretsManager) (types.Address, error) {
	if !secretsManager.HasSecret(secrets.ValidatorKey) {
		return types.ZeroAddress, nil
	}

	encodedKey, err := secretsManager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return types.ZeroAddress, err
	}

	privateKey, err := crypto.BytesToECDSAPrivateKey(encodedKey)
	if err != nil {
		return types.ZeroAddress, err
	}

	return crypto.PubKeyToAddress(&privateKey.PublicKey), nil
}

// LoadBLSPublicKey loads BLS key by SecretsManager and returns BLS Public Key
func LoadBLSPublicKey(secretsManager secrets.SecretsManager) (string, error) {
	if !secretsManager.HasSecret(secrets.ValidatorBLSKey) {
		return "", nil
	}

	encodedKey, err := secretsManager.GetSecret(secrets.ValidatorBLSKey)
	if err != nil {
		return "", err
	}

	secretKey, err := crypto.BytesToBLSSecretKey(encodedKey)
	if err != nil {
		return "", err
	}

	pubkeyBytes, err := crypto.BLSSecretKeyToPubkeyBytes(secretKey)
	if err != nil {
		return "", err
	}

	return hex.EncodeToHex(pubkeyBytes), nil
}

// LoadNodeID loads Libp2p key by SecretsManager and returns Node ID
func LoadNodeID(secretsManager secrets.SecretsManager) (string, error) {
	if !secretsManager.HasSecret(secrets.NetworkKey) {
		return "", nil
	}

	encodedKey, err := secretsManager.GetSecret(secrets.NetworkKey)
	if err != nil {
		return "", err
	}

	parsedKey, err := network.ParseLibp2pKey(encodedKey)
	if err != nil {
		return "", err
	}

	nodeID, err := peer.IDFromPrivateKey(parsedKey)
	if err != nil {
		return "", err
	}

	return nodeID.String(), nil
}

/* START ALPHA CHANGE 3233 -- remove programmatic cloud identity backend selection -- */
/* FINISH ALPHA CHANGE 3233 -- remove programmatic cloud identity backend selection -- */
