package signer

import (
	"fmt"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/types"
)

/* START ALPHA CHANGE 2000 -- generate and verify operator-bound canonical BLS possession proofs -- */
const DomainINNBCBLSPoPString = "DOMAIN_INNBC_BLS_PROOF_OF_POSSESSION"

var DomainINNBCBLSPoP = crypto.Keccak256([]byte(DomainINNBCBLSPoPString))

func MakeINNBCBLSPoP(
	privateKey *bls.PrivateKey,
	operator types.Address,
) (*bls.Signature, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("missing BLS private key")
	}
	if operator == types.ZeroAddress {
		return nil, fmt.Errorf("zero BLS proof operator")
	}

	publicKeyBytes := privateKey.PublicKey().Marshal()
	if _, err := bls.UnmarshalPublicKey(publicKeyBytes); err != nil {
		return nil, fmt.Errorf("invalid BLS signing key: %w", err)
	}

	return privateKey.Sign(
		innbcBLSPoPMessage(operator, publicKeyBytes),
		DomainINNBCBLSPoP,
	)
}

func VerifyINNBCBLSPoP(
	publicKeyBytes []byte,
	operator types.Address,
	proofBytes []byte,
) (*bls.PublicKey, error) {
	if operator == types.ZeroAddress {
		return nil, fmt.Errorf("zero BLS proof operator")
	}
	if len(publicKeyBytes) != bls.PublicKeySize {
		return nil, fmt.Errorf(
			"invalid BLS public key length: got %d, expected %d",
			len(publicKeyBytes), bls.PublicKeySize,
		)
	}
	if len(proofBytes) != bls.SignatureSize {
		return nil, fmt.Errorf(
			"invalid BLS proof length: got %d, expected %d",
			len(proofBytes), bls.SignatureSize,
		)
	}

	publicKey, err := bls.UnmarshalPublicKey(publicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid BLS public key: %w", err)
	}

	proof, err := bls.UnmarshalSignature(proofBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid BLS proof: %w", err)
	}

	if !proof.Verify(
		publicKey,
		innbcBLSPoPMessage(operator, publicKeyBytes),
		DomainINNBCBLSPoP,
	) {
		return nil, fmt.Errorf("BLS proof does not match operator and public key")
	}

	return publicKey, nil
}

func innbcBLSPoPMessage(operator types.Address, publicKeyBytes []byte) []byte {
	message := make([]byte, types.AddressLength+bls.PublicKeySize)
	copy(message[:types.AddressLength], operator[:])
	copy(message[types.AddressLength:], publicKeyBytes)

	return message
}

/* FINISH ALPHA CHANGE 2000 -- generate and verify operator-bound canonical BLS possession proofs -- */
