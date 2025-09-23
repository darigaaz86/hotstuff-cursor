package evm

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/trie"
	"github.com/relab/hotstuff/txpool"
	"golang.org/x/crypto/sha3"
)

// State change interface for trie state
type trieStateChange interface {
	revert(*TrieStateDB)
}

type trieBalanceChange struct {
	account txpool.Address
	prev    *big.Int
}

func (ch trieBalanceChange) revert(s *TrieStateDB) {
	account := s.GetAccount(ch.account)
	account.Balance = ch.prev
	s.SetAccount(ch.account, account)
}

type trieNonceChange struct {
	account txpool.Address
	prev    uint64
}

func (ch trieNonceChange) revert(s *TrieStateDB) {
	account := s.GetAccount(ch.account)
	account.Nonce = ch.prev
	s.SetAccount(ch.account, account)
}

type trieCodeChange struct {
	account  txpool.Address
	prevCode []byte
}

func (ch trieCodeChange) revert(s *TrieStateDB) {
	s.SetCode(ch.account, ch.prevCode)
}

type trieStorageChange struct {
	account  txpool.Address
	key      hotstuff.Hash
	prevalue hotstuff.Hash
}

func (ch trieStorageChange) revert(s *TrieStateDB) {
	s.SetState(ch.account, ch.key, ch.prevalue)
}

// TrieStateDB implements StateDB using Merkle Patricia Trie
type TrieStateDB struct {
	// World state trie (accounts)
	stateTrie *trie.MerklePatriciaTrie

	// Storage tries for contracts (per account)
	storageTries map[txpool.Address]*trie.MerklePatriciaTrie

	// Database for persistence
	db trie.Database

	// Journal for rollback support
	journal []trieStateChange

	// Snapshots for nested transactions
	snapshots []int

	logger logging.Logger
}

// AccountRLP represents the RLP-encoded account data stored in the trie
type AccountRLP struct {
	Nonce       uint64        `json:"nonce"`
	Balance     *big.Int      `json:"balance"`
	StorageRoot hotstuff.Hash `json:"storageRoot"`
	CodeHash    hotstuff.Hash `json:"codeHash"`
}

// NewTrieStateDB creates a new trie-based state database
func NewTrieStateDB(db trie.Database) *TrieStateDB {
	return &TrieStateDB{
		stateTrie:    trie.NewMerklePatriciaTrie(),
		storageTries: make(map[txpool.Address]*trie.MerklePatriciaTrie),
		db:           db,
		journal:      make([]trieStateChange, 0),
		snapshots:    make([]int, 0),
		logger:       logging.New("trie-state"),
	}
}

// NewTrieStateDBWithRoot creates a state database with an existing root
func NewTrieStateDBWithRoot(db trie.Database, stateRoot hotstuff.Hash) (*TrieStateDB, error) {
	// Load the state trie from database
	rootNode, err := db.Get(stateRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to load state root: %w", err)
	}

	stateTrie := trie.NewMerklePatriciaTrieWithRoot(rootNode)

	return &TrieStateDB{
		stateTrie:    stateTrie,
		storageTries: make(map[txpool.Address]*trie.MerklePatriciaTrie),
		db:           db,
		journal:      make([]trieStateChange, 0),
		snapshots:    make([]int, 0),
		logger:       logging.New("trie-state"),
	}, nil
}

// GetAccount retrieves an account from the state trie
func (s *TrieStateDB) GetAccount(addr txpool.Address) *AccountState {
	accountData, found := s.stateTrie.Get(addr[:])
	if !found {
		// Return empty account
		return &AccountState{
			Balance: big.NewInt(0),
			Nonce:   0,
		}
	}

	// Decode account data
	var accountRLP AccountRLP
	if err := json.Unmarshal(accountData, &accountRLP); err != nil {
		s.logger.Errorf("Failed to decode account data: %v", err)
		return &AccountState{
			Balance: big.NewInt(0),
			Nonce:   0,
		}
	}

	return &AccountState{
		Balance:     accountRLP.Balance,
		Nonce:       accountRLP.Nonce,
		CodeHash:    accountRLP.CodeHash,
		StorageRoot: accountRLP.StorageRoot,
	}
}

