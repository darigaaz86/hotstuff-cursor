package evm

import (
	"fmt"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/txpool"
)

// AccountState represents the state of an account in the EVM
type AccountState struct {
	Balance  *big.Int `json:"balance"`
	Nonce    uint64   `json:"nonce"`
	CodeHash hotstuff.Hash `json:"codeHash,omitempty"`
	StorageRoot hotstuff.Hash `json:"storageRoot,omitempty"`
}

// StateDB represents the EVM state database interface
type StateDB interface {
	// Account operations
	GetAccount(addr txpool.Address) *AccountState
	SetAccount(addr txpool.Address, account *AccountState)
	DeleteAccount(addr txpool.Address)
	
	// Balance operations
	GetBalance(addr txpool.Address) *big.Int
	SetBalance(addr txpool.Address, balance *big.Int)
	AddBalance(addr txpool.Address, amount *big.Int)
	SubBalance(addr txpool.Address, amount *big.Int)
	
	// Nonce operations
	GetNonce(addr txpool.Address) uint64
	SetNonce(addr txpool.Address, nonce uint64)
	
	// Code operations
	GetCode(addr txpool.Address) []byte
	SetCode(addr txpool.Address, code []byte)
	GetCodeHash(addr txpool.Address) hotstuff.Hash
	GetCodeSize(addr txpool.Address) int
	
	// Storage operations
	GetState(addr txpool.Address, key hotstuff.Hash) hotstuff.Hash
	SetState(addr txpool.Address, key, value hotstuff.Hash)
	
	// State management
	CreateAccount(addr txpool.Address)
	Exist(addr txpool.Address) bool
	Empty(addr txpool.Address) bool
	
	// Snapshot and revert
	Snapshot() int
	RevertToSnapshot(id int)
	
	// Commit changes and get state root
	Commit() (hotstuff.Hash, error)
	
	// Copy creates a deep copy of the state
	Copy() StateDB
}

// InMemoryStateDB provides a simple in-memory implementation of StateDB
type InMemoryStateDB struct {
	accounts map[txpool.Address]*AccountState
	storage  map[txpool.Address]map[hotstuff.Hash]hotstuff.Hash
	code     map[txpool.Address][]byte
	
	// Snapshot management
	snapshots []map[txpool.Address]*AccountState
	journal   []stateChange
}

type stateChange interface {
	revert(*InMemoryStateDB)
}

type balanceChange struct {
	account txpool.Address
	prev    *big.Int
}

func (ch balanceChange) revert(s *InMemoryStateDB) {
	s.GetAccount(ch.account).Balance = ch.prev
}

type nonceChange struct {
	account txpool.Address
	prev    uint64
}

func (ch nonceChange) revert(s *InMemoryStateDB) {
	s.GetAccount(ch.account).Nonce = ch.prev
}

type codeChange struct {
	account txpool.Address
	prevCode []byte
}

func (ch codeChange) revert(s *InMemoryStateDB) {
	s.code[ch.account] = ch.prevCode
}

type storageChange struct {
	account txpool.Address
	key     hotstuff.Hash
	prevalue hotstuff.Hash
}

func (ch storageChange) revert(s *InMemoryStateDB) {
	s.SetState(ch.account, ch.key, ch.prevalue)
}

// NewInMemoryStateDB creates a new in-memory state database
func NewInMemoryStateDB() *InMemoryStateDB {
	return &InMemoryStateDB{
		accounts:  make(map[txpool.Address]*AccountState),
		storage:   make(map[txpool.Address]map[hotstuff.Hash]hotstuff.Hash),
		code:      make(map[txpool.Address][]byte),
		snapshots: make([]map[txpool.Address]*AccountState, 0),
		journal:   make([]stateChange, 0),
	}
}

// GetAccount retrieves an account state
func (s *InMemoryStateDB) GetAccount(addr txpool.Address) *AccountState {
	if account, exists := s.accounts[addr]; exists {
		// Return a copy to prevent external mutations
		return &AccountState{
			Balance:     new(big.Int).Set(account.Balance),
			Nonce:       account.Nonce,
			CodeHash:    account.CodeHash,
			StorageRoot: account.StorageRoot,
		}
	}
	// Return empty account if not found
	return &AccountState{
		Balance: big.NewInt(0),
		Nonce:   0,
	}
}

