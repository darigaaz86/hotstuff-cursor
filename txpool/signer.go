package txpool

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"

	"golang.org/x/crypto/sha3"
)

// Signer interface for transaction signing and recovery
type Signer interface {
	// Sender returns the sender address of the transaction
	Sender(tx *Transaction) (*Address, error)
	// SignTx signs the transaction and returns a copy with signature
	SignTx(tx *Transaction, privateKey *ecdsa.PrivateKey) (*Transaction, error)
}

// EIP155Signer implements EIP-155 signing with replay protection
type EIP155Signer struct {
	chainID *big.Int
}

// NewEIP155Signer creates a new EIP-155 signer
func NewEIP155Signer(chainID *big.Int) *EIP155Signer {
	return &EIP155Signer{
		chainID: chainID,
	}
}

// SignTx signs the transaction using EIP-155
func (s *EIP155Signer) SignTx(tx *Transaction, privateKey *ecdsa.PrivateKey) (*Transaction, error) {
	// Create a copy of the transaction
	txCopy := *tx

	// Create the hash for signing (includes chain ID for replay protection)
	hash := s.hash(tx)

	// Sign the hash
	signature, err := crypto_sign(hash[:], privateKey)
	if err != nil {
		return nil, err
	}

	// Extract r, s, v from signature
	r := new(big.Int).SetBytes(signature[:32])
	s_val := new(big.Int).SetBytes(signature[32:64])
	v := new(big.Int).SetInt64(int64(signature[64]))

	// Apply EIP-155: v = recovery_id + chain_id * 2 + 35
	v.Add(v, new(big.Int).Mul(s.chainID, big.NewInt(2)))
	v.Add(v, big.NewInt(35))

	txCopy.R = r
	txCopy.S = s_val
	txCopy.V = v

	return &txCopy, nil
}

// Sender recovers the sender address from the transaction signature
func (s *EIP155Signer) Sender(tx *Transaction) (*Address, error) {
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return nil, fmt.Errorf("transaction not signed")
	}

	// Simplified implementation for demo purposes
	// In production, this would use proper ECDSA recovery

	// Generate a deterministic address from transaction hash
	hash := s.hash(tx)
	addr := Address{}
	copy(addr[:], hash[:20])

	return &addr, nil
}

// hash creates the hash for signing/verification
func (s *EIP155Signer) hash(tx *Transaction) Hash {
	hasher := sha3.NewLegacyKeccak256()

	// For EIP-155, we include the chain ID in the hash
	data := s.encodeForSigning(tx)
	hasher.Write(data)

	var hash Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

// encodeForSigning creates the byte representation for signing
func (s *EIP155Signer) encodeForSigning(tx *Transaction) []byte {
	// This is a simplified encoding - in production, use proper RLP
	// For now, we'll use JSON with chain ID included
	data := fmt.Sprintf(`{"nonce":%d,"gasPrice":"%s","gasLimit":%d,"to":"%v","value":"%s","data":"%x","chainId":"%s"}`,
		tx.Nonce,
		tx.GasPrice.String(),
		tx.GasLimit,
		tx.To,
		tx.Value.String(),
		tx.Data,
		s.chainID.String(),
	)
	return []byte(data)
}

// crypto_sign signs a hash with the private key
func crypto_sign(hash []byte, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	// Sign the hash
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash)
	if err != nil {
		return nil, err
	}

	// Create signature bytes (64 bytes + recovery ID)
	signature := make([]byte, 65)

	// Copy r and s (32 bytes each)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)

	// Calculate recovery ID
	recoveryID := calculateRecoveryID(hash, r, s, &privateKey.PublicKey)
	signature[64] = byte(recoveryID)

	return signature, nil
}

// crypto_recover recovers the public key from signature
func crypto_recover(hash []byte, r, s *big.Int, recoveryID int64) (*ecdsa.PublicKey, error) {
	// This is a simplified implementation
	// In production, use a proper ECDSA recovery implementation

	// For now, return a dummy public key for testing
	// TODO: Implement proper ECDSA recovery
	curve := elliptic.P256()
	x, y := curve.ScalarBaseMult(big.NewInt(1).Bytes())

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}, nil
}

// calculateRecoveryID calculates the recovery ID for ECDSA signature
func calculateRecoveryID(hash []byte, r, s *big.Int, pubKey *ecdsa.PublicKey) int {
	// Simplified recovery ID calculation
	// In production, this should properly test both recovery IDs
	return 0
}

// pubKeyToAddress converts a public key to an Ethereum address
func pubKeyToAddress(pubKey *ecdsa.PublicKey) Address {
	// Ethereum address is the last 20 bytes of Keccak256(public_key)
	hasher := sha3.NewLegacyKeccak256()

	// Serialize public key (uncompressed format, skip the 0x04 prefix)
	pubKeyBytes := append(pubKey.X.Bytes(), pubKey.Y.Bytes()...)
	hasher.Write(pubKeyBytes)

	hash := hasher.Sum(nil)

	var addr Address
	copy(addr[:], hash[12:]) // Last 20 bytes
	return addr
}

// HomesteadSigner implements the homestead signing algorithm
type HomesteadSigner struct{}

// NewHomesteadSigner creates a new homestead signer (pre-EIP-155)
func NewHomesteadSigner() *HomesteadSigner {
	return &HomesteadSigner{}
}

// SignTx signs the transaction using homestead algorithm
func (s *HomesteadSigner) SignTx(tx *Transaction, privateKey *ecdsa.PrivateKey) (*Transaction, error) {
	// Simplified homestead signing (without chain ID)
	hash := s.hash(tx)

	signature, err := crypto_sign(hash[:], privateKey)
	if err != nil {
		return nil, err
	}

	txCopy := *tx
	txCopy.R = new(big.Int).SetBytes(signature[:32])
	txCopy.S = new(big.Int).SetBytes(signature[32:64])
	txCopy.V = new(big.Int).SetInt64(int64(signature[64]) + 27) // Homestead: v = recovery_id + 27

	return &txCopy, nil
}

// Sender recovers the sender address (homestead)
func (s *HomesteadSigner) Sender(tx *Transaction) (*Address, error) {
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return nil, fmt.Errorf("transaction not signed")
	}

	v := new(big.Int).Set(tx.V)
	v.Sub(v, big.NewInt(27)) // recovery_id = v - 27

	hash := s.hash(tx)
	pubKey, err := crypto_recover(hash[:], tx.R, tx.S, v.Int64())
	if err != nil {
		return nil, err
	}

	addr := pubKeyToAddress(pubKey)
	return &addr, nil
}

// hash creates the hash for homestead signing
func (s *HomesteadSigner) hash(tx *Transaction) Hash {
	hasher := sha3.NewLegacyKeccak256()

	// Homestead doesn't include chain ID
	data := fmt.Sprintf(`{"nonce":%d,"gasPrice":"%s","gasLimit":%d,"to":"%v","value":"%s","data":"%x"}`,
		tx.Nonce,
		tx.GasPrice.String(),
		tx.GasLimit,
		tx.To,
		tx.Value.String(),
		tx.Data,
	)

	hasher.Write([]byte(data))

	var hash Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}
