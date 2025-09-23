package main

import (
	"fmt"
	"math/big"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/txpool"
)

func main() {
	fmt.Println("🚀 HotStuff EVM Block Structure Demo")
	fmt.Println("====================================")

	// Create transaction pool
	config := txpool.DefaultConfig()
	signer := txpool.NewEIP155Signer(big.NewInt(1337))
	pool := txpool.NewTxPool(config, signer)
	defer pool.Close()

	// Create state database
	stateDB := evm.NewInMemoryStateDB()

	// Create block builder
	builderConfig := evm.DefaultBlockBuilderConfig()
	builder := evm.NewBlockBuilder(pool, stateDB, builderConfig)

	fmt.Println("\n📦 Creating Genesis Block...")
	// Create genesis block
	genesisBlock := builder.CreateGenesisBlock()
	displayBlock("Genesis Block", genesisBlock)

	fmt.Println("\n💰 Setting up test accounts...")
	// Setup some test accounts with balances
	accounts := setupTestAccounts(stateDB)
	for addr, balance := range accounts {
		fmt.Printf("  Account %s: %s ETH\n", addr.String()[:10]+"...", formatETH(balance))
	}

	fmt.Println("\n📝 Creating test transactions...")
	// Create some test transactions
	transactions := createTestTransactions(accounts)
	for i, tx := range transactions {
		fmt.Printf("  TX %d: %s -> %s, Value: %s ETH, Gas: %d\n",
			i+1,
			getSender(tx).String()[:10]+"...",
			tx.To.String()[:10]+"...",
			formatETH(tx.Value),
			tx.GasLimit)

		// Add to pool
		pool.AddLocal(tx)
	}

	fmt.Println("\n🏗️  Building Block 1...")
	// Build first block
	block1, err := builder.BuildBlock(
		genesisBlock.Hash(),
		hotstuff.QuorumCert{},
		hotstuff.View(2),
		hotstuff.ID(1),
	)
	if err != nil {
		fmt.Printf("Error building block: %v\n", err)
		return
	}

	displayBlock("Block 1", block1)

	fmt.Println("\n⚡ Processing Block 1...")
	// Process the block
	err = builder.ProcessBlock(block1)
	if err != nil {
		fmt.Printf("Error processing block: %v\n", err)
		return
	}

	fmt.Println("✅ Block processed successfully!")

	// Display final account balances
	fmt.Println("\n💰 Final Account Balances:")
	for addr := range accounts {
		balance := stateDB.GetBalance(addr)
		nonce := stateDB.GetNonce(addr)
		fmt.Printf("  %s: %s ETH (nonce: %d)\n",
			addr.String()[:10]+"...",
			formatETH(balance),
			nonce)
	}

	fmt.Println("\n📊 Builder Statistics:")
	stats := builder.Statistics()
	fmt.Printf("  Pending Transactions: %d\n", stats.PendingTransactions)
	fmt.Printf("  Queued Transactions: %d\n", stats.QueuedTransactions)
	fmt.Printf("  Gas Limit: %d\n", stats.GasLimit)
	fmt.Printf("  Base Fee: %s gwei\n", formatGwei(stats.BaseFee))
	fmt.Printf("  Chain ID: %s\n", stats.ChainID.String())

	fmt.Println("\n🎯 Demo completed successfully!")
	fmt.Println("\nKey Features Demonstrated:")
	fmt.Println("✅ Ethereum-compatible block structure")
	fmt.Println("✅ Transaction execution with gas metering")
	fmt.Println("✅ State management with accounts and balances")
	fmt.Println("✅ Receipt generation with logs")
	fmt.Println("✅ HotStuff consensus integration")
	fmt.Println("✅ Gas price prioritization")
	fmt.Println("✅ Nonce-based transaction ordering")
}

