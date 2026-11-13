package secrets

import (
	"errors"

	"github.com/hashicorp/go-hclog"
)

/* START ALPHA CHANGE 3231 -- retain the local identity path parameter and remove remote configuration keys -- */
// Path is the path to the base working directory.
const Path = "path"

/* FINISH ALPHA CHANGE 3231 -- retain the local identity path parameter and remove remote configuration keys -- */

// Define constant names for available secrets
const (
	// ValidatorKey is the private key secret of the validator node
	ValidatorKey = "validator-key"

	// ValidatorBLSKey is the bls secret key of the validator node
	ValidatorBLSKey = "validator-bls-key"

	// NetworkKey is the libp2p private key secret used for networking
	NetworkKey = "network-key"
)

/* START ALPHA CHANGE 1612 -- move local secret path ownership into helper datadir -- */
// Local secret paths are defined by helper/datadir so key generation,
// inspection, networking, and server startup share one canonical layout.
/* FINISH ALPHA CHANGE 1612 -- move local secret path ownership into helper datadir -- */

var (
	ErrSecretNotFound = errors.New("secret not found")
)

/* START ALPHA CHANGE 3231 -- remove the retired identity backend type enumeration -- */
/* FINISH ALPHA CHANGE 3231 -- remove the retired identity backend type enumeration -- */

// SecretsManager defines the base public interface that all
// secret manager implementations should have
type SecretsManager interface {
	// Setup performs secret manager-specific setup
	Setup() error

	// GetSecret gets the secret by name
	GetSecret(name string) ([]byte, error)

	// SetSecret sets the secret to a provided value
	SetSecret(name string, value []byte) error

	// HasSecret checks if the secret is present
	HasSecret(name string) bool

	// RemoveSecret removes the secret from storage
	RemoveSecret(name string) error
}

// SecretsManagerParams defines the configuration params for the
// secrets manager
type SecretsManagerParams struct {
	// Local logger object
	Logger hclog.Logger

	// Extra contains additional data needed for the SecretsManager to function
	Extra map[string]interface{}
}

/* START ALPHA CHANGE 3231 -- remove the unused backend factory type and backend support selector -- */
/* FINISH ALPHA CHANGE 3231 -- remove the unused backend factory type and backend support selector -- */
