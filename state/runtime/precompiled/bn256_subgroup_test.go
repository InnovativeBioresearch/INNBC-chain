package precompiled

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	bn256 "github.com/umbracle/go-eth-bn256"
)

/* START ALPHA CHANGE 2039 -- reject on-curve non-subgroup inputs while preserving valid pairing identities -- */
func TestBN256PairingSubgroupValidation(t *testing.T) {
	precompile := &bn256Pairing{p: &Precompiled{}}

	result, err := precompile.run(nil, types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolTrue, result)

	result, err = precompile.run(make([]byte, 192), types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolTrue, result)

	/* With an infinity G1 input, the old implementation skipped this pair
	   and accepted the invalid G2 input. Membership must still be checked. */
	invalidKey := nonSubgroupG2ForTest(t)
	input := append(make([]byte, 64), invalidKey...)
	_, err = precompile.run(input, types.ZeroAddress, nil)
	require.ErrorContains(t, err, "outside the prime-order subgroup")

	g1 := new(bn256.G1).ScalarBaseMult(big.NewInt(1))
	g2 := new(bn256.G2).ScalarBaseMult(big.NewInt(1))
	negativeG1 := new(bn256.G1).Neg(g1)

	input = append([]byte{}, g1.Marshal()...)
	input = append(input, g2.Marshal()...)
	input = append(input, negativeG1.Marshal()...)
	input = append(input, g2.Marshal()...)

	result, err = precompile.run(input, types.ZeroAddress, nil)
	require.NoError(t, err)
	require.Equal(t, abiBoolTrue, result)
}

func nonSubgroupG2ForTest(t *testing.T) []byte {
	t.Helper()

	modulus, ok := new(big.Int).SetString(
		"21888242871839275222246405745257275088696311157297823662689037894645226208583",
		10,
	)
	require.True(t, ok)

	/* The twist is y² = x³ + 3/(9+i).
	   Therefore its constant is 27/82 - (3/82)i. */
	inverse82 := new(big.Int).ModInverse(big.NewInt(82), modulus)
	require.NotNil(t, inverse82)
	inverse2 := new(big.Int).ModInverse(big.NewInt(2), modulus)
	require.NotNil(t, inverse2)

	realB := new(big.Int).Mod(
		new(big.Int).Mul(big.NewInt(27), inverse82), modulus,
	)
	imagB := new(big.Int).Mod(
		new(big.Int).Mul(big.NewInt(-3), inverse82), modulus,
	)

	for x := int64(0); x < 256; x++ {
		realRHS := new(big.Int).Exp(big.NewInt(x), big.NewInt(3), modulus)
		realRHS.Add(realRHS, realB).Mod(realRHS, modulus)

		norm := new(big.Int).Mul(realRHS, realRHS)
		norm.Add(norm, new(big.Int).Mul(imagB, imagB)).Mod(norm, modulus)
		root := new(big.Int).ModSqrt(norm, modulus)
		if root == nil {
			continue
		}

		for _, sign := range []int64{1, -1} {
			uSquared := new(big.Int).Mul(root, big.NewInt(sign))
			uSquared.Add(uSquared, realRHS)
			uSquared.Mul(uSquared, inverse2).Mod(uSquared, modulus)

			u := new(big.Int).ModSqrt(uSquared, modulus)
			if u == nil || u.Sign() == 0 {
				continue
			}

			inverse2U := new(big.Int).ModInverse(
				new(big.Int).Lsh(new(big.Int).Set(u), 1), modulus,
			)
			if inverse2U == nil {
				continue
			}
			v := new(big.Int).Mod(
				new(big.Int).Mul(imagB, inverse2U), modulus,
			)

			encoded := make([]byte, 128)
			big.NewInt(x).FillBytes(encoded[32:64])
			v.FillBytes(encoded[64:96])
			u.FillBytes(encoded[96:128])

			point := new(bn256.G2)
			if _, err := point.Unmarshal(encoded); err != nil {
				continue
			}
			if !point.IsInfinity() && !point.InCorrectSubgroup() {
				return encoded
			}
		}
	}

	t.Fatal("failed to construct the bounded non-subgroup test input")

	return nil
}

/* FINISH ALPHA CHANGE 2039 -- reject on-curve non-subgroup inputs while preserving valid pairing identities -- */
