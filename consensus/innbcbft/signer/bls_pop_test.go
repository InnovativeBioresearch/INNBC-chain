package signer

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
	bn256 "github.com/umbracle/go-eth-bn256"
)

/* START ALPHA CHANGE 2038 -- test possession binding canonical input rejection and the rogue-key construction -- */
func TestINNBCBLSPoP(t *testing.T) {
	key, err := bls.UnmarshalPrivateKey([]byte("11"))
	require.NoError(t, err)
	otherKey, err := bls.UnmarshalPrivateKey([]byte("13"))
	require.NoError(t, err)

	operator := types.StringToAddress("0x1001")
	publicKey := key.PublicKey().Marshal()
	proof, err := MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	decoded, err := VerifyINNBCBLSPoP(publicKey, operator, proofBytes)
	require.NoError(t, err)
	require.Equal(t, publicKey, decoded.Marshal())

	wrongDomainProof, err := key.Sign(
		innbcBLSPoPMessage(operator, publicKey),
		DomainCheckpointManager,
	)
	require.NoError(t, err)
	wrongDomainBytes, err := wrongDomainProof.Marshal()
	require.NoError(t, err)

	altered := append([]byte(nil), proofBytes...)
	altered[len(altered)-1] ^= 1

	cases := []struct {
		name     string
		key      []byte
		operator types.Address
		proof    []byte
	}{
		{"wrong operator", publicKey, types.StringToAddress("0x1002"), proofBytes},
		{"zero operator", publicKey, types.ZeroAddress, proofBytes},
		{"wrong key", otherKey.PublicKey().Marshal(), operator, proofBytes},
		{"wrong domain", publicKey, operator, wrongDomainBytes},
		{"altered proof", publicKey, operator, altered},
		{"missing proof", publicKey, operator, nil},
		{"short proof", publicKey, operator, proofBytes[:63]},
		{"long proof", publicKey, operator, append(append([]byte(nil), proofBytes...), 0)},
		{"short key", publicKey[:127], operator, proofBytes},
		{"long key", append(append([]byte(nil), publicKey...), 0), operator, proofBytes},
		{"infinity key", make([]byte, 128), operator, proofBytes},
		{"infinity proof", publicKey, operator, make([]byte, 64)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := VerifyINNBCBLSPoP(tc.key, tc.operator, tc.proof)
			require.Error(t, err)
		})
	}

	_, err = MakeINNBCBLSPoP(nil, operator)
	require.Error(t, err)
	_, err = MakeINNBCBLSPoP(bls.NewZeroPrivateKey(), operator)
	require.Error(t, err)
}

func TestINNBCBLSPoPRejectsRogueKeyProof(t *testing.T) {
	honest, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)
	attacker, err := bls.UnmarshalPrivateKey([]byte("11"))
	require.NoError(t, err)

	honestPoint := new(bn256.G2)
	_, err = honestPoint.Unmarshal(honest.PublicKey().Marshal())
	require.NoError(t, err)

	aggregatePoint := new(bn256.G2)
	_, err = aggregatePoint.Unmarshal(attacker.PublicKey().Marshal())
	require.NoError(t, err)

	roguePoint := new(bn256.G2).Add(
		aggregatePoint,
		new(bn256.G2).Neg(honestPoint),
	)
	rogueKey, err := bls.UnmarshalPublicKey(roguePoint.Marshal())
	require.NoError(t, err)

	message := []byte("forged checkpoint")
	forged, err := attacker.Sign(message, DomainCheckpointManager)
	require.NoError(t, err)

	require.True(t, forged.VerifyAggregated(
		[]*bls.PublicKey{honest.PublicKey(), rogueKey},
		message,
		DomainCheckpointManager,
	))

	operator := types.StringToAddress("0x1003")
	fakeProof, err := attacker.Sign(
		innbcBLSPoPMessage(operator, rogueKey.Marshal()),
		DomainINNBCBLSPoP,
	)
	require.NoError(t, err)
	fakeProofBytes, err := fakeProof.Marshal()
	require.NoError(t, err)

	_, err = VerifyINNBCBLSPoP(rogueKey.Marshal(), operator, fakeProofBytes)
	require.Error(t, err)
}

/* FINISH ALPHA CHANGE 2038 -- test possession binding canonical input rejection and the rogue-key construction -- */
