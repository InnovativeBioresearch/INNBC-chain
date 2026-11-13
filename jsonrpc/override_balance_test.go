/* START ALPHA CHANGE 3097 -- verify full-width balance overrides validation and real EVM state isolation -- */
package jsonrpc

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func overrideBalanceQuote(t *testing.T, value string) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func overrideBalanceMaximum() *big.Int {
	return new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256),
		big.NewInt(1),
	)
}

func overrideBalanceInvalidInputs(t *testing.T) []struct {
	name string
	raw  string
} {
	t.Helper()

	aboveMaximum := new(big.Int).Lsh(big.NewInt(1), 256)

	return []struct {
		name string
		raw  string
	}{
		{"above_max_hex", overrideBalanceQuote(t, "0x"+aboveMaximum.Text(16))},
		{"above_max_decimal_string", overrideBalanceQuote(t, aboveMaximum.String())},
		{"above_max_json_integer", aboveMaximum.String()},
		{"negative_string", `"-1"`},
		{"negative_number", `-1`},
		{"negative_zero", `-0`},
		{"positive_sign", `"+1"`},
		{"fraction_string", `"1.5"`},
		{"fraction_number", `1.5`},
		{"exponent_string", `"1e2"`},
		{"exponent_number", `1e2`},
		{"empty_string", `""`},
		{"empty_hex", `"0x"`},
		{"invalid_hex", `"0xzz"`},
		{"partially_valid_hex", `"0x1234zz"`},
		{"prefixless_hex_letters", `"ff"`},
		{"uppercase_prefix", `"0X10"`},
		{"whitespace_string", `" 1"`},
		{"boolean", `true`},
		{"array", `[1]`},
		{"object", `{"value":1}`},
		{"long_hex", overrideBalanceQuote(t, "0x1"+strings.Repeat("0", 1024))},
		{"long_decimal_string", overrideBalanceQuote(t, "1"+strings.Repeat("0", 1024))},
		{"long_json_integer", "1" + strings.Repeat("0", 1024)},
	}
}

func TestOverrideBalance_FullWidthFormatsAndDetachedConversion(t *testing.T) {
	values := []struct {
		name  string
		value *big.Int
	}{
		{"zero", new(big.Int)},
		{"max_uint64", new(big.Int).SetUint64(^uint64(0))},
		{"two_to_64", new(big.Int).Lsh(big.NewInt(1), 64)},
		{
			"large_balance",
			new(big.Int).Add(
				new(big.Int).Lsh(big.NewInt(1), 200),
				big.NewInt(11),
			),
		},
		{"max_uint256", overrideBalanceMaximum()},
	}

	for _, value := range values {
		formats := []struct {
			name string
			raw  string
		}{
			{"hex_string", overrideBalanceQuote(t, "0x"+value.value.Text(16))},
			{"decimal_string", overrideBalanceQuote(t, value.value.String())},
			{"json_integer", value.value.String()},
			{
				"leading_zero_hex",
				overrideBalanceQuote(t,
					"0x"+strings.Repeat("0", 1024)+value.value.Text(16),
				),
			},
			{
				"leading_zero_decimal",
				overrideBalanceQuote(t,
					strings.Repeat("0", 1024)+value.value.String(),
				),
			},
		}

		for _, format := range formats {
			t.Run(value.name+"/"+format.name, func(t *testing.T) {
				var account overrideAccount
				require.NoError(t, json.Unmarshal(
					[]byte(`{"balance":`+format.raw+`}`),
					&account,
				))
				require.NotNil(t, account.Balance)
				require.Zero(t,
					value.value.Cmp((*big.Int)(account.Balance)),
				)

				converted := account.ToType()
				require.NotNil(t, converted.Balance)
				require.Zero(t, value.value.Cmp(converted.Balance))

				// Mutating the execution value must not modify the request.
				converted.Balance.Add(converted.Balance, big.NewInt(1))
				require.Zero(t,
					value.value.Cmp((*big.Int)(account.Balance)),
				)

				second := account.ToType()
				require.Zero(t, value.value.Cmp(second.Balance))
				require.NotSame(t, converted.Balance, second.Balance)

				encoded, err := json.Marshal(account.Balance)
				require.NoError(t, err)
				require.Equal(t,
					`"0x`+value.value.Text(16)+`"`,
					string(encoded),
				)
			})
		}
	}
}

