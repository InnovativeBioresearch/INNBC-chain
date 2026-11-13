/* START ALPHA CHANGE 3166 -- inspect startup metadata without suppressing read errors or rewriting records -- */
package storage

import (
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/types"
)

func (s *KeyValueStorage) ReadStartupState() (StartupState, error) {
	empty, err := s.db.IsEmpty()
	if err != nil {
		return StartupState{}, fmt.Errorf("inspect database emptiness: %w", err)
	}
	if empty {
		return StartupState{Empty: true}, nil
	}

	hashData, _, err := s.startupValue(HEAD, HASH, types.HashLength, true)
	if err != nil {
		return StartupState{}, err
	}
	numberData, _, err := s.startupValue(HEAD, NUMBER, 8, true)
	if err != nil {
		return StartupState{}, err
	}
	genesisData, hasGenesis, err := s.startupValue(
		CANONICAL, common.EncodeUint64ToBytes(0), types.HashLength, false,
	)
	if err != nil {
		return StartupState{}, err
	}

	headHash := types.BytesToHash(hashData)
	headNumber := common.EncodeBytesToUint64(numberData)
	canonicalData, _, err := s.startupValue(
		CANONICAL, common.EncodeUint64ToBytes(headNumber), types.HashLength, true,
	)
	if err != nil {
		return StartupState{}, err
	}
	if types.BytesToHash(canonicalData) != headHash {
		return StartupState{}, fmt.Errorf(
			"head hash does not match canonical block %d", headNumber,
		)
	}

	header, err := s.ReadHeader(headHash)
	if err != nil {
		return StartupState{}, fmt.Errorf("read startup head header %s: %w", headHash, err)
	}
	if header == nil || header.Number != headNumber {
		return StartupState{}, fmt.Errorf(
			"stored head header does not match head number %d", headNumber,
		)
	}

	result := StartupState{
		Head:       header,
		HasGenesis: hasGenesis,
	}
	if hasGenesis {
		result.GenesisHash = types.BytesToHash(genesisData)
		genesisHeader := header
		if headNumber != 0 {
			genesisHeader, err = s.ReadHeader(result.GenesisHash)
			if err != nil {
				return StartupState{}, fmt.Errorf("read startup genesis header: %w", err)
			}
		}
		if genesisHeader == nil ||
			genesisHeader.Number != 0 ||
			genesisHeader.Hash != result.GenesisHash {
			return StartupState{}, fmt.Errorf("invalid stored genesis header")
		}
	}

	return result, nil
}

func (s *KeyValueStorage) startupValue(
	prefix []byte,
	key []byte,
	size int,
	required bool,
) ([]byte, bool, error) {
	fullKey := append(append([]byte(nil), prefix...), key...)
	data, found, err := s.db.Get(fullKey)
	if err != nil {
		return nil, false, fmt.Errorf("read startup record %x: %w", fullKey, err)
	}
	if !found {
		if required {
			return nil, false, fmt.Errorf("required startup record %x is missing", fullKey)
		}
		return nil, false, nil
	}
	if len(data) != size {
		return nil, false, fmt.Errorf(
			"invalid startup record %x: expected %d bytes, got %d",
			fullKey, size, len(data),
		)
	}

	return data, true, nil
}

/* FINISH ALPHA CHANGE 3166 -- inspect startup metadata without suppressing read errors or rewriting records -- */