// SetAccount sets an account state
func (s *InMemoryStateDB) SetAccount(addr txpool.Address, account *AccountState) {
	s.accounts[addr] = &AccountState{
		Balance:     new(big.Int).Set(account.Balance),
		Nonce:       account.Nonce,
		CodeHash:    account.CodeHash,
		StorageRoot: account.StorageRoot,
	}
}

// DeleteAccount removes an account
func (s *InMemoryStateDB) DeleteAccount(addr txpool.Address) {
	delete(s.accounts, addr)
	delete(s.storage, addr)
	delete(s.code, addr)
}

// GetBalance returns the balance of an account
func (s *InMemoryStateDB) GetBalance(addr txpool.Address) *big.Int {
	return s.GetAccount(addr).Balance
}

// SetBalance sets the balance of an account
func (s *InMemoryStateDB) SetBalance(addr txpool.Address, balance *big.Int) {
	account := s.GetAccount(addr)
	s.journal = append(s.journal, balanceChange{addr, account.Balance})
	account.Balance = new(big.Int).Set(balance)
	s.SetAccount(addr, account)
}

// AddBalance adds to the balance of an account
func (s *InMemoryStateDB) AddBalance(addr txpool.Address, amount *big.Int) {
	account := s.GetAccount(addr)
	s.journal = append(s.journal, balanceChange{addr, account.Balance})
	account.Balance = new(big.Int).Add(account.Balance, amount)
	s.SetAccount(addr, account)
}

// SubBalance subtracts from the balance of an account
func (s *InMemoryStateDB) SubBalance(addr txpool.Address, amount *big.Int) {
	account := s.GetAccount(addr)
	s.journal = append(s.journal, balanceChange{addr, account.Balance})
	account.Balance = new(big.Int).Sub(account.Balance, amount)
	s.SetAccount(addr, account)
}

// GetNonce returns the nonce of an account
func (s *InMemoryStateDB) GetNonce(addr txpool.Address) uint64 {
	return s.GetAccount(addr).Nonce
}

// SetNonce sets the nonce of an account
func (s *InMemoryStateDB) SetNonce(addr txpool.Address, nonce uint64) {
	account := s.GetAccount(addr)
	s.journal = append(s.journal, nonceChange{addr, account.Nonce})
	account.Nonce = nonce
	s.SetAccount(addr, account)
}

// GetCode returns the code of an account
func (s *InMemoryStateDB) GetCode(addr txpool.Address) []byte {
	if code, exists := s.code[addr]; exists {
		return code
	}
	return []byte{}
}

// SetCode sets the code of an account
func (s *InMemoryStateDB) SetCode(addr txpool.Address, code []byte) {
	s.journal = append(s.journal, codeChange{addr, s.code[addr]})
	s.code[addr] = make([]byte, len(code))
	copy(s.code[addr], code)
}

// GetCodeHash returns the code hash of an account
func (s *InMemoryStateDB) GetCodeHash(addr txpool.Address) hotstuff.Hash {
	return s.GetAccount(addr).CodeHash
}

// GetCodeSize returns the size of the code
func (s *InMemoryStateDB) GetCodeSize(addr txpool.Address) int {
	return len(s.GetCode(addr))
}

// GetState retrieves a storage value
func (s *InMemoryStateDB) GetState(addr txpool.Address, key hotstuff.Hash) hotstuff.Hash {
	if storage, exists := s.storage[addr]; exists {
		if value, exists := storage[key]; exists {
			return value
		}
	}
	return hotstuff.Hash{} // Empty hash for non-existent values
}

// SetState sets a storage value
func (s *InMemoryStateDB) SetState(addr txpool.Address, key, value hotstuff.Hash) {
	if s.storage[addr] == nil {
		s.storage[addr] = make(map[hotstuff.Hash]hotstuff.Hash)
	}
	
	prev := s.GetState(addr, key)
	s.journal = append(s.journal, storageChange{addr, key, prev})
	s.storage[addr][key] = value
}

// CreateAccount creates a new account
func (s *InMemoryStateDB) CreateAccount(addr txpool.Address) {
	s.SetAccount(addr, &AccountState{
		Balance: big.NewInt(0),
		Nonce:   0,
	})
}

