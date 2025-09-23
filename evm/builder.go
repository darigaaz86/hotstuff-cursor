package evm

import (
	"fmt"
	"math/big"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/txpool"
)

// BlockBuilder builds EVM-compatible blocks for HotStuff consensus
type BlockBuilder struct {
	txPool   *txpool.TxPool
	executor *Executor
	stateDB  StateDB
	config   BlockBuilderConfig
	logger   logging.Logger
}

// BlockBuilderConfig holds configuration for block building
type BlockBuilderConfig struct {
	GasLimit  uint64        // Maximum gas per block
	BaseFee   *big.Int      // Base fee for EIP-1559
	ChainID   *big.Int      // Chain ID
	BlockTime time.Duration // Target block time
	MaxTxs    int           // Maximum transactions per block
}

// DefaultBlockBuilderConfig returns default configuration
func DefaultBlockBuilderConfig() BlockBuilderConfig {
	return BlockBuilderConfig{
		GasLimit:  8000000,                // 8M gas limit (similar to Ethereum)
		BaseFee:   big.NewInt(1000000000), // 1 gwei
		ChainID:   big.NewInt(1337),       // Local development chain
		BlockTime: 12 * time.Second,       // 12 second blocks
		MaxTxs:    100,                    // Maximum 100 transactions per block
	}
}

// NewBlockBuilder creates a new EVM block builder
func NewBlockBuilder(txPool *txpool.TxPool, stateDB StateDB, config BlockBuilderConfig) *BlockBuilder {
	executorConfig := ExecutionConfig{
		GasLimit: config.GasLimit,
		BaseFee:  config.BaseFee,
		ChainID:  config.ChainID,
	}

	return &BlockBuilder{
		txPool:   txPool,
		executor: NewExecutor(executorConfig),
		stateDB:  stateDB,
		config:   config,
		logger:   logging.New("evm-builder"),
	}
}

// BuildBlock creates a new EVM block for proposal
func (bb *BlockBuilder) BuildBlock(parent hotstuff.Hash, cert hotstuff.QuorumCert,
	view hotstuff.View, proposer hotstuff.ID) (*EVMBlock, error) {

	bb.logger.Infof("Building block for view %d", view)

	// Get transactions from the pool
	transactions := bb.selectTransactions()

	bb.logger.Infof("Selected %d transactions for block", len(transactions))

	// Validate transactions against current state
	if err := bb.executor.ValidateTransactionList(transactions, bb.stateDB); err != nil {
		bb.logger.Warnf("Transaction validation failed, creating empty block: %v", err)
		transactions = []*txpool.Transaction{} // Create empty block if validation fails
	}

	// Get current state root
	stateRoot, err := bb.stateDB.Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to get state root: %w", err)
	}

	// Create the block
	block := NewEVMBlock(parent, cert, transactions, view, proposer, stateRoot, bb.config.GasLimit)

	// Execute transactions and generate receipts
	if len(transactions) > 0 {
		receipts, err := bb.executor.ExecuteBlock(block, bb.stateDB)
		if err != nil {
			bb.logger.Errorf("Block execution failed: %v", err)
			// Create empty block on execution failure
			block = NewEVMBlock(parent, cert, []*txpool.Transaction{}, view, proposer, stateRoot, bb.config.GasLimit)
		} else {
			// Update block with execution results
			block.UpdateReceipts(receipts)

			// Remove executed transactions from pool
			bb.removeExecutedTransactions(transactions)
		}
	}

	bb.logger.Infof("Built block %s with %d transactions, gas used: %d/%d",
		block.Hash().String()[:8], len(block.Transactions), block.Header.GasUsed, block.Header.GasLimit)

	return block, nil
}

// selectTransactions selects transactions from the pool for block inclusion
func (bb *BlockBuilder) selectTransactions() []*txpool.Transaction {
	// Get transactions from pool ordered by gas price
	txs := bb.txPool.GetTransactionsForBlock(bb.config.GasLimit)

	// Limit to maximum transactions per block
	if len(txs) > bb.config.MaxTxs {
		txs = txs[:bb.config.MaxTxs]
	}

	bb.logger.Debugf("Pool returned %d transactions", len(txs))

	return txs
}

// removeExecutedTransactions removes transactions from the pool after execution
func (bb *BlockBuilder) removeExecutedTransactions(transactions []*txpool.Transaction) {
	// For now, we don't have a direct removal method in the pool
	// In a full implementation, we would remove these transactions
	// This would be implemented in the txpool package
	bb.logger.Debugf("Would remove %d executed transactions from pool", len(transactions))
}