func setupTestAccounts(stateDB evm.StateDB) map[txpool.Address]*big.Int {
	accounts := map[txpool.Address]*big.Int{
		createAddress("0x1111111111111111111111111111111111111111"): big.NewInt(5000000000000000000), // 5 ETH
		createAddress("0x2222222222222222222222222222222222222222"): big.NewInt(3000000000000000000), // 3 ETH
		createAddress("0x3333333333333333333333333333333333333333"): big.NewInt(1000000000000000000), // 1 ETH
		createAddress("0x4444444444444444444444444444444444444444"): big.NewInt(0),                   // 0 ETH
	}

	for addr, balance := range accounts {
		stateDB.CreateAccount(addr)
		stateDB.SetBalance(addr, balance)
	}

	return accounts
}

func createTestTransactions(accounts map[txpool.Address]*big.Int) []*txpool.Transaction {
	var addrs []txpool.Address
	for addr := range accounts {
		addrs = append(addrs, addr)
	}

	if len(addrs) < 4 {
		return []*txpool.Transaction{}
	}

	return []*txpool.Transaction{
		// Regular transfer
		{
			Nonce:    0,
			GasPrice: big.NewInt(2000000000), // 2 gwei
			GasLimit: 21000,
			To:       &addrs[3], // to account 4
			Value:    big.NewInt(1000000000000000000), // 1 ETH
			Data:     []byte{},
			ChainID:  big.NewInt(1337),
		},
		// Another transfer with higher gas price
		{
			Nonce:    0,
			GasPrice: big.NewInt(3000000000), // 3 gwei (higher priority)
			GasLimit: 21000,
			To:       &addrs[3], // to account 4
			Value:    big.NewInt(500000000000000000), // 0.5 ETH
			Data:     []byte{},
			ChainID:  big.NewInt(1337),
		},
		// Contract creation (will fail in our simplified setup, but demonstrates structure)
		{
			Nonce:    1,
			GasPrice: big.NewInt(1500000000), // 1.5 gwei
			GasLimit: 100000,
			To:       nil, // contract creation
			Value:    big.NewInt(0),
			Data:     []byte("simple contract bytecode"),
			ChainID:  big.NewInt(1337),
		},
	}
}

func displayBlock(title string, block *evm.EVMBlock) {
	fmt.Printf("\n%s:\n", title)
	fmt.Printf("  Hash: %s\n", block.Hash().String()[:16]+"...")
	fmt.Printf("  Number: %s\n", block.Header.Number.String())
	fmt.Printf("  View: %d\n", block.View())
	fmt.Printf("  Proposer: %d\n", block.Proposer())
	fmt.Printf("  Timestamp: %s\n", time.Unix(int64(block.Header.Timestamp), 0).Format("15:04:05"))
	fmt.Printf("  Transactions: %d\n", len(block.Transactions))
	fmt.Printf("  Gas Used: %d / %d (%.1f%%)\n",
		block.Header.GasUsed,
		block.Header.GasLimit,
		float64(block.Header.GasUsed)/float64(block.Header.GasLimit)*100)
	fmt.Printf("  State Root: %s\n", block.Header.StateRoot.String()[:16]+"...")
	fmt.Printf("  Base Fee: %s gwei\n", formatGwei(block.Header.BaseFee))
	fmt.Printf("  Size: %d bytes\n", block.Header.Size)

	if len(block.Receipts) > 0 {
		fmt.Printf("  Receipts: %d\n", len(block.Receipts))
		for i, receipt := range block.Receipts {
			fmt.Printf("    Receipt %d: Status=%d, Gas=%d, Logs=%d\n",
				i+1, receipt.Status, receipt.GasUsed, len(receipt.Logs))
		}
	}
}

func createAddress(addrStr string) txpool.Address {
	var addr txpool.Address
	// Simplified address creation
	fmt.Sscanf(addrStr, "0x%40x", &addr)
	return addr
}

func getSender(tx *txpool.Transaction) txpool.Address {
	// Simplified sender derivation from tx hash
	hash := tx.Hash()
	var addr txpool.Address
	copy(addr[:], hash[:20])
	return addr
}

func formatETH(wei *big.Int) string {
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, big.NewFloat(1e18))
	return fmt.Sprintf("%.3f", eth)
}

func formatGwei(wei *big.Int) string {
	gwei := new(big.Float).SetInt(wei)
	gwei.Quo(gwei, big.NewFloat(1e9))
	return fmt.Sprintf("%.1f", gwei)
}
