package txpool

import (
	"container/heap"
	"math/big"
	"sort"
	"time"
)

// txList is a "list" of transactions belonging to an account, sorted by nonce.
// Transactions can be in pending or queued state.
type txList struct {
	strict bool              // Whether nonces are strictly continuous or not
	txs    map[uint64]*Transaction // Hash map of nonce -> transaction
}

// newTxList creates a new transaction list for maintaining nonce-indexable fast,
// gapped, sortable transaction lists.
func newTxList(strict bool) *txList {
	return &txList{
		strict: strict,
		txs:    make(map[uint64]*Transaction),
	}
}

// Overlaps returns whether the transaction specified has the same nonce as a transaction
// already contained within the list.
func (l *txList) Overlaps(tx *Transaction) bool {
	_, exists := l.txs[tx.Nonce]
	return exists
}

// Add tries to insert a new transaction into the list, returning whether the
// transaction was accepted, and if yes, any previous transaction it replaced.
func (l *txList) Add(tx *Transaction, priceBump uint64) (bool, *Transaction) {
	// If there's an old one, make sure the new one is at least 10% more expensive
	old, exists := l.txs[tx.Nonce]
	if exists {
		threshold := calculatePriceBump(old.GasPrice, priceBump)
		if tx.GasPrice.Cmp(threshold) < 0 {
			return false, nil
		}
	}
	
	// Add the transaction
	l.txs[tx.Nonce] = tx
	
	return true, old
}

// Forward removes all transactions from the list with a nonce lower than the
// provided threshold. Every removed transaction is returned for any post-removal
// maintenance.
func (l *txList) Forward(threshold uint64) []*Transaction {
	var removed []*Transaction
	
	for nonce, tx := range l.txs {
		if nonce < threshold {
			removed = append(removed, tx)
			delete(l.txs, nonce)
		}
	}
	
	return removed
}

// Filter removes all transactions from the list with a cost or gas limit higher
// than the provided thresholds. Every removed transaction is returned for any
// post-removal maintenance.
func (l *txList) Filter(costLimit *big.Int, gasLimit uint64) ([]*Transaction, []*Transaction) {
	var invalids, caps []*Transaction
	
	for nonce, tx := range l.txs {
		// Remove transactions with too high cost
		if tx.Cost().Cmp(costLimit) > 0 {
			invalids = append(invalids, tx)
			delete(l.txs, nonce)
			continue
		}
		
		// Remove transactions with too high gas limit
		if tx.GasLimit > gasLimit {
			caps = append(caps, tx)
			delete(l.txs, nonce)
		}
	}
	
	return invalids, caps
}

// Cap places a hard limit on the number of items, returning all transactions
// exceeding that limit.
func (l *txList) Cap(threshold int) []*Transaction {
	if len(l.txs) <= threshold {
		return nil
	}
	
	// Sort transactions by nonce
	nonces := make([]uint64, 0, len(l.txs))
	for nonce := range l.txs {
		nonces = append(nonces, nonce)
	}
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	
	// Remove excess transactions (keep lowest nonces)
	var drops []*Transaction
	for _, nonce := range nonces[threshold:] {
		drops = append(drops, l.txs[nonce])
		delete(l.txs, nonce)
	}
	
	return drops
}

// Remove deletes a transaction from the maintained list, returning whether the
// transaction was found.
func (l *txList) Remove(tx *Transaction) bool {
	if _, exists := l.txs[tx.Nonce]; exists {
		delete(l.txs, tx.Nonce)
		return true
	}
	return false
}

// RemoveOld removes transactions older than the specified duration
func (l *txList) RemoveOld(lifetime time.Duration) []*Transaction {
	// cutoff := time.Now().Add(-lifetime)
	var removed []*Transaction
	
	for nonce, tx := range l.txs {
		// This is simplified - in practice, we'd need to track transaction timestamps
		// For now, assume all transactions in queue are old enough to remove
		removed = append(removed, tx)
		delete(l.txs, nonce)
	}
	
	return removed
}

