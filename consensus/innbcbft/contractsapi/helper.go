package contractsapi

import (
	"github.com/umbracle/ethgo"
)

// StateTransactionInput is an abstraction for different state transaction inputs
type StateTransactionInput interface {
	// EncodeAbi contains logic for encoding arbitrary data into ABI format
	EncodeAbi() ([]byte, error)
	// DecodeAbi contains logic for decoding given ABI data
	DecodeAbi(b []byte) error
}

// EventAbi is an interface representing an event generated in contractsapi
type EventAbi interface {
	// Sig returns the event ABI signature or ID (which is unique for all event types)
	Sig() ethgo.Hash
	// Encode does abi encoding of given event
	Encode() ([]byte, error)
	// ParseLog parses the provided receipt log to given event type
	ParseLog(log *ethgo.Log) (bool, error)
}

/* START ALPHA CHANGE 481 -- remove helper dependencies on generated bridge and state-sync bindings -- */
/* FINISH ALPHA CHANGE 481 -- remove helper dependencies on generated bridge and state-sync bindings -- */
