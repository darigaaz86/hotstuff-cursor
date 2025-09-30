package blockchain

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/txpool"
)

// L1Blockchain represents a Layer 1 blockchain with EVM support
type L1Blockchain struct {
	// Core components
	stateDB  evm.StateDB
	executor *evm.Executor
	txPool   *txpool.TxPool
	logger   logging.Logger

	// Block storage
	mu             sync.RWMutex
	blocks         map[hotstuff.Hash]*evm.EVMBlock
	blocksByNumber map[uint64]*evm.EVMBlock
	txToBlock      map[hotstuff.Hash]hotstuff.Hash
	latestBlock    *evm.EVMBlock
	blockNumber    uint64

	// Block production
	blockProductionEnabled bool
	stopProduction         chan bool
	chainID                *big.Int
}

// NewL1Blockchain creates a new Layer 1 blockchain
func NewL1Blockchain(config L1BlockchainConfig) *L1Blockchain {
	bc := &L1Blockchain{
		stateDB:        config.StateDB,
		executor:       config.Executor,
		txPool:         config.TxPool,
		logger:         logging.New("l1-blockchain"),
		blocks:         make(map[hotstuff.Hash]*evm.EVMBlock),
		blocksByNumber: make(map[uint64]*evm.EVMBlock),
		txToBlock:      make(map[hotstuff.Hash]hotstuff.Hash),
		blockNumber:    0,
		stopProduction: make(chan bool),
		chainID:        big.NewInt(1337),
	}

	// Initialize genesis block
	bc.initGenesis()

	// Start block production
	bc.StartBlockProduction()

	return bc
}

// L1BlockchainConfig holds configuration for L1Blockchain
type L1BlockchainConfig struct {
	StateDB  evm.StateDB
	Executor *evm.Executor
	TxPool   *txpool.TxPool
}

// StartBlockProduction starts automatic block production
func (bc *L1Blockchain) StartBlockProduction() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.blockProductionEnabled {
		return // Already running
	}

	bc.blockProductionEnabled = true
	bc.logger.Info("Starting Layer 1 blockchain block production...")

	go bc.blockProductionLoop()
}

// StopBlockProduction stops automatic block production
func (bc *L1Blockchain) StopBlockProduction() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if !bc.blockProductionEnabled {
		return // Not running
	}

	bc.blockProductionEnabled = false
	bc.logger.Info("Stopping Layer 1 blockchain block production...")

	// Send stop signal
	select {
	case bc.stopProduction <- true:
	default:
	}
}

// blockProductionLoop runs the automatic block production
func (bc *L1Blockchain) blockProductionLoop() {
	ticker := time.NewTicker(3 * time.Second) // Create blocks every 3 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bc.ProcessPendingTransactions()
		case <-bc.stopProduction:
			bc.logger.Info("Layer 1 blockchain block production stopped")
			return
		}
	}
}

// ProcessPendingTransactions processes transactions from the txpool and creates a new block
func (bc *L1Blockchain) ProcessPendingTransactions() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Get pending transactions from the pool
	pendingTxs := bc.txPool.GetTransactionsForBlock(8000000) // Use block gas limit
	if len(pendingTxs) == 0 {
		return // No transactions to process
	}

	bc.blockNumber++
	bc.logger.Infof("Creating block %d with %d transactions", bc.blockNumber, len(pendingTxs))

	// Create new block
	newBlock := &evm.EVMBlock{
		Header: evm.EVMBlockHeader{
			Number:      big.NewInt(int64(bc.blockNumber)),
			GasLimit:    8000000,
			GasUsed:     0,
			Timestamp:   uint64(time.Now().Unix()),
			Coinbase:    bc.deriveCoinbaseAddress(),
			Difficulty:  big.NewInt(1),
			BaseFee:     big.NewInt(1000000000),
			StateRoot:   bc.stateDB.GetStateRoot(),
			TxRoot:      hotstuff.Hash{},
			ReceiptRoot: hotstuff.Hash{},
		},
		Transactions: make([]*txpool.Transaction, 0),
		Receipts:     make([]*evm.TransactionReceipt, 0),
	}

	// Set parent hash
	if bc.latestBlock != nil {
		newBlock.Header.ParentHash = bc.latestBlock.Hash()
	}

	// Execute transactions
	var cumulativeGasUsed uint64
	for i, tx := range pendingTxs {
		// Derive sender for execution (simplified)
		from := bc.deriveSenderFromTx(tx)

		// Ensure sender has some balance for gas
		if bc.stateDB.GetBalance(from).Sign() == 0 {
			// Fund sender with some ETH for demo
			bc.stateDB.CreateAccount(from)
			balance := new(big.Int)
			balance.SetString("1000000000000000000000", 10) // 1000 ETH
			bc.stateDB.SetBalance(from, balance)
			bc.logger.Infof("Auto-funded account %s with 1000 ETH for demo", from.String()[:10])
		}

		receipt, err := bc.executor.ExecuteTransaction(tx, bc.stateDB, newBlock, uint64(i), cumulativeGasUsed)
		if err != nil {
			bc.logger.Errorf("Transaction execution failed for %s: %v", tx.Hash().String()[:10], err)
			// Create failed receipt
			receipt = &evm.TransactionReceipt{
				TxHash:            tx.Hash(),
				TxIndex:           uint64(i),
				From:              from,
				To:                tx.To,
				Status:            0,           // Failed
				GasUsed:           tx.GasLimit, // Assume all gas used on failure
				CumulativeGasUsed: cumulativeGasUsed + tx.GasLimit,
				Logs:              []*evm.Log{},
				EffectiveGasPrice: tx.GasPrice,
			}
		}

		newBlock.Transactions = append(newBlock.Transactions, tx)
		newBlock.Receipts = append(newBlock.Receipts, receipt)
		cumulativeGasUsed = receipt.CumulativeGasUsed

		// Map transaction to block
		txHash := tx.Hash()
		var txHashHotstuff hotstuff.Hash
		copy(txHashHotstuff[:], txHash[:])
		bc.txToBlock[txHashHotstuff] = newBlock.Hash()
	}

	// Update block header
	newBlock.Header.GasUsed = cumulativeGasUsed
	newBlock.Header.StateRoot = bc.stateDB.GetStateRoot()
	// For now, simplified root calculations
	newBlock.Header.TxRoot = hotstuff.Hash{}
	newBlock.Header.ReceiptRoot = hotstuff.Hash{}

	// Store block
	blockHash := newBlock.Hash()
	bc.blocks[blockHash] = newBlock
	bc.blocksByNumber[bc.blockNumber] = newBlock
	bc.latestBlock = newBlock

	// Remove processed transactions from pool
	bc.txPool.RemoveTransactions(pendingTxs)

	bc.logger.Infof("Block %d created: %s, gas used: %d/%d",
		bc.blockNumber, blockHash.String()[:10], cumulativeGasUsed, newBlock.Header.GasLimit)
}

