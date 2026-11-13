/* START ALPHA CHANGE 3182 -- strictly parse optional external genesis trie roots without normalization -- */
package genesis

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/innovativebioresearch/innbc-chain/types"
)

// parseInitialTrieRoot preserves empty input as the no-external-root default.
// Nonempty input must contain exactly 64 hexadecimal digits, with an optional
// lowercase 0x prefix. A full-length zero hash retains its existing meaning.
func (p *genesisParams) parseInitialTrieRoot() (types.Hash, error) {
	raw := p.initialStateRoot
	if raw == "" {
		return types.ZeroHash, nil
	}

	payload := strings.TrimPrefix(raw, "0x")
	if len(payload) != types.HashLength*2 {
		return types.ZeroHash, fmt.Errorf(
			"invalid --%s value %q: expected exactly %d hexadecimal digits",
			trieRootFlag, raw, types.HashLength*2,
		)
	}

	decoded, err := hex.DecodeString(payload)
	if err != nil {
		return types.ZeroHash, fmt.Errorf(
			"invalid --%s value %q: %w",
			trieRootFlag, raw, err,
		)
	}

	var root types.Hash
	copy(root[:], decoded)

	return root, nil
}

/* FINISH ALPHA CHANGE 3182 -- strictly parse optional external genesis trie roots without normalization -- */
