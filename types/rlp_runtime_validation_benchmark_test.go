/* START ALPHA CHANGE 3018 -- measure runtime preflight and decoding overhead -- */
package types

import "testing"

func BenchmarkRuntimeRLP_Preflight(b *testing.B) {
	legacy := runtimeRLPTestTransaction(LegacyTx)
	legacy.Input = make([]byte, 125_000)

	dynamic := runtimeRLPTestTransaction(DynamicFeeTx)
	dynamic.Input = make([]byte, 125_000)

	block := &Block{Header: &Header{Number: 1}}
	for index := 0; index < 10; index++ {
		block.Transactions = append(block.Transactions, dynamic.Copy())
	}

	// Encoding fixtures only; no finalized-block validity is claimed.
	for _, fixture := range []struct {
		name  string
		raw   []byte
		kind  TxType
		block bool
	}{
		{"legacy_large", legacy.MarshalRLP(), LegacyTx, false},
		{"dynamic_large", dynamic.MarshalRLP(), DynamicFeeTx, false},
		{"block_ten_dynamic", block.MarshalRLP(), LegacyTx, true},
	} {
		/* START ALPHA CHANGE 3031 -- identify the baseline as direct parser entry with shared validation -- */
		for _, mode := range []string{"guard_only", "guarded_decode", "parser_entry_baseline"} {
			/* FINISH ALPHA CHANGE 3031 -- identify the baseline as direct parser entry with shared validation -- */
			b.Run(fixture.name+"/"+mode, func(b *testing.B) {
				raw := fixture.raw
				payload := raw
				if !fixture.block && fixture.kind != LegacyTx {
					payload = raw[1:]
				}

				b.ReportAllocs()
				b.SetBytes(int64(len(raw)))
				b.ResetTimer()

				for index := 0; index < b.N; index++ {
					var err error

					switch mode {
					case "guard_only":
						if fixture.block {
							err = validateRuntimeBlockRLP(raw)
						} else {
							err = validateRuntimeTransactionRLP(payload, fixture.kind)
						}
					case "guarded_decode":
						if fixture.block {
							err = new(Block).UnmarshalRLP(raw)
						} else {
							err = new(Transaction).UnmarshalRLP(raw)
						}
					case "parser_entry_baseline":
						// Direct parser entry bypasses public preflight.
						// Shared transaction-field validation still runs.
						if fixture.block {
							decoded := new(Block)
							err = UnmarshalRlp(decoded.unmarshalRLPFrom, raw)
						} else {
							decoded := &Transaction{Type: fixture.kind}
							err = UnmarshalRlp(decoded.unmarshalRLPFrom, payload)
						}
					}

					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3018 -- measure runtime preflight and decoding overhead -- */
