/* START ALPHA CHANGE 2970 -- validate RLP structure without recursive decoding -- */
package types

import "errors"

var (
	ErrInvalidRLPStructure = errors.New("invalid RLP structure")
	ErrRLPNestingTooDeep   = errors.New("RLP list nesting exceeds the supported structure")
)

// BlockRLPMaxDepth covers the current block representation:
//
//	block -> transactions -> transaction -> empty access list.
//
// Header fields and uncle headers require fewer list levels.
// Embedded header extra data is checked independently by its decoder.
const BlockRLPMaxDepth = 4

// ValidateRLPStructure checks one complete canonical RLP item.
//
// It skips byte-string payloads, walks lists without recursion, and checks
// every item against its enclosing list before converting lengths to int.
// It does not decode values or allocate storage proportional to the input.
//
// This is a structural check, not transaction or consensus validation.
func ValidateRLPStructure(input []byte, maxDepth int) error {
	var ends [64]int

	if len(input) == 0 || maxDepth < 1 || maxDepth > len(ends) {
		return ErrInvalidRLPStructure
	}

	offset := 0
	depth := 0
	rootSeen := false

	for {
		for depth > 0 && offset == ends[depth-1] {
			depth--
		}

		if depth == 0 && rootSeen {
			if offset != len(input) {
				return ErrInvalidRLPStructure
			}
			return nil
		}

		limit := len(input)
		if depth > 0 {
			limit = ends[depth-1]
		}
		if offset >= limit {
			return ErrInvalidRLPStructure
		}

		list, payloadStart, itemEnd, err := rlpStructureItem(input, offset, limit)
		if err != nil {
			return err
		}
		rootSeen = true

		if list {
			if depth >= maxDepth {
				return ErrRLPNestingTooDeep
			}
			ends[depth] = itemEnd
			depth++
			offset = payloadStart
		} else {
			offset = itemEnd
		}
	}
}

func rlpStructureItem(input []byte, offset, limit int) (bool, int, int, error) {
	prefix := input[offset]
	if prefix < 0x80 {
		return false, offset, offset + 1, nil
	}

	var (
		list        bool
		longForm    bool
		headerBytes = 1
		size        uint64
	)

	switch {
	case prefix <= 0xb7:
		size = uint64(prefix - 0x80)
	case prefix <= 0xbf:
		longForm = true
		headerBytes += int(prefix - 0xb7)
	case prefix <= 0xf7:
		list = true
		size = uint64(prefix - 0xc0)
	default:
		list = true
		longForm = true
		headerBytes += int(prefix - 0xf7)
	}

	if headerBytes > limit-offset {
		return false, 0, 0, ErrInvalidRLPStructure
	}

	if longForm {
		if input[offset+1] == 0 {
			return false, 0, 0, ErrInvalidRLPStructure
		}
		for _, value := range input[offset+1 : offset+headerBytes] {
			size = size<<8 | uint64(value)
		}
		if size < 56 {
			return false, 0, 0, ErrInvalidRLPStructure
		}
	}

	payloadStart := offset + headerBytes
	if size > uint64(limit-payloadStart) {
		return false, 0, 0, ErrInvalidRLPStructure
	}
	itemEnd := payloadStart + int(size)

	if !list && size == 1 && input[payloadStart] < 0x80 {
		return false, 0, 0, ErrInvalidRLPStructure
	}

	return list, payloadStart, itemEnd, nil
}

/* FINISH ALPHA CHANGE 2970 -- validate RLP structure without recursive decoding -- */
