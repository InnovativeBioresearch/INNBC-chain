/* START ALPHA CHANGE 3168 -- reject unreadable recent segments instead of selecting an older startup head -- */
package segmented

import (
	"fmt"

	bcstorage "github.com/innovativebioresearch/innbc-chain/blockchain/storage"
)

func (s *Storage) ReadStartupState() (bcstorage.StartupState, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.closed {
		return bcstorage.StartupState{}, fmt.Errorf("segmented blockchain storage is closed")
	}

	genesisStorage := s.segments[0]
	if genesisStorage == nil {
		return bcstorage.StartupState{}, fmt.Errorf("permanent genesis database is missing")
	}
	result, err := genesisStorage.ReadStartupState()
	if err != nil {
		return bcstorage.StartupState{}, fmt.Errorf("inspect permanent genesis database: %w", err)
	}
	if !result.Empty &&
		(result.Head == nil ||
			result.Head.Number != 0 ||
			!result.HasGenesis ||
			result.Head.Hash != result.GenesisHash) {
		return bcstorage.StartupState{}, fmt.Errorf("invalid permanent genesis startup metadata")
	}

	for _, start := range s.starts {
		if start == 0 {
			continue
		}
		segment := s.segments[start]
		if segment == nil {
			return bcstorage.StartupState{}, fmt.Errorf("blockchain segment %d is missing", start)
		}

		local, err := segment.ReadStartupState()
		if err != nil {
			return bcstorage.StartupState{}, fmt.Errorf(
				"inspect blockchain segment %d: %w", start, err,
			)
		}
		if local.Empty {
			continue
		}
		if local.Head == nil ||
			local.HasGenesis ||
			s.segmentStart(local.Head.Number) != start {
			return bcstorage.StartupState{}, fmt.Errorf(
				"invalid startup metadata for blockchain segment %d", start,
			)
		}
		if result.Empty {
			return bcstorage.StartupState{}, fmt.Errorf(
				"blockchain segment %d contains records but permanent genesis storage is empty",
				start,
			)
		}
		if local.Head.Number > result.Head.Number {
			result.Head = local.Head
		}
	}

	return result, nil
}

/* FINISH ALPHA CHANGE 3168 -- reject unreadable recent segments instead of selecting an older startup head -- */