// initGenesis initializes the genesis block
func (bc *L1Blockchain) initGenesis() {
	genesis := &evm.EVMBlock{
		Header: evm.EVMBlockHeader{
			Number:      big.NewInt(0),
			GasLimit:    8000000,
			GasUsed:     0,
			Timestamp:   uint64(time.Now().Unix()),
			Coinbase:    txpool.Address{},
			Difficulty:  big.NewInt(1),
			BaseFee:     big.NewInt(1000000000),
			StateRoot:   bc.stateDB.GetStateRoot(),
			TxRoot:      hotstuff.Hash{},
			ReceiptRoot: hotstuff.Hash{},
		},
		Transactions: []*txpool.Transaction{},
		Receipts:     []*evm.TransactionReceipt{},
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	genesisHash := genesis.Hash()
	bc.blocks[genesisHash] = genesis
	bc.blocksByNumber[0] = genesis
	bc.latestBlock = genesis
	bc.logger.Infof("Genesis block initialized: %s", genesisHash.String()[:10])
}

// deriveCoinbaseAddress derives a coinbase address for block rewards
func (bc *L1Blockchain) deriveCoinbaseAddress() txpool.Address {
	// For demo, use a simple derivation
	var coinbase txpool.Address
	coinbaseStr := fmt.Sprintf("miner_%d", bc.blockNumber)
	copy(coinbase[:], coinbaseStr)
	return coinbase
}

// deriveSenderFromTx derives sender address from transaction (simplified for demo)
func (bc *L1Blockchain) deriveSenderFromTx(tx *txpool.Transaction) txpool.Address {
	hash := tx.Hash()
	var addr txpool.Address
	copy(addr[:], hash[:20])
	return addr
}

// GetBlock returns a block by hash
func (bc *L1Blockchain) GetBlock(hash hotstuff.Hash) (*evm.EVMBlock, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	block, exists := bc.blocks[hash]
	if !exists {
		return nil, fmt.Errorf("block not found")
	}
	return block, nil
}

// GetBlockByNumber returns a block by number
func (bc *L1Blockchain) GetBlockByNumber(number uint64) (*evm.EVMBlock, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	block, exists := bc.blocksByNumber[number]
	if !exists {
		return nil, fmt.Errorf("block not found")
	}
	return block, nil
}

// GetLatestBlock returns the latest block
func (bc *L1Blockchain) GetLatestBlock() (*evm.EVMBlock, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.latestBlock == nil {
		return nil, fmt.Errorf("no blocks available")
	}
	return bc.latestBlock, nil
}

// GetBlockNumber returns the current block number
func (bc *L1Blockchain) GetBlockNumber() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.blockNumber
}

// GetTransaction returns a transaction by hash and its block
func (bc *L1Blockchain) GetTransaction(hash hotstuff.Hash) (*txpool.Transaction, *evm.EVMBlock, uint64, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Check if transaction is in a block
	blockHash, exists := bc.txToBlock[hash]
	if !exists {
		return nil, nil, 0, fmt.Errorf("transaction not found")
	}

	block, exists := bc.blocks[blockHash]
	if !exists {
		return nil, nil, 0, fmt.Errorf("block not found")
	}

	// Find transaction in block
	for i, tx := range block.Transactions {
		txHash := tx.Hash()
		var txHashHotstuff hotstuff.Hash
		copy(txHashHotstuff[:], txHash[:])
		if txHashHotstuff == hash {
			return tx, block, uint64(i), nil
		}
	}

	return nil, nil, 0, fmt.Errorf("transaction not found in block")
}

// GetTransactionReceipt returns a transaction receipt
func (bc *L1Blockchain) GetTransactionReceipt(hash hotstuff.Hash) (*evm.TransactionReceipt, *evm.EVMBlock, error) {
	_, block, txIndex, err := bc.GetTransaction(hash)
	if err != nil {
		return nil, nil, err
	}

	if int(txIndex) >= len(block.Receipts) {
		return nil, nil, fmt.Errorf("receipt not found")
	}

	return block.Receipts[txIndex], block, nil
}

// Close shuts down the blockchain
func (bc *L1Blockchain) Close() {
	bc.StopBlockProduction()
}
