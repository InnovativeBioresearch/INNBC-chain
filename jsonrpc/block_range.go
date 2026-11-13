/* START ALPHA CHANGE 2480 -- resolve zero log-range limits to a finite default -- */
package jsonrpc

const DefaultBlockRangeLimit uint64 = 256

func ResolveBlockRangeLimit(limit uint64) uint64 {
	if limit == 0 {
		return DefaultBlockRangeLimit
	}

	return limit
}

/* FINISH ALPHA CHANGE 2480 -- resolve zero log-range limits to a finite default -- */
