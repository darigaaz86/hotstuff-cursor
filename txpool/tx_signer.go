package txpool

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/relab/hotstuff"
	"golang.org/x/crypto/sha3"
)

// TransactionSigner provides utilities for signing transactions
type TransactionSigner struct {
	chainID *big.Int
}

// NewTransactionSigner creates a new transaction signer
func NewTransactionSigner(chainID *big.Int) *TransactionSigner {
	return &TransactionSigner{
		chainID: chainID,
	}
}

// SignTransaction signs a transaction with the given private key
func (s *TransactionSigner) SignTransaction(tx *Transaction, privateKey *ecdsa.PrivateKey) error {
	// Calculate the transaction hash for signing
	txHash := s.hashForSigning(tx)

	// Sign the hash
	r, sigS, err := ecdsa.Sign(rand.Reader, privateKey, txHash[:])
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	// EIP-155: v = CHAIN_ID * 2 + 35 + recovery_id
	// For simplicity, we'll use recovery_id = 0
	v := big.NewInt(27) // recovery_id 0 + 27
	if s.chainID != nil && s.chainID.Sign() != 0 {
		v = new(big.Int).Mul(s.chainID, big.NewInt(2))
		v = new(big.Int).Add(v, big.NewInt(35)) // EIP-155: 35 + recovery_id
	}

	// Set signature values
	tx.V = v
	tx.R = r
	tx.S = sigS

	return nil
}

// hashForSigning calculates the hash that should be signed
func (s *TransactionSigner) hashForSigning(tx *Transaction) hotstuff.Hash {
	hasher := sha3.NewLegacyKeccak256()

	// Write transaction fields
	hasher.Write(s.encodeUint64(tx.Nonce))
	hasher.Write(tx.GasPrice.Bytes())
	hasher.Write(s.encodeUint64(tx.GasLimit))

	if tx.To != nil {
		hasher.Write(tx.To[:])
	}

	hasher.Write(tx.Value.Bytes())
	hasher.Write(tx.Data)

	// EIP-155: include chain ID in hash
	if s.chainID != nil && s.chainID.Sign() != 0 {
		hasher.Write(tx.ChainID.Bytes())
		hasher.Write([]byte{0})
		hasher.Write([]byte{0})
	}

	var hash hotstuff.Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

// GenerateKeyPair generates a new ECDSA key pair for demo purposes
func GenerateKeyPair() (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	privateKey, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return privateKey, &privateKey.PublicKey, nil
}

// AddressFromPublicKey derives an Ethereum address from a public key
func AddressFromPublicKey(pubKey *ecdsa.PublicKey) Address {
	hasher := sha3.NewLegacyKeccak256()

	// Encode public key as 64 bytes (32 bytes X + 32 bytes Y), no prefix
	xBytes := pubKey.X.Bytes()
	yBytes := pubKey.Y.Bytes()

	// Ensure each coordinate is exactly 32 bytes
	x32 := make([]byte, 32)
	y32 := make([]byte, 32)
	copy(x32[32-len(xBytes):], xBytes)
	copy(y32[32-len(yBytes):], yBytes)

	// Concatenate X and Y coordinates
	pubKeyBytes := append(x32, y32...)
	hasher.Write(pubKeyBytes)

	hash := hasher.Sum(nil)

	// Take the last 20 bytes as the address
	var addr Address
	copy(addr[:], hash[12:])
	return addr
}

// encodeUint64 encodes a uint64 as bytes
func (s *TransactionSigner) encodeUint64(n uint64) []byte {
	if n == 0 {
		return []byte{}
	}

	bytes := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		bytes[i] = byte(n)
		n >>= 8
		if n == 0 {
			return bytes[i:]
		}
	}
	return bytes
}

// CreateSignedTransaction creates and signs a transaction
func CreateSignedTransaction(
	nonce uint64,
	to *Address,
	value *big.Int,
	gasLimit uint64,
	gasPrice *big.Int,
	data []byte,
	chainID *big.Int,
	privateKey *ecdsa.PrivateKey,
) (*Transaction, error) {
	tx := &Transaction{
		Nonce:    nonce,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
		To:       to,
		Value:    value,
		Data:     data,
		ChainID:  chainID,
	}

	signer := NewTransactionSigner(chainID)
	if err := signer.SignTransaction(tx, privateKey); err != nil {
		return nil, err
	}

	return tx, nil
}

// GetCurve returns the elliptic curve used for ECDSA
func GetCurve() elliptic.Curve {
	return btcec.S256()
}
