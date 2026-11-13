package genesis

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2042 -- test proof-bearing explicit genesis entries with IPv6 addresses -- */
/* START ALPHA CHANGE 3843 -- preserve IPv6 proof coverage with a complete bootstrap identity -- */
func TestExplicitGenesisBLSPoP(t *testing.T) {
	key, err := bls.UnmarshalPrivateKey([]byte("7"))
	require.NoError(t, err)
	operator := types.StringToAddress("0x10001")
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	const bootstrapAddr = "/ip6/::1/tcp/30301/p2p/16Uiu2HAm22et517T1XhG181xGUDikDxUFrek4CuYTgUJjC38vxZR"

	entry := strings.Join([]string{
		bootstrapAddr,
		operator.String(),
		"0x" + hex.EncodeToString(key.PublicKey().Marshal()),
		"0x" + hex.EncodeToString(proofBytes),
	}, ":")

	params := &genesisParams{validators: []string{entry}}
	validators, err := params.getValidatorAccounts()
	require.NoError(t, err)
	require.Len(t, validators, 1)
	require.Equal(t, bootstrapAddr, validators[0].MultiAddr)
	require.Equal(t, operator, validators[0].Address)
	_, err = validators[0].ToValidatorMetadata()
	require.NoError(t, err)

	params.validators = []string{
		strings.TrimSuffix(entry, ":0x"+hex.EncodeToString(proofBytes)),
	}
	_, err = params.getValidatorAccounts()
	require.Error(t, err)
}

/* FINISH ALPHA CHANGE 3843 -- preserve IPv6 proof coverage with a complete bootstrap identity -- */

/* FINISH ALPHA CHANGE 2042 -- test proof-bearing explicit genesis entries with IPv6 addresses -- */