// Exist checks if an account exists
func (s *InMemoryStateDB) Exist(addr txpool.Address) bool {
	_, exists := s.accounts[addr]
	return exists
}

// Empty checks if an account is empty
func (s *InMemoryStateDB) Empty(addr txpool.Address) bool {
	account := s.GetAccount(addr)
	return account.Nonce == 0 && account.Balance.Sign() == 0 && len(s.GetCode(addr)) == 0
}

// Snapshot creates a snapshot of the current state
func (s *InMemoryStateDB) Snapshot() int {
	// Create a deep copy of accounts
	snapshot := make(map[txpool.Address]*AccountState)
	for addr, account := range s.accounts {
		snapshot[addr] = &AccountState{
			Balance:     new(big.Int).Set(account.Balance),
			Nonce:       account.Nonce,
			CodeHash:    account.CodeHash,
			StorageRoot: account.StorageRoot,
		}
	}
	
	s.snapshots = append(s.snapshots, snapshot)
	return len(s.snapshots) - 1
}

// RevertToSnapshot reverts the state to a previous snapshot
func (s *InMemoryStateDB) RevertToSnapshot(id int) {
	if id < 0 || id >= len(s.snapshots) {
		return
	}
	
	// Restore accounts from snapshot
	s.accounts = s.snapshots[id]
	
	// Remove snapshots after the reverted one
	s.snapshots = s.snapshots[:id]
	
	// Clear journal
	s.journal = s.journal[:0]
}

// Commit finalizes the state changes and returns the state root
func (s *InMemoryStateDB) Commit() (hotstuff.Hash, error) {
	// Clear journal since we're committing
	s.journal = s.journal[:0]
	
	// Calculate state root (simplified - in production this would be a Merkle Patricia Trie root)
	return s.calculateStateRoot(), nil
}

// calculateStateRoot computes a simplified state root
func (s *InMemoryStateDB) calculateStateRoot() hotstuff.Hash {
	// This is a simplified state root calculation
	// In production, this should use Merkle Patricia Trie
	hasher := hotstuff.Hash{}
	
	for addr, account := range s.accounts {
		// Mix address and account data into the hash
		for i := 0; i < 20 && i < 32; i++ {
			hasher[i] ^= addr[i]
		}
		if account.Balance != nil {
			balanceBytes := account.Balance.Bytes()
			for i := 0; i < len(balanceBytes) && i < 32; i++ {
				hasher[i] ^= balanceBytes[i]
			}
		}
		hasher[0] ^= byte(account.Nonce)
	}
	
	return hasher
}

// Copy creates a deep copy of the state database
func (s *InMemoryStateDB) Copy() StateDB {
	copy := NewInMemoryStateDB()
	
	// Copy accounts
	for addr, account := range s.accounts {
		copy.accounts[addr] = &AccountState{
			Balance:     new(big.Int).Set(account.Balance),
			Nonce:       account.Nonce,
			CodeHash:    account.CodeHash,
			StorageRoot: account.StorageRoot,
		}
	}
	
	// Copy storage
	for addr, storage := range s.storage {
		copy.storage[addr] = make(map[hotstuff.Hash]hotstuff.Hash)
		for key, value := range storage {
			copy.storage[addr][key] = value
		}
	}
	
	// Copy code
	for addr, code := range s.code {
		copy.code[addr] = make([]byte, len(code))
		copy.code[addr] = append(copy.code[addr], code...)
	}
	
	return copy
}

// Genesis account setup for testing
func (s *InMemoryStateDB) SetupGenesisAccounts() {
	// Create some test accounts with initial balances
	testAccounts := map[string]*big.Int{
		"0x1000000000000000000000000000000000000001": big.NewInt(1000000000000000000), // 1 ETH
		"0x1000000000000000000000000000000000000002": big.NewInt(2000000000000000000), // 2 ETH
		"0x1000000000000000000000000000000000000003": big.NewInt(5000000000000000000), // 5 ETH
	}
	
	for addrStr, balance := range testAccounts {
		var addr txpool.Address
		fmt.Sscanf(addrStr, "0x%40x", &addr)
		s.CreateAccount(addr)
		s.SetBalance(addr, balance)
	}
}
