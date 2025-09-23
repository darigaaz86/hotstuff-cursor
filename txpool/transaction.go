package txpool

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"

	"golang.org/x/crypto/sha3"
)

// Transaction represents an Ethereum-style transaction
type Transaction struct {
	// Core transaction fields
	Nonce    uint64   `json:"nonce"`
	GasPrice *big.Int `json:"gasPrice"`
	GasLimit uint64   `json:"gasLimit"`
	To       *Address `json:"to"` // nil for contract creation
	Value    *big.Int `json:"value"`
	Data     []byte   `json:"data"`

	// EIP-155: Simple replay attack protection
	ChainID *big.Int `json:"chainId"`

	// Signature fields
	V *big.Int `json:"v"`
	R *big.Int `json:"r"`
	S *big.Int `json:"s"`

	// Cached values (not serialized)
	hash Hash     `json:"-"`
	from *Address `json:"-"`
	size uint64   `json:"-"`
}

// Address represents a 20-byte Ethereum address
type Address [20]byte

// Hash represents a 32-byte hash
type Hash [32]byte

// String returns the hex representation of the address
func (addr Address) String() string {
	return fmt.Sprintf("0x%x", addr[:])
}

// String returns the hex representation of the hash
func (h Hash) String() string {
	return fmt.Sprintf("0x%x", h[:])
}

// NewTransaction creates a new transaction
func NewTransaction(nonce uint64, to *Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	return &Transaction{
		Nonce:    nonce,
		To:       to,
		Value:    amount,
		GasLimit: gasLimit,
		GasPrice: gasPrice,
		Data:     data,
		ChainID:  big.NewInt(1), // Default to mainnet chain ID
	}
}

// Hash calculates and returns the transaction hash
func (tx *Transaction) Hash() Hash {
	if tx.hash == (Hash{}) {
		tx.hash = tx.calculateHash()
	}
	return tx.hash
}

// calculateHash computes the Keccak256 hash of the transaction
func (tx *Transaction) calculateHash() Hash {
	hasher := sha3.NewLegacyKeccak256()

	// RLP encoding simulation (simplified)
	data := tx.encodeForHashing()
	hasher.Write(data)

	var hash Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

// encodeForHashing creates a byte representation for hashing
// This is a simplified version - in production, use RLP encoding
func (tx *Transaction) encodeForHashing() []byte {
	data, _ := json.Marshal(struct {
		Nonce    uint64   `json:"nonce"`
		GasPrice *big.Int `json:"gasPrice"`
		GasLimit uint64   `json:"gasLimit"`
		To       *Address `json:"to"`
		Value    *big.Int `json:"value"`
		Data     []byte   `json:"data"`
		ChainID  *big.Int `json:"chainId"`
	}{
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		GasLimit: tx.GasLimit,
		To:       tx.To,
		Value:    tx.Value,
		Data:     tx.Data,
		ChainID:  tx.ChainID,
	})
	return data
}

// Sign signs the transaction with the given private key
func (tx *Transaction) Sign(privateKey *ecdsa.PrivateKey) error {
	// EIP-155 signing
	signer := NewEIP155Signer(tx.ChainID)
	signedTx, err := signer.SignTx(tx, privateKey)
	if err != nil {
		return err
	}

	tx.V = signedTx.V
	tx.R = signedTx.R
	tx.S = signedTx.S

	// Clear cached values
	tx.hash = Hash{}
	tx.from = nil

	return nil
}

// From returns the sender address of the transaction
func (tx *Transaction) From() (*Address, error) {
	if tx.from != nil {
		return tx.from, nil
	}

	// Recover the sender from the signature
	signer := NewEIP155Signer(tx.ChainID)
	from, err := signer.Sender(tx)
	if err != nil {
		return nil, err
	}

	tx.from = from
	return from, nil
}

// Cost returns the total cost of the transaction (value + gas * gasPrice)
func (tx *Transaction) Cost() *big.Int {
	total := new(big.Int).Set(tx.Value)
	gas := new(big.Int).SetUint64(tx.GasLimit)
	gas.Mul(gas, tx.GasPrice)
	total.Add(total, gas)
	return total
}

// Size returns the size of the transaction in bytes
func (tx *Transaction) Size() uint64 {
	if tx.size == 0 {
		data, _ := json.Marshal(tx)
		tx.size = uint64(len(data))
	}
	return tx.size
}

// IsContractCreation returns true if the transaction creates a contract
func (tx *Transaction) IsContractCreation() bool {
	return tx.To == nil
}

// ToCommand converts the transaction to a command
func (tx *Transaction) ToCommand() Command {
	return &TransactionCommand{tx: tx}
}

// TransactionCommand implements the Command interface
type TransactionCommand struct {
	tx *Transaction
}

func (tc *TransactionCommand) ID() string {
	return tc.tx.Hash().String()
}

// TransactionFromCommand creates a transaction from a command
func TransactionFromCommand(cmd Command) (*Transaction, error) {
	if tc, ok := cmd.(*TransactionCommand); ok {
		return tc.tx, nil
	}
	return nil, fmt.Errorf("invalid command type")
}

// Validate performs basic validation of the transaction
func (tx *Transaction) Validate() error {
	// Check for nil values
	if tx.GasPrice == nil {
		return fmt.Errorf("gas price cannot be nil")
	}
	if tx.Value == nil {
		return fmt.Errorf("value cannot be nil")
	}
	if tx.ChainID == nil {
		return fmt.Errorf("chain ID cannot be nil")
	}

	// Check for negative values
	if tx.GasPrice.Sign() < 0 {
		return fmt.Errorf("gas price cannot be negative")
	}
	if tx.Value.Sign() < 0 {
		return fmt.Errorf("value cannot be negative")
	}

	// Check gas limit
	if tx.GasLimit == 0 {
		return fmt.Errorf("gas limit cannot be zero")
	}

	// Note: Signature verification is simplified for now
	// In production, proper ECDSA signature verification would be implemented

	return nil
}