func TestOverrideBalance_DecimalMeaningAndHexDigits(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{
		{`"10"`, 10},
		{`10`, 10},
		{`"010"`, 10},
		{`"0x10"`, 16},
		{`"0xabc"`, 2748},
		{`"0xAbCd"`, 43981},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var account overrideAccount
			require.NoError(t, json.Unmarshal(
				[]byte(`{"balance":`+tc.raw+`}`),
				&account,
			))
			require.NotNil(t, account.Balance)
			require.Zero(t,
				big.NewInt(tc.want).Cmp(account.ToType().Balance),
			)
		})
	}
}

func TestOverrideBalance_InvalidInputPreservesReceiver(t *testing.T) {
	for _, tc := range overrideBalanceInvalidInputs(t) {
		t.Run(tc.name, func(t *testing.T) {
			var receiver argUint256
			require.NoError(t, receiver.UnmarshalText([]byte("7")))

			require.Error(t, json.Unmarshal([]byte(tc.raw), &receiver))
			require.Zero(t,
				big.NewInt(7).Cmp((*big.Int)(&receiver)),
			)

			var account overrideAccount
			require.Error(t, json.Unmarshal(
				[]byte(`{"balance":`+tc.raw+`}`),
				&account,
			))
		})
	}

	for _, input := range []string{
		"-1",
		"+1",
		"1.5",
		"1e2",
		"",
		"0x",
		"0x1234zz",
		"0x1" + strings.Repeat("0", 64),
		"1" + strings.Repeat("0", 78),
	} {
		t.Run("text/"+input, func(t *testing.T) {
			var receiver argUint256
			require.NoError(t, receiver.UnmarshalText([]byte("7")))
			require.Error(t, receiver.UnmarshalText([]byte(input)))
			require.Zero(t,
				big.NewInt(7).Cmp((*big.Int)(&receiver)),
			)
		})
	}
}

func TestOverrideBalance_OptionalFieldsAndNonceWidth(t *testing.T) {
	for _, input := range []string{
		`{}`,
		`{"balance":null}`,
	} {
		var account overrideAccount
		require.NoError(t, json.Unmarshal([]byte(input), &account))
		require.Nil(t, account.Balance)
		require.Nil(t, account.ToType().Balance)
	}

	for _, input := range []string{
		`{"balance":0}`,
		`{"balance":"0"}`,
		`{"balance":"0x0"}`,
	} {
		var account overrideAccount
		require.NoError(t, json.Unmarshal([]byte(input), &account))
		require.NotNil(t, account.Balance)
		require.NotNil(t, account.ToType().Balance)
		require.Zero(t, account.ToType().Balance.Sign())
	}

	var account overrideAccount
	require.NoError(t, json.Unmarshal(
		[]byte(`{"nonce":"0xffffffffffffffff","balance":"0x10000000000000000"}`),
		&account,
	))
	require.NotNil(t, account.Nonce)
	require.Equal(t, ^uint64(0), *account.ToType().Nonce)

	require.Error(t, json.Unmarshal(
		[]byte(`{"nonce":"0x10000000000000000"}`),
		&overrideAccount{},
	))
}

func TestOverrideBalance_DispatchRejectsBeforeWork(t *testing.T) {
	address := types.StringToAddress("0x9002").String()

	for _, tc := range overrideBalanceInvalidInputs(t) {
		t.Run(tc.name, func(t *testing.T) {
			dispatcher, counts := hexInputDispatcher(t)
			account := map[string]any{
				"balance": json.RawMessage(tc.raw),
			}
			params := []any{
				hexInputTransactionArgs(),
				"latest",
				map[string]any{address: account},
			}

			response := hexInputRequest(t, dispatcher, "eth_call", params)
			hexInputRequireError(t, response, -32602)
			hexInputRequireNoWork(t, counts)

			account["balance"] = "0x10000000000000000"
			response = hexInputRequest(t, dispatcher, "eth_call", params)
			require.Nil(t, response.Error)
			require.Equal(t, int32(1), counts.endpoints.Load())
			require.Equal(t, int32(1), counts.apply.Load())
			require.True(t, counts.backend.Load() > 0)
			require.Zero(t, counts.trace.Load())
			require.Zero(t, counts.pool.Load())
		})
	}
}

