# HotStuff with Persistent Storage

A Byzantine Fault Tolerant consensus protocol with **persistent storage** using BadgerDB.

## 🆕 New Features

- **Persistent Storage**: BadgerDB integration for crash recovery
- **Command-line flags**: `--persistent` and `--data-dir`
- **Performance benchmarks**: ~20% overhead for data durability
- **Backward compatible**: Default in-memory behavior preserved

## 🚀 Quick Start

```bash
# Build
make

# In-memory (original)
./hotstuff run --duration 10s --replicas 4

# Persistent (new)
./hotstuff run --persistent --data-dir ./data --duration 10s --replicas 4
```

## 📊 Performance

| Storage | Throughput | Impact |
|---------|------------|--------|
| In-Memory | 518 cmd/s | Baseline |
| Persistent | 411 cmd/s | -20.7% |

## 📁 Key Files

- `blockchain/badgerstore.go` - BadgerDB implementation
- `blockchain/statestore.go` - State persistence
- `PERSISTENCE_METRICS.md` - Detailed performance analysis
- `benchmarks/` - Performance measurement tools

## 🧪 Testing

```bash
go test ./blockchain/... -v
cd benchmarks && ./simple_benchmark.sh
```

**Enterprise-grade BFT consensus with data durability!**