// SetAccount stores an account in the state trie
func (s *TrieStateDB) SetAccount(addr txpool.Address, account *AccountState) {
	accountRLP := AccountRLP{
		Nonce:       account.Nonce,
		Balance:     account.Balance,
		StorageRoot: account.StorageRoot,
		CodeHash:    account.CodeHash,
	}

	accountData, err := json.Marshal(accountRLP)
	if err != nil {
		s.logger.Errorf("Failed to encode account data: %v", err)
		return
	}

	if err := s.stateTrie.Put(addr[:], accountData); err != nil {
		s.logger.Errorf("Failed to store account: %v", err)
	}
}

// DeleteAccount removes an account from the state trie
func (s *TrieStateDB) DeleteAccount(addr txpool.Address) {
	if err := s.stateTrie.Delete(addr[:]); err != nil {
		s.logger.Errorf("Failed to delete account: %v", err)
	}

	// Also remove storage trie
	delete(s.storageTries, addr)
}

// GetBalance returns the balance of an account
func (s *TrieStateDB) GetBalance(addr txpool.Address) *big.Int {
	account := s.GetAccount(addr)
	return new(big.Int).Set(account.Balance)
}

// SetBalance sets the balance of an account
func (s *TrieStateDB) SetBalance(addr txpool.Address, balance *big.Int) {
	account := s.GetAccount(addr)

	// Record change for journal
	s.journal = append(s.journal, trieBalanceChange{addr, account.Balance})

	account.Balance = new(big.Int).Set(balance)
	s.SetAccount(addr, account)
}

// AddBalance adds to the balance of an account
func (s *TrieStateDB) AddBalance(addr txpool.Address, amount *big.Int) {
	account := s.GetAccount(addr)

	// Record change for journal
	s.journal = append(s.journal, trieBalanceChange{addr, account.Balance})

	account.Balance = new(big.Int).Add(account.Balance, amount)
	s.SetAccount(addr, account)
}

// SubBalance subtracts from the balance of an account
func (s *TrieStateDB) SubBalance(addr txpool.Address, amount *big.Int) {
	account := s.GetAccount(addr)

	// Record change for journal
	s.journal = append(s.journal, trieBalanceChange{addr, account.Balance})

	account.Balance = new(big.Int).Sub(account.Balance, amount)
	s.SetAccount(addr, account)
}

// GetNonce returns the nonce of an account
func (s *TrieStateDB) GetNonce(addr txpool.Address) uint64 {
	account := s.GetAccount(addr)
	return account.Nonce
}

// SetNonce sets the nonce of an account
func (s *TrieStateDB) SetNonce(addr txpool.Address, nonce uint64) {
	account := s.GetAccount(addr)

	// Record change for journal
	s.journal = append(s.journal, trieNonceChange{addr, account.Nonce})

	account.Nonce = nonce
	s.SetAccount(addr, account)
}

// GetCode returns the code of an account
func (s *TrieStateDB) GetCode(addr txpool.Address) []byte {
	account := s.GetAccount(addr)
	if account.CodeHash == (hotstuff.Hash{}) {
		return []byte{}
	}

	// Load code from database
	codeKey := append([]byte("code:"), account.CodeHash[:]...)
	codeData, found := s.stateTrie.Get(codeKey)
	if !found {
		return []byte{}
	}

	return codeData
}

// SetCode sets the code of an account
func (s *TrieStateDB) SetCode(addr txpool.Address, code []byte) {
	account := s.GetAccount(addr)

	// Record change for journal
	oldCode := s.GetCode(addr)
	s.journal = append(s.journal, trieCodeChange{addr, oldCode})

	if len(code) == 0 {
		account.CodeHash = hotstuff.Hash{}
	} else {
		// Calculate code hash
		hasher := sha3.NewLegacyKeccak256()
		hasher.Write(code)
		copy(account.CodeHash[:], hasher.Sum(nil))

		// Store code in trie
		codeKey := append([]byte("code:"), account.CodeHash[:]...)
		if err := s.stateTrie.Put(codeKey, code); err != nil {
			s.logger.Errorf("Failed to store code: %v", err)
		}
	}

	s.SetAccount(addr, account)
}

