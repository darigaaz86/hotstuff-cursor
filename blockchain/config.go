package blockchain

import (
	"fmt"
	"path/filepath"

	"github.com/relab/hotstuff/modules"
)

// StorageType defines the type of storage backend to use
type StorageType string

const (
	// MemoryStorage uses in-memory maps (existing implementation)
	MemoryStorage StorageType = "memory"
	// BadgerStorage uses BadgerDB for persistent storage
	BadgerStorage StorageType = "badger"
)

// Config contains configuration options for the blockchain storage
type Config struct {
	// StorageType specifies which storage backend to use
	StorageType StorageType

	// DataDir is the directory where persistent data will be stored (for BadgerStorage)
	DataDir string

	// DBName is the name of the database directory (for BadgerStorage)
	DBName string
}

// DefaultConfig returns a default configuration using memory storage
func DefaultConfig() Config {
	return Config{
		StorageType: MemoryStorage,
		DataDir:     "./data",
		DBName:      "hotstuff.db",
	}
}

// NewBlockChain creates a new blockchain instance based on the configuration
func NewBlockChain(config Config) (modules.BlockChain, error) {
	switch config.StorageType {
	case MemoryStorage:
		return New(), nil

	case BadgerStorage:
		dbPath := filepath.Join(config.DataDir, config.DBName)
		return NewBadgerBlockChain(dbPath)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.StorageType)
	}
}

// NewMemoryBlockChain creates a new in-memory blockchain (backward compatibility)
func NewMemoryBlockChain() modules.BlockChain {
	return New()
}

// NewPersistentBlockChain creates a new persistent blockchain using BadgerDB
func NewPersistentBlockChain(dataDir string) (modules.BlockChain, error) {
	config := Config{
		StorageType: BadgerStorage,
		DataDir:     dataDir,
		DBName:      "hotstuff.db",
	}
	return NewBlockChain(config)
}
