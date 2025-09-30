# HotStuff: Layer 1 EVM-Compatible Blockchain

[![Go Reference](https://pkg.go.dev/badge/github.com/relab/consensus.svg)](https://pkg.go.dev/github.com/relab/hotstuff)
![Test](https://github.com/relab/hotstuff/workflows/Test/badge.svg)
![golangci-lint](https://github.com/relab/hotstuff/workflows/golangci-lint/badge.svg)
[![codecov](https://codecov.io/gh/relab/hotstuff/branch/master/graph/badge.svg?token=IYZ7WD6ZAH)](https://codecov.io/gh/relab/hotstuff)

HotStuff is a **complete Layer 1 blockchain** that combines the robust HotStuff consensus protocol with a full Ethereum Virtual Machine (EVM) implementation. It provides a production-ready blockchain with smart contract support, cryptographic transaction signing, and Ethereum-compatible JSON-RPC API.

## 🚀 **Key Features**

- **🔥 HotStuff Consensus**: Byzantine fault-tolerant consensus with optimal latency
- **⚡ EVM Compatibility**: Full Ethereum Virtual Machine with smart contract execution
- **🔐 Cryptographic Security**: Required transaction signing with ECDSA and EIP-155
- **🌐 JSON-RPC API**: Standard Ethereum-compatible interface (eth_sendRawTransaction, eth_blockNumber, etc.)
- **🗃️ Persistent Storage**: BadgerDB-backed state with Merkle Patricia Trie
- **💰 Transaction Pool**: Mempool with gas price prioritization
- **🔧 Developer Tools**: Key generation and transaction signing utilities
- **🌍 MetaMask Compatible**: Connect with popular Ethereum wallets

## 📋 **Contents**

- [Quick Start](#quick-start)
- [Layer 1 Blockchain Usage](#layer-1-blockchain-usage)
- [Smart Contract Deployment](#smart-contract-deployment)
- [Transaction Signing](#transaction-signing)
- [JSON-RPC API](#json-rpc-api)
- [Development](#development)
- [Research Framework](#research-framework)

## 🚀 **Quick Start**

### Build the Blockchain

```bash
# Clone and build
git clone https://github.com/relab/hotstuff
cd hotstuff
make

# Build transaction signing utility
go build -o sign-tx ./cmd/sign-tx
```

### Start Layer 1 Blockchain

```bash
# Start HotStuff blockchain with RPC server
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0
```

Expected output:

```
INFO    l1-blockchain   Layer 1 blockchain initialized with automatic block production
INFO    rpc-server      JSON-RPC server started on 127.0.0.1:8545
INFO    rpc-server      Ethereum JSON-RPC API available at: http://127.0.0.1:8545
INFO    rpc-server      You can now connect MetaMask to: http://127.0.0.1:8545
```

### Test Blockchain Connection

```bash
# Check chain ID
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
# Returns: {"jsonrpc":"2.0","result":"0x539","id":1}

# Check block number
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
# Returns: {"jsonrpc":"2.0","result":"0x0","id":1}
```

## 🚀 **Complete Workflow Example**

Here's a complete end-to-end example for token deployment and querying:

```bash
# 1. Start blockchain (in background)
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0 &

# 2. Generate keys
./sign-tx -genkey
# Save your private key! Example: 94051b2c142e9d29c8f32257ad7cbc9b736169b49f65d51ce8f19b3fb7a27fc6
# Save your address! Example: 0x307831346465376436306332633130366239663663336163646263653030623061396639663666656639

# 3. Deploy token contract
./sign-tx -key 94051b2c142e9d29c8f32257ad7cbc9b736169b49f65d51ce8f19b3fb7a27fc6 \
  -data 0x6064600055606460015560006000f3 -gas 500000 -gasPrice 1000000000
# Copy and run the generated curl command

# 4. Wait 5 seconds, then check block height
curl -s -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
# Should show: {"jsonrpc":"2.0","result":"0x1","id":1}

# 5. Check your ETH balance (auto-funded by HotStuff)
curl -s -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBalance",
    "params": ["YOUR_ADDRESS_FROM_STEP_2", "latest"],
    "id": 1
  }'
# Should show large balance: ~1000 ETH in wei

# 6. Check token contract storage (100 tokens total supply)
curl -s -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getStorageAt",
    "params": ["0x59221ccb2e2c66164d141ad9d6a6171bbb157900", "0x0", "latest"],
    "id": 1
  }'
# Should show: {"jsonrpc":"2.0","result":"0x64","id":1} (0x64 = 100 in hex)

# 7. Monitor blocks in real-time
while true; do
  BLOCK=$(curl -s -X POST http://127.0.0.1:8545 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | \
    grep -o '"result":"[^"]*"' | cut -d'"' -f4)
  echo "Current block: $BLOCK ($(($BLOCK)) in decimal)"
  sleep 3
done
```

**Real Output Example:**

```bash
$ curl -s -X POST http://127.0.0.1:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
{"jsonrpc":"2.0","result":"0x1","id":1}

$ curl -s -X POST http://127.0.0.1:8545 -d '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x59221ccb2e2c66164d141ad9d6a6171bbb157900","0x0","latest"],"id":1}'
{"jsonrpc":"2.0","result":"0x64","id":1}

$ # 0x64 = 100 tokens in the contract!
```

## 🔐 **Transaction Signing**

As a **real Layer 1 blockchain**, HotStuff enforces cryptographic transaction signing for security.

### Generate Key Pair

```bash
./sign-tx -genkey
```

Output:

```
=== Generated Key Pair ===
Private Key: 94051b2c142e9d29c8f32257ad7cbc9b736169b49f65d51ce8f19b3fb7a27fc6
Public Key: 56e5773b5eb534f748cf402c4cf3abbf60bcc7013f19c184630901f39ddb3fbe...
Address: 0x307831346465376436306332633130366239663663336163646263653030623061396639663666656639

=== Usage ===
Save the private key securely!
Use it with: ./sign-tx -key 94051b2c142e9d29c8f32257ad7cbc9b736169b49f65d51ce8f19b3fb7a27fc6 -to 0x... -value 1000000000000000000
```

**⚠️ Save your private key securely!**

### Sign Transactions

```bash
# Create signed transaction for contract deployment
./sign-tx -key 94051b2c142e9d29c8f32257ad7cbc9b736169b49f65d51ce8f19b3fb7a27fc6 \
  -data 0x6064600055606460015560006000f3 \
  -gas 500000 \
  -gasPrice 1000000000
```

This outputs a complete curl command for `eth_sendRawTransaction`.

## 💰 **Smart Contract Deployment & Token Operations**

### Step 1: Deploy ERC-20 Token Contract

Deploy a simple token contract that stores balances:

```bash
# Generate keys first
./sign-tx -genkey
# Save the private key and address!

# Sign the contract deployment transaction
./sign-tx -key YOUR_PRIVATE_KEY \
  -data 0x6064600055606460015560006000f3 \
  -gas 500000 \
  -gasPrice 1000000000

# Deploy using the generated curl command
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_sendRawTransaction",
    "params": ["0x003b9aca00200000...signed_transaction..."],
    "id": 1
  }'
```

**✅ Expected Response**: `{"jsonrpc":"2.0","result":"0x...transaction_hash...","id":1}`

**Wait 3-5 seconds for automatic block production**, then check:

### Step 2: Check Block Height & Contract Deployment

```bash
# Check current block number (should increase from 0)
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
# Expected: {"jsonrpc":"2.0","result":"0x1","id":1}

# Get block details to see your contract deployment
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBlockByNumber",
    "params": ["0x1", true],
    "id": 1
  }'
```

**Expected**: Block with your transaction and contract creation.

### Step 3: Query Token Contract Storage

The deployed contract stores:

- **Storage slot 0**: Total supply (100 tokens)
- **Storage slot 1**: Deployer balance (100 tokens)

```bash
# Check total supply (storage slot 0)
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getStorageAt",
    "params": [
      "0x59221ccb2e2c66164d141ad9d6a6171bbb157900",
      "0x0000000000000000000000000000000000000000000000000000000000000000",
      "latest"
    ],
    "id": 1
  }'
# Expected: {"jsonrpc":"2.0","result":"0x64","id":1} (0x64 = 100 in hex)

# Check deployer balance (storage slot 1) 
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getStorageAt",
    "params": [
      "0x59221ccb2e2c66164d141ad9d6a6171bbb157900",
      "0x0000000000000000000000000000000000000000000000000000000000000001",
      "latest"
    ],
    "id": 1
  }'
# Expected: {"jsonrpc":"2.0","result":"0x64","id":1} (0x64 = 100 tokens)
```

### Step 4: Query Account ETH Balance

Check the auto-funded account balance (HotStuff auto-funds accounts for demo):

```bash
# Check deployer's ETH balance (should show 1000 ETH from auto-funding)
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBalance",
    "params": ["YOUR_ADDRESS_FROM_SIGN_TX", "latest"],
    "id": 1
  }'
# Expected: Large balance like "0x3635c9adc5dea00000" (1000 ETH in wei)
```

### Step 5: Token Transfer (Advanced)

Create a token transfer transaction to another address:

```bash
# First, create another account to transfer to
./sign-tx -genkey
# Save the recipient address, e.g.: 0x307865613737306566376138663138663035316166303137633336333162633734653230376233366432

# Create transfer transaction with proper ABI encoding
# transfer(address,uint256) = 0xa9059cbb + padded_recipient_address + padded_amount
./sign-tx -key 94051b2c142e9d29c8f32257ad7cbc9b736169b49f65d51ce8f19b3fb7a27fc6 \
  -to 0x59221ccb2e2c66164d141ad9d6a6171bbb157900 \
  -data 0xa9059cbb000000000000000000000000307865613737306566376138663138663035316166303137633336333162633734653230376233366432000000000000000000000000000000000000000000000000000000000000000000000032 \
  -gas 100000 \
  -gasPrice 1000000000

# Submit the transfer transaction using the generated curl command
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_sendRawTransaction",
    "params": ["0x003b9aca00a059221ccb2e2c66164d141ad9d6a6171bbb157900a9059cbb0000000000000000000000003078656137373065663761386631386630353161663031376333363331626337346532303762333664320000000000000000000000000000000000000000000000000000000000000000000000320a95c22d5bc8de2b9552deddf2ed9427d09e5b548876aa0c977ea7e22f8d75081dcd61e685608a563633aeeabe5e3c19fdf313209e6f96956509cf226e0680f87694"],
    "id": 1
  }'
```

**Data Field Breakdown**:

- `0xa9059cbb` - Function selector for `transfer(address,uint256)`
- `000000000000000000000000` - 12 bytes padding
- `307865613737306566376138663138663035316166303137633336333162633734653230376233366432` - Recipient address (20 bytes)
- `000000000000000000000000000000000000000000000000000000000000000032` - Amount: 50 tokens (32 bytes)

**Wait for next block (3 seconds), then verify the transfer worked by checking storage.**

### Block Height Monitoring

Monitor blockchain progress in real-time:

```bash
# Check block height every few seconds
while true; do
  echo "Block: $(curl -s -X POST http://127.0.0.1:8545 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | \
    grep -o '"result":"[^"]*"' | cut -d'"' -f4)"
  sleep 2
done
```

### Token Contract Details

**Bytecode**: `0x6064600055606460015560006000f3`

**What it does**:

1. `60 64` - Push 100 (0x64) onto stack
2. `60 00` - Push storage slot 0 onto stack
3. `55` - SSTORE: Store 100 in slot 0 (total supply)
4. `60 64` - Push 100 onto stack again
5. `60 01` - Push storage slot 1 onto stack  
6. `55` - SSTORE: Store 100 in slot 1 (deployer balance)
7. `60 00 60 00 f3` - Return empty (end constructor)

### Unsigned Transaction (Rejected)

```bash
curl -X POST http://127.0.0.1:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_sendTransaction", 
    "params": [{
      "from": "0x1234567890123456789012345678901234567890",
      "data": "0x6064600055606460015560006000f3",
      "gas": "0x7a120",
      "gasPrice": "0x3b9aca00",
      "value": "0x0"
    }],
    "id": 1
  }'
```

**❌ Response**: `{"jsonrpc":"2.0","error":{"code":-32603,"message":"failed to send transaction","data":"transaction not signed: missing signature fields"},"id":1}`

## 🛠️ **Troubleshooting**

### Common Issues & Solutions

**❌ "Invalid data: encoding/hex: invalid byte: U+0052 'R'"**

```bash
# Problem: Using placeholder text instead of actual hex address
./sign-tx -data 0xa9059cbb000000000000000000000000RECIPIENT_ADDRESS...

# Solution: Replace RECIPIENT_ADDRESS with actual hex address (without 0x)
./sign-tx -data 0xa9059cbb000000000000000000000000307865613737306566376138663138663035316166303137633336333162633734653230376233366432...
```

**❌ "invalid address format"**

```bash
# Problem: Incorrect address length or format
curl ... "params": ["0x123..."]  # Too short

# Solution: Use complete 42-character addresses
curl ... "params": ["0x307865613737306566376138663138663035316166303137633336333162633734653230376233366432"]
```

**❌ "transaction not signed"**

```bash
# Problem: Using eth_sendTransaction (requires signing)
curl ... "method": "eth_sendTransaction"

# Solution: Use eth_sendRawTransaction with signed transaction
curl ... "method": "eth_sendRawTransaction", "params": ["0x...signed_data..."]
```

**❌ Block number stays at 0**

- **Cause**: Transaction might be stuck in mempool
- **Solution**: Wait 5+ seconds for automatic block production, or check transaction pool

**✅ Verify Everything Works**:

```bash
# 1. Check blockchain is running
curl -s -X POST http://127.0.0.1:8545 -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'

# 2. Check block height increases
curl -s -X POST http://127.0.0.1:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 3. Check contract storage
curl -s -X POST http://127.0.0.1:8545 -d '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x59221ccb2e2c66164d141ad9d6a6171bbb157900","0x0","latest"],"id":1}'
```

## 🌐 **JSON-RPC API**

HotStuff implements standard Ethereum JSON-RPC methods:

### Blockchain Queries

- `eth_chainId` - Get chain ID (1337)
- `eth_blockNumber` - Get latest block number
- `eth_getBlockByNumber` - Get block by number
- `eth_getBlockByHash` - Get block by hash

### Account Operations

- `eth_getBalance` - Get account balance
- `eth_getTransactionCount` - Get account nonce
- `eth_getCode` - Get contract code
- `eth_getStorageAt` - Get contract storage

### Transaction Operations

- `eth_sendRawTransaction` - Submit signed transaction ✅
- `eth_sendTransaction` - Submit unsigned transaction ❌ (rejected)
- `eth_getTransactionByHash` - Get transaction details
- `eth_getTransactionReceipt` - Get transaction receipt

### Execution

- `eth_call` - Execute read-only contract call
- `eth_estimateGas` - Estimate gas usage
- `eth_gasPrice` - Get current gas price

## 🎮 **Interactive Demo**

For a complete demonstration with automatic key management:

```bash
go run examples/layer1_demo/main.go
```

This provides:

- ✅ Automatic blockchain startup
- ✅ Key generation and transaction signing
- ✅ ERC-20 token contract deployment
- ✅ Token transfer demonstrations
- ✅ RPC interaction examples

## 🔧 **Development**

### Build Dependencies

- [Go](https://go.dev) (at least version 1.18)
- [BadgerDB](https://github.com/dgraph-io/badger) for persistent storage

### Compile

```bash
# Linux and macOS
make

# Windows
.\build.ps1
```

### Configuration Options

```bash
./hotstuff run [options]

# Essential flags:
--rpc                    # Enable JSON-RPC server
--rpc-addr 127.0.0.1:8545 # RPC server address
--replicas 4             # Number of consensus replicas
--clients 0              # Disable automatic client noise
--duration 300s          # Runtime duration
--persistent             # Enable persistent storage
--data-dir ./data        # Data directory for persistence
```

### Clean Operation (No Client Noise)

```bash
# Recommended for development - clean consensus logs only
./hotstuff run --rpc --rpc-addr 127.0.0.1:8545 --duration 300s --replicas 4 --clients 0
```

## 📊 **Architecture**

### Core Components

- **🔥 Consensus Layer**: HotStuff Byzantine fault tolerance
- **⚡ Execution Layer**: EVM with opcodes (PUSH, SSTORE, CALL, etc.)
- **🗃️ Storage Layer**: Merkle Patricia Trie with BadgerDB
- **💰 Transaction Pool**: Gas price prioritized mempool
- **🌐 RPC Layer**: Ethereum-compatible JSON-RPC
- **🔐 Crypto Layer**: ECDSA signing with EIP-155

### Consensus Implementations

- `chainedhotstuff`: Three-phase pipelined HotStuff
- `fasthotstuff`: Two-chain version with forking protection  
- `simplehotstuff`: Simplified HotStuff variant

### Storage Options

- **In-Memory**: Fast, non-persistent (default)
- **BadgerDB**: Persistent key-value storage with Merkle Patricia Trie

## 🔐 **Security Features**

### Layer 1 Security Model

- **✅ Cryptographic Signatures**: All transactions must be signed
- **✅ EIP-155 Protection**: Chain ID prevents replay attacks
- **✅ ECDSA Verification**: Standard Ethereum signature scheme
- **✅ Gas Price Validation**: Economic spam protection
- **✅ Nonce Enforcement**: Prevents transaction reordering

### Production Security

Unlike development blockchains, HotStuff enforces real security:

- **No unsigned transactions accepted**
- **Proper key management required**
- **Cryptographic transaction verification**
- **Byzantine fault tolerance via consensus**

## 🌍 **MetaMask Integration**

Connect MetaMask to your HotStuff blockchain:

1. **Network Settings**:
   - Network Name: `HotStuff Local`
   - RPC URL: `http://127.0.0.1:8545`
   - Chain ID: `1337`
   - Currency Symbol: `ETH`

2. **Import Private Key** (from `./sign-tx -genkey`)

3. **Start Transacting** with full wallet support!

## 📚 **Research Framework**

HotStuff also serves as a research framework for consensus protocols:

### Experimentation

```bash
# Run distributed experiments
./hotstuff help run

# Generate performance plots
./plot --help
```

### Safety Testing

Implements the Twins strategy for consensus safety testing.
See [twins documentation](docs/twins.md).

### Modular Architecture

- **Consensus**: Pluggable consensus implementations
- **Crypto**: ECDSA and BLS12-381 threshold signatures  
- **Synchronizer**: View synchronization algorithms
- **Blockchain**: Storage backends
- **Networking**: Gorums-based communication

## 📖 **Further Reading**

- [Layer 1 Demo Guide](examples/layer1_demo/README.md) - Complete blockchain usage
- [MetaMask Setup](examples/layer1_demo/METAMASK_GUIDE.md) - Wallet integration
- [Experimentation Guide](docs/experimentation.md) - Research usage
- [Twins Safety Testing](docs/twins.md) - Consensus validation

## 🎯 **Use Cases**

- **🏗️ DApp Development**: Deploy and test smart contracts
- **🔬 Research**: Experiment with consensus protocols
- **📚 Education**: Learn blockchain and consensus mechanisms
- **🚀 Prototyping**: Build Layer 1 blockchain applications
- **🔧 Testing**: Validate Ethereum-compatible systems

---

**Transform your ideas into a working Layer 1 blockchain with HotStuff!** 🚀

## References

[1] M. Yin, D. Malkhi, M. K. Reiter, G. Golan Gueta, and I. Abraham, "HotStuff: BFT Consensus in the Lens of Blockchain," Mar 2018.

[2] Hanish Gogada, Hein Meling, Leander Jehl, and John Ingve Olsen. [An Extensible Framework for Implementing and Validating Byzantine Fault-Tolerant Protocols](https://dl.acm.org/doi/10.1145/3584684.3597266). In ApPLIED 2023.
