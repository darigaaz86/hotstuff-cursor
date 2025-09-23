package txpool

import (
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
)

// Config holds transaction pool configuration
type Config struct {
	// Pool limits
	GlobalSlots uint64 // Maximum number of executable transaction slots for all accounts
	GlobalQueue uint64 // Maximum number of non-executable transaction slots for all accounts
	
	// Per-account limits
	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	AccountQueue uint64 // Maximum number of non-executable transaction slots per account
	
	// Pricing and gas
	PriceLimit uint64        // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64        // Minimum price bump percentage to replace an already existing transaction (nonce)
	Lifetime   time.Duration // Maximum amount of time non-executable transactions are queued
	
	// Journal
	Journal string // Journal of local transactions to survive node restarts
	NoLocals bool  // Whether local transaction handling should be disabled
}

// DefaultConfig returns the default configuration for transaction pool
func DefaultConfig() Config {
	return Config{
		GlobalSlots:  4096,
		GlobalQueue:  1024,
		AccountSlots: 16,
		AccountQueue: 64,
		PriceLimit:   1,
		PriceBump:    10,
		Lifetime:     3 * time.Hour,
		Journal:      "transactions.rlp",
		NoLocals:     false,
	}
}

// TxPool manages pending and queued transactions
type TxPool struct {
	config Config
	signer Signer
	logger logging.Logger

	mu sync.RWMutex

	// Transaction storage
	pending map[Address]*txList // All currently processable transactions
	queue   map[Address]*txList // Queued but non-processable transactions
	all     *txLookup           // All transactions to allow lookups
	priced  *txPricedList       // All transactions sorted by price

	// Statistics
	stats struct {
		pending int // Number of pending transactions
		queued  int // Number of queued transactions
	}

	// Event subscriptions for new transactions
	subscribers []chan<- *Transaction
	
	// Quit channel
	quit chan struct{}
}

// NewTxPool creates a new transaction pool
func NewTxPool(config Config, signer Signer) *TxPool {
	pool := &TxPool{
		config:      config,
		signer:      signer,
		logger:      logging.New("txpool"),
		pending:     make(map[Address]*txList),
		queue:       make(map[Address]*txList),
		all:         newTxLookup(),
		priced:      newTxPricedList(&config),
		subscribers: make([]chan<- *Transaction, 0),
		quit:        make(chan struct{}),
	}

	// Start the cleanup goroutine
	go pool.loop()

	return pool
}

// AddLocal adds a local transaction to the pool
func (pool *TxPool) AddLocal(tx *Transaction) error {
	return pool.add(tx, true)
}

// AddRemote adds a remote transaction to the pool
func (pool *TxPool) AddRemote(tx *Transaction) error {
	return pool.add(tx, false)
}

// add adds a transaction to the pool
func (pool *TxPool) add(tx *Transaction, local bool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Validate transaction
	if err := pool.validateTx(tx, local); err != nil {
		return err
	}

	// Get sender address
	from, err := pool.signer.Sender(tx)
	if err != nil {
		return err
	}

	// If the transaction pool is full, reject the transaction
	if uint64(pool.all.Count()) >= pool.config.GlobalSlots+pool.config.GlobalQueue {
		// If this is a replacement transaction, allow it
		if old := pool.all.Get(tx.Hash()); old != nil {
			return pool.replace(old, tx, local)
		}
		return fmt.Errorf("transaction pool is full")
	}

	// Try to replace an existing transaction with the same nonce
	if list := pool.pending[*from]; list != nil && list.Overlaps(tx) {
		inserted, old := list.Add(tx, pool.config.PriceBump)
		if !inserted {
			return fmt.Errorf("replacement transaction underpriced")
		}
		if old != nil {
			pool.all.Remove(old.Hash())
			pool.priced.Removed(1)
		}
		pool.all.Add(tx)
		pool.priced.Put(tx)
		
		pool.logger.Infof("Replaced transaction hash=%s nonce=%d", tx.Hash().String(), tx.Nonce)
		pool.notifySubscribers(tx)
		return nil
	}

	// New transaction for the pending list, create account list if first transaction
	if pool.pending[*from] == nil {
		pool.pending[*from] = newTxList(true)
	}
	list := pool.pending[*from]

	inserted, old := list.Add(tx, pool.config.PriceBump)
	if !inserted {
		return fmt.Errorf("transaction underpriced")
	}
	if old != nil {
		pool.all.Remove(old.Hash())
		pool.priced.Removed(1)
	}

	pool.all.Add(tx)
	pool.priced.Put(tx)
	pool.stats.pending++

	pool.logger.Infof("Added transaction hash=%s nonce=%d from=%s", tx.Hash().String(), tx.Nonce, from.String())
	pool.notifySubscribers(tx)

	return nil
}

