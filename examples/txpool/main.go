package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/relab/hotstuff/txpool"
)

func main() {
	fmt.Println("🚀 HotStuff Transaction Pool Example")
	fmt.Println("=====================================")

	// Create transaction pool configuration
	config := txpool.DefaultConfig()
	config.PriceLimit = 1000000000 // 1 Gwei minimum

	// Create EIP-155 signer for chain ID 1337 (local development)
	signer := txpool.NewEIP155Signer(big.NewInt(1337))

	// Create the transaction pool
	pool := txpool.NewTxPool(config, signer)
	defer pool.Close()

	fmt.Printf("✅ Created transaction pool with chain ID: %d\n", 1337)

	// Subscribe to new transactions
	txCh := pool.Subscribe()
	go func() {
		for tx := range txCh {
			fmt.Printf("📨 New transaction: hash=%s nonce=%d gasPrice=%s\n",
				tx.Hash().String()[:10]+"...", tx.Nonce, tx.GasPrice.String())
		}
	}()

	// Generate test accounts
	accounts := make([]*ecdsa.PrivateKey, 3)
	for i := range accounts {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatalf("Failed to generate account %d: %v", i, err)
		}
		accounts[i] = privateKey
	}

	fmt.Printf("✅ Generated %d test accounts\n", len(accounts))

	// Create and submit transactions
	fmt.Println("\n📤 Submitting transactions...")

	for i, account := range accounts {
		for nonce := uint64(0); nonce < 3; nonce++ {
			// Create recipient address
			to := txpool.Address{byte(i + 1), byte(nonce + 1)}

			// Create transaction with varying gas prices
			gasPrice := big.NewInt(1000000000 + int64(i*500000000)) // Varying gas prices

			tx := txpool.NewTransaction(
				nonce,
				&to,
				big.NewInt(1000+int64(nonce*100)), // Varying amounts
				21000,
				gasPrice,
				[]byte(fmt.Sprintf("data-%d-%d", i, nonce)),
			)

			// Sign the transaction
			err := tx.Sign(account)
			if err != nil {
				log.Printf("Failed to sign transaction: %v", err)
				continue
			}

			// Submit to pool
			err = pool.AddLocal(tx)
			if err != nil {
				log.Printf("Failed to add transaction: %v", err)
				continue
			}

			fmt.Printf("  ✓ Account %d, Nonce %d: %s\n", i, nonce, tx.Hash().String()[:10]+"...")
		}
	}

	// Give some time for async processing
	time.Sleep(100 * time.Millisecond)

	// Show pool statistics
	pending, queued := pool.Stats()
	fmt.Printf("\n📊 Pool Statistics:\n")
	fmt.Printf("   Pending: %d transactions\n", pending)
	fmt.Printf("   Queued:  %d transactions\n", queued)

	// Get pending transactions
	pendingTxs := pool.Pending()
	fmt.Printf("\n📋 Pending transactions by account:\n")
	for addr, txs := range pendingTxs {
		fmt.Printf("   %s: %d transactions\n", addr.String()[:10]+"...", len(txs))
		for _, tx := range txs {
			fmt.Printf("     - Nonce %d, Gas Price: %s Wei\n", tx.Nonce, tx.GasPrice.String())
		}
	}

	// Simulate block creation
	fmt.Println("\n⛏️  Creating block...")
	blockGasLimit := uint64(100000)
	blockTxs := pool.GetTransactionsForBlock(blockGasLimit)

	fmt.Printf("   Selected %d transactions for block (gas limit: %d)\n", len(blockTxs), blockGasLimit)

	// Convert transactions to HotStuff commands
	commands := pool.ToCommands(blockTxs)
	fmt.Printf("   Converted to %d HotStuff commands\n", len(commands))

	// Show command conversion
	fmt.Println("\n🔄 Command Conversion Example:")
	if len(commands) > 0 {
		// Take first command and convert back
		originalTx := blockTxs[0]
		cmd := commands[0]

		fmt.Printf("   Original TX Hash: %s\n", originalTx.Hash().String()[:16]+"...")
		fmt.Printf("   Command Length:   %d bytes\n", len(cmd))

		// Convert back to transaction
		recoveredTx, err := txpool.TransactionFromCommand(cmd)
		if err != nil {
			log.Printf("Failed to recover transaction: %v", err)
		} else {
			fmt.Printf("   Recovered TX:     Hash=%s, Nonce=%d\n",
				recoveredTx.Hash().String()[:16]+"...", recoveredTx.Nonce)

			if recoveredTx.Hash() == originalTx.Hash() {
				fmt.Printf("   ✅ Perfect round-trip conversion!\n")
			} else {
				fmt.Printf("   ❌ Hash mismatch in conversion\n")
			}
		}
	}

	// Demonstrate transaction validation
	fmt.Println("\n🔍 Transaction Validation:")

	// Create invalid transaction (negative value)
	invalidTx := &txpool.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(1000000000),
		GasLimit: 21000,
		Value:    big.NewInt(-1000), // Invalid negative value
		ChainID:  big.NewInt(1337),
	}

	err := invalidTx.Validate()
	if err != nil {
		fmt.Printf("   ✅ Correctly rejected invalid transaction: %v\n", err)
	} else {
		fmt.Printf("   ❌ Should have rejected invalid transaction\n")
	}

	// Demonstrate gas price prioritization
	fmt.Println("\n⛽ Gas Price Prioritization:")
	if len(blockTxs) >= 2 {
		fmt.Printf("   Highest gas price: %s Wei (selected first)\n", blockTxs[0].GasPrice.String())
		fmt.Printf("   Second highest:    %s Wei\n", blockTxs[1].GasPrice.String())

		if blockTxs[0].GasPrice.Cmp(blockTxs[1].GasPrice) >= 0 {
			fmt.Printf("   ✅ Transactions correctly ordered by gas price\n")
		} else {
			fmt.Printf("   ❌ Transaction ordering issue\n")
		}
	}

	// Summary
	fmt.Println("\n🎯 Summary:")
	fmt.Println("   - Transaction pool successfully created and configured")
	fmt.Println("   - Multiple accounts created and transactions submitted")
	fmt.Println("   - Transactions validated and prioritized by gas price")
	fmt.Println("   - Seamless integration with HotStuff command system")
	fmt.Println("   - Ready for EVM integration!")

	fmt.Println("\n✨ Transaction pool demonstration completed!")
}