// GetCodeHash returns the code hash of an account
func (s *TrieStateDB) GetCodeHash(addr txpool.Address) hotstuff.Hash {
	account := s.GetAccount(addr)
	return account.CodeHash
}

// GetCodeSize returns the size of the code
func (s *TrieStateDB) GetCodeSize(addr txpool.Address) int {
	return len(s.GetCode(addr))
}

// GetState retrieves a storage value
func (s *TrieStateDB) GetState(addr txpool.Address, key hotstuff.Hash) hotstuff.Hash {
	storageTrie := s.getStorageTrie(addr, false)
	if storageTrie == nil {
		return hotstuff.Hash{}
	}

	value, found := storageTrie.Get(key[:])
	if !found {
		return hotstuff.Hash{}
	}

	var result hotstuff.Hash
	copy(result[:], value)
	return result
}

// SetState sets a storage value
func (s *TrieStateDB) SetState(addr txpool.Address, key, value hotstuff.Hash) {
	storageTrie := s.getStorageTrie(addr, true)
	if storageTrie == nil {
		return
	}

	// Record change for journal
	prev := s.GetState(addr, key)
	s.journal = append(s.journal, trieStorageChange{addr, key, prev})

	if value == (hotstuff.Hash{}) {
		// Delete the key
		storageTrie.Delete(key[:])
	} else {
		// Set the value
		storageTrie.Put(key[:], value[:])
	}

	// Update account's storage root
	account := s.GetAccount(addr)
	account.StorageRoot = storageTrie.Root()
	s.SetAccount(addr, account)
}

// getStorageTrie gets or creates a storage trie for an account
func (s *TrieStateDB) getStorageTrie(addr txpool.Address, create bool) *trie.MerklePatriciaTrie {
	if storageTrie, exists := s.storageTries[addr]; exists {
		return storageTrie
	}

	account := s.GetAccount(addr)
	if account.StorageRoot == (hotstuff.Hash{}) {
		if !create {
			return nil
		}
		// Create new storage trie
		storageTrie := trie.NewMerklePatriciaTrie()
		s.storageTries[addr] = storageTrie
		return storageTrie
	}

	// Load existing storage trie
	rootNode, err := s.db.Get(account.StorageRoot)
	if err != nil {
		s.logger.Errorf("Failed to load storage trie: %v", err)
		if !create {
			return nil
		}
		// Create new if load fails
		storageTrie := trie.NewMerklePatriciaTrie()
		s.storageTries[addr] = storageTrie
		return storageTrie
	}

	storageTrie := trie.NewMerklePatriciaTrieWithRoot(rootNode)
	s.storageTries[addr] = storageTrie
	return storageTrie
}

// CreateAccount creates a new account
func (s *TrieStateDB) CreateAccount(addr txpool.Address) {
	s.SetAccount(addr, &AccountState{
		Balance: big.NewInt(0),
		Nonce:   0,
	})
}

// Exist checks if an account exists
func (s *TrieStateDB) Exist(addr txpool.Address) bool {
	_, found := s.stateTrie.Get(addr[:])
	return found
}

// Empty checks if an account is empty
func (s *TrieStateDB) Empty(addr txpool.Address) bool {
	account := s.GetAccount(addr)
	return account.Nonce == 0 &&
		account.Balance.Sign() == 0 &&
		account.CodeHash == (hotstuff.Hash{})
}

// Snapshot creates a snapshot of the current state
func (s *TrieStateDB) Snapshot() int {
	snapshot := len(s.journal)
	s.snapshots = append(s.snapshots, snapshot)
	return len(s.snapshots) - 1
}

// RevertToSnapshot reverts the state to a previous snapshot
func (s *TrieStateDB) RevertToSnapshot(id int) {
	if id < 0 || id >= len(s.snapshots) {
		return
	}

	// Get the journal position for this snapshot
	journalPos := s.snapshots[id]

	// Revert changes
	for i := len(s.journal) - 1; i >= journalPos; i-- {
		s.journal[i].revert(s)
	}

	// Truncate journal and snapshots
	s.journal = s.journal[:journalPos]
	s.snapshots = s.snapshots[:id]
}