// validateTx validates a transaction
func (pool *TxPool) validateTx(tx *Transaction, local bool) error {
	// Basic validation
	if err := tx.Validate(); err != nil {
		return err
	}

	// Check gas price
	if tx.GasPrice.Cmp(big.NewInt(int64(pool.config.PriceLimit))) < 0 {
		return fmt.Errorf("gas price too low: %s", tx.GasPrice.String())
	}

	// Check transaction size
	if tx.Size() > 32*1024 {
		return fmt.Errorf("transaction size too large: %d bytes", tx.Size())
	}

	return nil
}

// replace replaces an existing transaction
func (pool *TxPool) replace(oldTx, newTx *Transaction, local bool) error {
	// Price bump check
	if newTx.GasPrice.Cmp(priceBump(oldTx.GasPrice, pool.config.PriceBump)) < 0 {
		return fmt.Errorf("replacement transaction underpriced")
	}

	// Remove old transaction
	pool.all.Remove(oldTx.Hash())
	pool.priced.Removed(1)

	// Add new transaction
	pool.all.Add(newTx)
	pool.priced.Put(newTx)

	pool.logger.Infof("Replaced transaction old=%s new=%s", oldTx.Hash().String(), newTx.Hash().String())
	pool.notifySubscribers(newTx)

	return nil
}

// Get retrieves a transaction by hash
func (pool *TxPool) Get(hash Hash) *Transaction {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.all.Get(hash)
}

// Pending returns all currently processable transactions
func (pool *TxPool) Pending() map[Address][]*Transaction {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	pending := make(map[Address][]*Transaction)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	return pending
}

// NextNonce returns the next nonce for the given address
func (pool *TxPool) NextNonce(addr Address) uint64 {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	if list := pool.pending[addr]; list != nil {
		return list.LastNonce() + 1
	}
	
	// If no pending transactions, return 0 (will need state integration later)
	return 0
}

// Subscribe subscribes to new transaction events
func (pool *TxPool) Subscribe() <-chan *Transaction {
	ch := make(chan *Transaction, 64)
	pool.mu.Lock()
	pool.subscribers = append(pool.subscribers, ch)
	pool.mu.Unlock()
	return ch
}

// Stats returns transaction pool statistics
func (pool *TxPool) Stats() (pending int, queued int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.stats.pending, pool.stats.queued
}

// GetTransactionsForBlock returns transactions suitable for inclusion in a block
func (pool *TxPool) GetTransactionsForBlock(maxGas uint64) []*Transaction {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	var (
		transactions []*Transaction
		totalGas     uint64
	)

	// Collect transactions from all accounts, sorted by gas price
	var candidates []*Transaction
	for _, list := range pool.pending {
		candidates = append(candidates, list.Flatten()...)
	}

	// Sort by gas price (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].GasPrice.Cmp(candidates[j].GasPrice) > 0
	})

	// Select transactions up to gas limit
	for _, tx := range candidates {
		if totalGas+tx.GasLimit > maxGas {
			break
		}
		transactions = append(transactions, tx)
		totalGas += tx.GasLimit
	}

	return transactions
}

// ToCommands converts transactions to HotStuff commands
func (pool *TxPool) ToCommands(transactions []*Transaction) []hotstuff.Command {
	commands := make([]hotstuff.Command, len(transactions))
	for i, tx := range transactions {
		commands[i] = tx.ToCommand()
	}
	return commands
}

// notifySubscribers notifies all subscribers of a new transaction
func (pool *TxPool) notifySubscribers(tx *Transaction) {
	for _, ch := range pool.subscribers {
		select {
		case ch <- tx:
		default:
			// Channel is full, skip this subscriber
		}
	}
}

// loop runs the transaction pool cleanup
func (pool *TxPool) loop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pool.cleanup()
		case <-pool.quit:
			return
		}
	}
}

// cleanup removes old transactions
func (pool *TxPool) cleanup() {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Remove expired transactions from queue
	for addr, list := range pool.queue {
		// Remove transactions older than lifetime
		removed := list.RemoveOld(pool.config.Lifetime)
		for _, tx := range removed {
			pool.all.Remove(tx.Hash())
			pool.priced.Removed(1)
		}
		if list.Empty() {
			delete(pool.queue, addr)
		}
	}

	pool.logger.Infof("Transaction pool cleanup completed, pending: %d, queued: %d", 
		pool.stats.pending, pool.stats.queued)
}

// Close stops the transaction pool
func (pool *TxPool) Close() {
	close(pool.quit)
}

// priceBump calculates the required price bump
func priceBump(price *big.Int, bump uint64) *big.Int {
	percent := new(big.Int).SetUint64(bump)
	bump_amount := new(big.Int).Mul(price, percent)
	bump_amount.Div(bump_amount, big.NewInt(100))
	return new(big.Int).Add(price, bump_amount)
}