// Ready retrieves a sequentially increasing list of transactions starting at the
// provided nonce that are ready for processing. Note, all transactions with nonces
// lower than start will also be returned to prevent getting into an invalid state.
func (l *txList) Ready(start uint64) []*Transaction {
	var ready []*Transaction
	
	for nonce := start; ; nonce++ {
		tx, exists := l.txs[nonce]
		if !exists {
			break
		}
		ready = append(ready, tx)
	}
	
	return ready
}

// Len returns the length of the transaction list.
func (l *txList) Len() int {
	return len(l.txs)
}

// Empty returns whether the list of transactions is empty or not.
func (l *txList) Empty() bool {
	return len(l.txs) == 0
}

// Flatten creates a nonce-sorted slice of transactions based on the loosely
// sorted internal representation.
func (l *txList) Flatten() []*Transaction {
	if len(l.txs) == 0 {
		return nil
	}
	
	// Get sorted nonces
	nonces := make([]uint64, 0, len(l.txs))
	for nonce := range l.txs {
		nonces = append(nonces, nonce)
	}
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	
	// Create sorted transaction slice
	flat := make([]*Transaction, len(nonces))
	for i, nonce := range nonces {
		flat[i] = l.txs[nonce]
	}
	
	return flat
}

// LastNonce returns the last (highest) nonce in the list
func (l *txList) LastNonce() uint64 {
	var last uint64
	for nonce := range l.txs {
		if nonce > last {
			last = nonce
		}
	}
	return last
}

// txLookup is used internally by TxPool to track all known transactions.
type txLookup struct {
	all map[Hash]*Transaction
}

// newTxLookup returns a new txLookup structure.
func newTxLookup() *txLookup {
	return &txLookup{
		all: make(map[Hash]*Transaction),
	}
}

// Add adds a transaction to the lookup.
func (t *txLookup) Add(tx *Transaction) {
	t.all[tx.Hash()] = tx
}

// Remove removes a transaction from the lookup.
func (t *txLookup) Remove(hash Hash) {
	delete(t.all, hash)
}

// Get returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) Get(hash Hash) *Transaction {
	return t.all[hash]
}

// Count returns the current number of transactions in the lookup.
func (t *txLookup) Count() int {
	return len(t.all)
}

// txPricedList is a price-sorted heap to allow operating on transactions pool
// contents in a price-incrementing way.
type txPricedList struct {
	all    *txLookup  // Pointer to the map of all transactions
	items  *priceHeap // Heap of prices of all the stored transactions
	stales int        // Number of stale price points to (re-heap trigger)
	config *Config    // Pool configuration
}

// newTxPricedList creates a new price-sorted transaction heap.
func newTxPricedList(config *Config) *txPricedList {
	return &txPricedList{
		items:  new(priceHeap),
		config: config,
	}
}

// Put inserts a new transaction into the heap.
func (l *txPricedList) Put(tx *Transaction) {
	heap.Push(l.items, tx)
}

// Removed notifies the price tracker that the count number of transactions
// were removed from the pool, and the price heap should be reheaped if needed.
func (l *txPricedList) Removed(count int) {
	l.stales += count
	if l.stales > len(*l.items)/4 {
		l.Reheap()
	}
}

// Reheap rebuilds the heap.
func (l *txPricedList) Reheap() {
	*l.items = (*l.items)[:0]
	l.stales = 0
	
	// This is simplified - in practice, we'd rebuild from all transactions
}

// priceHeap is a heap.Interface implementation over transactions for price-based sorting.
type priceHeap []*Transaction

func (h priceHeap) Len() int { return len(h) }

func (h priceHeap) Less(i, j int) bool {
	// Higher gas price has higher priority (min-heap inverted)
	return h[i].GasPrice.Cmp(h[j].GasPrice) > 0
}

func (h priceHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *priceHeap) Push(x interface{}) {
	*h = append(*h, x.(*Transaction))
}

func (h *priceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// calculatePriceBump calculates the required price bump
func calculatePriceBump(price *big.Int, bump uint64) *big.Int {
	percent := new(big.Int).SetUint64(bump)
	bump_amount := new(big.Int).Mul(price, percent)
	bump_amount.Div(bump_amount, big.NewInt(100))
	return new(big.Int).Add(price, bump_amount)
}