// Commit finalizes the state changes and returns the state root
func (s *TrieStateDB) Commit() (hotstuff.Hash, error) {
	// Commit all storage tries and update account storage roots
	for addr, storageTrie := range s.storageTries {
		storageRoot := storageTrie.Root()
		if storageRoot != (hotstuff.Hash{}) {
			// Store storage trie in database
			if err := s.commitTrie(storageTrie); err != nil {
				return hotstuff.Hash{}, fmt.Errorf("failed to commit storage trie: %w", err)
			}
		}

		// Update account storage root
		account := s.GetAccount(addr)
		account.StorageRoot = storageRoot
		s.SetAccount(addr, account)
	}

	// Commit state trie
	if err := s.commitTrie(s.stateTrie); err != nil {
		return hotstuff.Hash{}, fmt.Errorf("failed to commit state trie: %w", err)
	}

	// Clear journal
	s.journal = s.journal[:0]
	s.snapshots = s.snapshots[:0]

	return s.stateTrie.Root(), nil
}

// commitTrie commits a trie to the database
func (s *TrieStateDB) commitTrie(t *trie.MerklePatriciaTrie) error {
	// This is a simplified commit - in a full implementation,
	// we would traverse the trie and store all dirty nodes

	root := t.Root()
	if root == (hotstuff.Hash{}) {
		return nil
	}

	// For now, we'll use a simplified approach where the trie
	// manages its own persistence through the database interface

	return nil
}

// Copy creates a deep copy of the state database
func (s *TrieStateDB) Copy() StateDB {
	newState := &TrieStateDB{
		stateTrie:    s.stateTrie.Copy(),
		storageTries: make(map[txpool.Address]*trie.MerklePatriciaTrie),
		db:           s.db,
		journal:      make([]trieStateChange, 0),
		snapshots:    make([]int, 0),
		logger:       s.logger,
	}

	// Copy storage tries
	for addr, storageTrie := range s.storageTries {
		newState.storageTries[addr] = storageTrie.Copy()
	}

	return newState
}

// GetStateRoot returns the current state root
func (s *TrieStateDB) GetStateRoot() hotstuff.Hash {
	return s.stateTrie.Root()
}

// GetStorageProof generates a storage proof for a key
func (s *TrieStateDB) GetStorageProof(addr txpool.Address, key hotstuff.Hash) ([][]byte, error) {
	storageTrie := s.getStorageTrie(addr, false)
	if storageTrie == nil {
		return nil, fmt.Errorf("no storage trie for account")
	}

	return storageTrie.Prove(key[:])
}

// GetAccountProof generates an account proof
func (s *TrieStateDB) GetAccountProof(addr txpool.Address) ([][]byte, error) {
	return s.stateTrie.Prove(addr[:])
}

// Stats returns statistics about the state database
func (s *TrieStateDB) Stats() TrieStateStats {
	stateStats := s.stateTrie.Stats()

	stats := TrieStateStats{
		StateTrieStats: stateStats,
		StorageTries:   len(s.storageTries),
		JournalSize:    len(s.journal),
		SnapshotCount:  len(s.snapshots),
	}

	// Collect storage trie stats
	for _, storageTrie := range s.storageTries {
		storageStats := storageTrie.Stats()
		stats.TotalStorageNodes += storageStats.NodeCount
	}

	if s.db != nil {
		stats.DatabaseStats = s.db.Stats()
	}

	return stats
}

// TrieStateStats contains statistics about the trie state database
type TrieStateStats struct {
	StateTrieStats    trie.TrieStats
	StorageTries      int
	TotalStorageNodes int
	JournalSize       int
	SnapshotCount     int
	DatabaseStats     trie.DatabaseStats
}

// String returns a string representation of the stats
func (s TrieStateStats) String() string {
	return fmt.Sprintf("TrieState(nodes=%d, storage_tries=%d, storage_nodes=%d, journal=%d, snapshots=%d)",
		s.StateTrieStats.NodeCount,
		s.StorageTries,
		s.TotalStorageNodes,
		s.JournalSize,
		s.SnapshotCount)
}

// SetupGenesisAccounts creates initial accounts for testing
func (s *TrieStateDB) SetupGenesisAccounts() {
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

	s.logger.Infof("Created %d genesis accounts", len(testAccounts))
}