func TestOverrideBalance_ActualExecutionAndStateIsolation(t *testing.T) {
	large := new(big.Int).Add(
		new(big.Int).Lsh(big.NewInt(1), 200),
		big.NewInt(11),
	)
	cases := []struct {
		name    string
		include bool
		balance any
		want    *big.Int
	}{
		{"omitted", false, nil, big.NewInt(7)},
		{"null", true, nil, big.NewInt(7)},
		{"explicit_zero", true, "0x0", new(big.Int)},
		{
			"two_to_64",
			true,
			"0x10000000000000000",
			new(big.Int).Lsh(big.NewInt(1), 64),
		},
		{"large_balance", true, large.String(), large},
		{
			"max_uint256",
			true,
			"0x" + overrideBalanceMaximum().Text(16),
			overrideBalanceMaximum(),
		},
	}

	// ADDRESS; BALANCE; PUSH1 0; MSTORE;
	// PUSH1 32; PUSH1 0; RETURN.
	balanceCode := []byte{
		0x30, 0x31, 0x60, 0x00, 0x52,
		0x60, 0x20, 0x60, 0x00, 0xf3,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, args := newSimulationNonceFixture(t)

			// Persist a known original balance while retaining the fixture's
			// original contract code and sender nonce.
			root, err := store.executor.WriteGenesis(
				map[types.Address]*chain.GenesisAccount{
					store.target: {Balance: big.NewInt(7)},
				},
				store.header.StateRoot,
			)
			require.NoError(t, err)
			store.header.StateRoot = root
			store.header.ComputeHash()

			snapshot, err := store.executor.StateAt(root)
			require.NoError(t, err)
			before := state.NewTxn(snapshot)
			originalBalance := new(big.Int).Set(before.GetBalance(store.target))
			originalCode := append([]byte(nil), before.GetCode(store.target)...)
			originalNonce := before.GetNonce(store.sender)
			originalSenderBalance := new(big.Int).Set(before.GetBalance(store.sender))
			require.NoError(t, before.Error())
			require.Zero(t, originalBalance.Cmp(big.NewInt(7)))

			// The contract must return the override itself, without an
			// incoming value transfer increasing the observed balance.
			args.Value = argBytesPtr([]byte{})
			argsBefore := *args

			account := map[string]any{
				"code": "0x" + hex.EncodeToString(balanceCode),
			}
			if tc.include {
				account["balance"] = tc.balance
			}

			encoded, err := json.Marshal(map[string]any{
				store.target.String(): account,
			})
			require.NoError(t, err)

			var overrides stateOverride
			require.NoError(t, json.Unmarshal(encoded, &overrides))
			overrideBefore, err := json.Marshal(overrides)
			require.NoError(t, err)

			result, err := newTestEthEndpoint(store).Call(
				args,
				BlockNumberOrHash{},
				&overrides,
			)
			require.NoError(t, err)

			output, ok := result.(*argBytes)
			require.True(t, ok)
			require.Len(t, *output, 32)
			require.Zero(t,
				tc.want.Cmp(new(big.Int).SetBytes(*output)),
			)
			require.Equal(t, 1, store.calls)
			require.Equal(t, argsBefore, *args)
			require.Equal(t, root, store.header.StateRoot)

			overrideAfter, err := json.Marshal(overrides)
			require.NoError(t, err)
			require.Equal(t, overrideBefore, overrideAfter)

			// Reopen the original committed root after actual EVM execution.
			snapshot, err = store.executor.StateAt(root)
			require.NoError(t, err)
			after := state.NewTxn(snapshot)
			require.Zero(t,
				originalBalance.Cmp(after.GetBalance(store.target)),
			)
			require.Equal(t, originalCode, after.GetCode(store.target))
			require.Equal(t, originalNonce, after.GetNonce(store.sender))
			require.Zero(t,
				originalSenderBalance.Cmp(after.GetBalance(store.sender)),
			)
			require.Equal(t, types.ZeroHash,
				after.GetState(store.target, types.ZeroHash),
			)
			require.NoError(t, after.Error())
		})
	}
}

/* FINISH ALPHA CHANGE 3097 -- verify full-width balance overrides validation and real EVM state isolation -- */
