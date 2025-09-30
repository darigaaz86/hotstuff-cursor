package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/relab/hotstuff/txpool"
)

func main() {
	var (
		to       = flag.String("to", "", "Recipient address (empty for contract creation)")
		value    = flag.String("value", "0", "Value to send in wei")
		gasLimit = flag.Uint64("gas", 21000, "Gas limit")
		gasPrice = flag.String("gasPrice", "1000000000", "Gas price in wei")
		data     = flag.String("data", "", "Transaction data (hex)")
		nonce    = flag.Uint64("nonce", 0, "Transaction nonce")
		chainID  = flag.Int64("chainId", 1337, "Chain ID")
		genKey   = flag.Bool("genkey", false, "Generate a new key pair")
		privKey  = flag.String("key", "", "Private key (hex, without 0x prefix)")
	)
	flag.Parse()

	// Generate key pair if requested
	if *genKey {
		generateKeyPair()
		return
	}

	if *privKey == "" {
		fmt.Println("Error: Private key is required. Use -key flag or -genkey to generate a new key.")
		os.Exit(1)
	}

	// Parse private key
	privateKey, err := parsePrivateKey(*privKey)
	if err != nil {
		log.Fatalf("Invalid private key: %v", err)
	}

	// Derive address from private key
	address := txpool.AddressFromPublicKey(&privateKey.PublicKey)
	fmt.Printf("From address: 0x%x\n", address[:])

	// Parse parameters
	valueBig := new(big.Int)
	if _, ok := valueBig.SetString(*value, 10); !ok {
		log.Fatalf("Invalid value: %s", *value)
	}

	gasPriceBig := new(big.Int)
	if _, ok := gasPriceBig.SetString(*gasPrice, 10); !ok {
		log.Fatalf("Invalid gas price: %s", *gasPrice)
	}

	var toAddr *txpool.Address
	if *to != "" {
		if len(*to) < 2 || (*to)[:2] != "0x" {
			log.Fatalf("Invalid 'to' address format. Use 0x prefix")
		}
		toBytes, err := hex.DecodeString((*to)[2:])
		if err != nil || len(toBytes) != 20 {
			log.Fatalf("Invalid 'to' address: %s", *to)
		}
		addr := txpool.Address{}
		copy(addr[:], toBytes)
		toAddr = &addr
	}

	var dataBytes []byte
	if *data != "" {
		if len(*data) >= 2 && (*data)[:2] == "0x" {
			*data = (*data)[2:]
		}
		dataBytes, err = hex.DecodeString(*data)
		if err != nil {
			log.Fatalf("Invalid data: %s", err)
		}
	}

	// Create and sign transaction
	tx, err := txpool.CreateSignedTransaction(
		*nonce,
		toAddr,
		valueBig,
		*gasLimit,
		gasPriceBig,
		dataBytes,
		big.NewInt(*chainID),
		privateKey,
	)
	if err != nil {
		log.Fatalf("Failed to create transaction: %v", err)
	}

	// Encode transaction as RLP for eth_sendRawTransaction
	rawTx := encodeTransaction(tx)

	fmt.Printf("\n=== Signed Transaction ===\n")
	fmt.Printf("Hash: 0x%x\n", tx.Hash())
	fmt.Printf("From: 0x%x\n", address)
	if toAddr != nil {
		fmt.Printf("To: 0x%x\n", *toAddr)
	} else {
		fmt.Printf("To: [Contract Creation]\n")
	}
	fmt.Printf("Value: %s wei\n", valueBig.String())
	fmt.Printf("Gas: %d\n", *gasLimit)
	fmt.Printf("Gas Price: %s wei\n", gasPriceBig.String())
	fmt.Printf("Nonce: %d\n", *nonce)
	fmt.Printf("Data: 0x%x\n", dataBytes)
	fmt.Printf("\n=== For eth_sendRawTransaction ===\n")
	fmt.Printf("Raw Transaction: 0x%x\n", rawTx)
	fmt.Printf("\n=== curl command ===\n")
	fmt.Printf(`curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_sendRawTransaction",
    "params": ["0x%x"],
    "id": 1
  }'`, rawTx)
	fmt.Println()
}

func generateKeyPair() {
	privateKey, publicKey, err := txpool.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}

	address := txpool.AddressFromPublicKey(publicKey)

	fmt.Printf("=== Generated Key Pair ===\n")
	fmt.Printf("Private Key: %x\n", privateKey.D.Bytes())
	fmt.Printf("Public Key: %x%x\n", publicKey.X.Bytes(), publicKey.Y.Bytes())
	fmt.Printf("Address: 0x%x\n", address[:])
	fmt.Printf("\n=== Usage ===\n")
	fmt.Printf("Save the private key securely!\n")
	fmt.Printf("Use it with: ./sign-tx -key %x -to 0x... -value 1000000000000000000\n", privateKey.D.Bytes())
}

func parsePrivateKey(keyHex string) (*ecdsa.PrivateKey, error) {
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, err
	}

	privateKey := new(ecdsa.PrivateKey)
	privateKey.D = new(big.Int).SetBytes(keyBytes)
	privateKey.PublicKey.Curve = txpool.GetCurve()
	privateKey.PublicKey.X, privateKey.PublicKey.Y = privateKey.PublicKey.Curve.ScalarBaseMult(keyBytes)

	return privateKey, nil
}

// encodeTransaction encodes a transaction for eth_sendRawTransaction
// This is a simplified RLP encoding for demo purposes
func encodeTransaction(tx *txpool.Transaction) []byte {
	// For demo purposes, we'll create a simple encoding
	// In production, use proper RLP encoding

	result := make([]byte, 0, 200)

	// Add transaction fields (simplified)
	result = append(result, byte(tx.Nonce))
	result = append(result, tx.GasPrice.Bytes()...)
	result = append(result, byte(tx.GasLimit))

	if tx.To != nil {
		result = append(result, tx.To[:]...)
	} else {
		result = append(result, make([]byte, 20)...) // Empty for contract creation
	}

	result = append(result, tx.Value.Bytes()...)
	result = append(result, tx.Data...)
	result = append(result, tx.V.Bytes()...)
	result = append(result, tx.R.Bytes()...)
	result = append(result, tx.S.Bytes()...)

	return result
}