// ProcessBlock processes an incoming block (from consensus)
func (bb *BlockBuilder) ProcessBlock(block *EVMBlock) error {
	bb.logger.Infof("Processing block %s with %d transactions",
		block.Hash().String()[:8], len(block.Transactions))

	// Create a copy of state for execution
	stateDB := bb.stateDB.Copy()

	// Execute all transactions
	receipts, err := bb.executor.ExecuteBlock(block, stateDB)
	if err != nil {
		return fmt.Errorf("failed to execute block: %w", err)
	}

	// Verify receipts match the block
	if err := bb.verifyReceipts(block, receipts); err != nil {
		return fmt.Errorf("receipt verification failed: %w", err)
	}

	// Update our state with the executed block
	bb.stateDB = stateDB

	// Remove processed transactions from pool
	bb.removeExecutedTransactions(block.Transactions)

	bb.logger.Infof("Successfully processed block %s", block.Hash().String()[:8])
	return nil
}

// verifyReceipts verifies that receipts match the block's expected state
func (bb *BlockBuilder) verifyReceipts(block *EVMBlock, receipts []*TransactionReceipt) error {
	if len(receipts) != len(block.Transactions) {
		return fmt.Errorf("receipt count mismatch: expected %d, got %d",
			len(block.Transactions), len(receipts))
	}

	// Verify receipt hashes match transaction hashes
	for i, receipt := range receipts {
		if receipt.TxHash != block.Transactions[i].Hash() {
			return fmt.Errorf("receipt %d hash mismatch", i)
		}
	}

	// Verify cumulative gas used
	if len(receipts) > 0 {
		finalGasUsed := receipts[len(receipts)-1].CumulativeGasUsed
		if finalGasUsed != block.Header.GasUsed {
			return fmt.Errorf("gas used mismatch: block header %d, receipts %d",
				block.Header.GasUsed, finalGasUsed)
		}
	}

	return nil
}

// GetStateDB returns the current state database
func (bb *BlockBuilder) GetStateDB() StateDB {
	return bb.stateDB
}

// GetPendingTransactions returns currently pending transactions
func (bb *BlockBuilder) GetPendingTransactions() []*txpool.Transaction {
	pending := bb.txPool.Pending()
	var transactions []*txpool.Transaction

	for _, txList := range pending {
		transactions = append(transactions, txList...)
	}

	return transactions
}

// EstimateGas estimates gas for a transaction
func (bb *BlockBuilder) EstimateGas(tx *txpool.Transaction) (uint64, error) {
	return bb.executor.EstimateGas(tx, bb.stateDB)
}

// GetTransactionReceipt returns the receipt for a transaction (mock implementation)
func (bb *BlockBuilder) GetTransactionReceipt(txHash txpool.Hash) *TransactionReceipt {
	// In a full implementation, this would look up receipts from storage
	// For now, return nil (not found)
	return nil
}

// GetBlockByHash returns a block by its hash (mock implementation)
func (bb *BlockBuilder) GetBlockByHash(hash hotstuff.Hash) *EVMBlock {
	// In a full implementation, this would look up blocks from storage
	// For now, return nil (not found)
	return nil
}

// GetBlockByNumber returns a block by its number (mock implementation)
func (bb *BlockBuilder) GetBlockByNumber(number *big.Int) *EVMBlock {
	// In a full implementation, this would look up blocks from storage
	// For now, return nil (not found)
	return nil
}

// Statistics returns block builder statistics
func (bb *BlockBuilder) Statistics() BlockBuilderStats {
	pending, queued := bb.txPool.Stats()

	return BlockBuilderStats{
		PendingTransactions: pending,
		QueuedTransactions:  queued,
		GasLimit:            bb.config.GasLimit,
		BaseFee:             bb.config.BaseFee,
		ChainID:             bb.config.ChainID,
	}
}

// BlockBuilderStats contains statistics about the block builder
type BlockBuilderStats struct {
	PendingTransactions int
	QueuedTransactions  int
	GasLimit            uint64
	BaseFee             *big.Int
	ChainID             *big.Int
}

// CreateGenesisBlock creates the genesis block for the chain
func (bb *BlockBuilder) CreateGenesisBlock() *EVMBlock {
	// Setup genesis accounts
	if memStateDB, ok := bb.stateDB.(*InMemoryStateDB); ok {
		memStateDB.SetupGenesisAccounts()
	}

	// Get genesis state root
	stateRoot, _ := bb.stateDB.Commit()

	// Create genesis block
	genesisBlock := NewEVMBlock(
		hotstuff.Hash{},         // No parent
		hotstuff.QuorumCert{},   // No certificate
		[]*txpool.Transaction{}, // No transactions
		1,                       // Genesis view
		0,                       // Genesis proposer
		stateRoot,
		bb.config.GasLimit,
	)

	// Set genesis block number to 0
	genesisBlock.Header.Number = big.NewInt(0)
	genesisBlock.Header.Timestamp = uint64(time.Now().Unix())

	// Recalculate hash with updated header
	genesisBlock.hash = genesisBlock.calculateHash()

	bb.logger.Infof("Created genesis block %s", genesisBlock.Hash().String()[:8])
	return genesisBlock
}
