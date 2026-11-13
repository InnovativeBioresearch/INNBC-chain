package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/helper/common"

	/* START ALPHA CHANGE 1613 -- use canonical node identity paths -- */
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	/* FINISH ALPHA CHANGE 1613 -- use canonical node identity paths -- */
	"github.com/innovativebioresearch/innbc-chain/secrets"
)

// LocalSecretsManager is a SecretsManager that
// stores secrets locally on disk
type LocalSecretsManager struct {
	// Logger object
	logger hclog.Logger

	// Path to the base working directory
	path string

	// Map of known secrets and their paths
	secretPathMap map[string]string

	// Mux for the secretPathMap
	secretPathMapLock sync.RWMutex
}

/* START ALPHA CHANGE 3232 -- construct local identity storage without remote configuration types -- */
// SecretsManagerFactory constructs local identity storage.
func SecretsManagerFactory(
	params *secrets.SecretsManagerParams,
) (secrets.SecretsManager, error) {
	// Set up the base object
	localManager := &LocalSecretsManager{
		logger:        params.Logger.Named("local"),
		secretPathMap: make(map[string]string),
	}
	/* FINISH ALPHA CHANGE 3232 -- construct local identity storage without remote configuration types -- */

	// Grab the path to the working directory
	path, ok := params.Extra[secrets.Path]
	if !ok {
		return nil, errors.New("no path specified for local secrets manager")
	}

	localManager.path, ok = path.(string)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	// Run the initial setup
	_ = localManager.Setup()

	return localManager, nil
}

// Setup sets up the local SecretsManager
func (l *LocalSecretsManager) Setup() error {
	// The local SecretsManager initially handles only the
	// validator and networking private keys
	l.secretPathMapLock.Lock()
	defer l.secretPathMapLock.Unlock()

	/* START ALPHA CHANGE 1614 -- isolate each local private key type -- */
	subDirectories := []string{
		filepath.Join(datadir.KeysDirName, datadir.ValidatorECDSADirName),
		filepath.Join(datadir.KeysDirName, datadir.ValidatorBLSDirName),
		filepath.Join(datadir.KeysDirName, datadir.LibP2PDirName),
	}

	// Private-key directories are accessible only to the node owner.
	if err := common.SetupDataDir(l.path, subDirectories, 0700); err != nil {
		return err
	}

	l.secretPathMap[secrets.ValidatorKey] = datadir.ValidatorKey(l.path)
	l.secretPathMap[secrets.ValidatorBLSKey] = datadir.ValidatorBLSKey(l.path)
	l.secretPathMap[secrets.NetworkKey] = datadir.LibP2PKey(l.path)
	/* FINISH ALPHA CHANGE 1614 -- isolate each local private key type -- */

	return nil
}

// GetSecret gets the local SecretsManager's secret from disk
func (l *LocalSecretsManager) GetSecret(name string) ([]byte, error) {
	l.secretPathMapLock.RLock()
	secretPath, ok := l.secretPathMap[name]
	l.secretPathMapLock.RUnlock()

	if !ok {
		return nil, secrets.ErrSecretNotFound
	}

	// Read the secret from disk
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return nil, fmt.Errorf(
			"unable to read secret from disk (%s), %w",
			secretPath,
			err,
		)
	}

	return secret, nil
}

// SetSecret saves the local SecretsManager's secret to disk
func (l *LocalSecretsManager) SetSecret(name string, value []byte) error {
	// If the data directory is not specified, skip write
	if l.path == "" {
		return nil
	}

	l.secretPathMapLock.Lock()
	secretPath, ok := l.secretPathMap[name]
	l.secretPathMapLock.Unlock()

	if !ok {
		return secrets.ErrSecretNotFound
	}

	// Checks for existing secret
	if _, err := os.Stat(secretPath); err == nil {
		return fmt.Errorf(
			"%s already initialized",
			secretPath,
		)
	}
	// Write the secret to disk
	if err := common.SaveFileSafe(secretPath, value, 0440); err != nil {
		return fmt.Errorf(
			"unable to write secret to disk (%s), %w",
			secretPath,
			err,
		)
	}

	return nil
}

// HasSecret checks if the secret is present on disk
func (l *LocalSecretsManager) HasSecret(name string) bool {
	_, err := l.GetSecret(name)

	return err == nil
}

// RemoveSecret removes the local SecretsManager's secret from disk
func (l *LocalSecretsManager) RemoveSecret(name string) error {
	l.secretPathMapLock.Lock()
	secretPath, ok := l.secretPathMap[name]
	defer l.secretPathMapLock.Unlock()

	if !ok {
		return secrets.ErrSecretNotFound
	}

	delete(l.secretPathMap, name)

	if removeErr := os.Remove(secretPath); removeErr != nil {
		return fmt.Errorf("unable to remove secret, %w", removeErr)
	}

	return nil
}
