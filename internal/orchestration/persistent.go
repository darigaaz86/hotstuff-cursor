package orchestration

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/byzantine"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/eventloop"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/replica"
	"github.com/relab/hotstuff/synchronizer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// PersistentWorkerConfig contains configuration for persistent storage
type PersistentWorkerConfig struct {
	DataDir       string // Base directory for all persistent data
	UsePersistent bool   // Whether to use persistent storage
}

// NewPersistentWorker creates a worker with persistent storage support
func NewPersistentWorker(config PersistentWorkerConfig, worker Worker) *PersistentWorker {
	return &PersistentWorker{
		Worker: worker,
		config: config,
	}
}

// PersistentWorker extends Worker with persistent storage capabilities
type PersistentWorker struct {
	Worker
	config PersistentWorkerConfig
}

// createReplicaWithPersistence creates a replica with optional persistent storage
func (w *PersistentWorker) createReplicaWithPersistence(opts *orchestrationpb.ReplicaOpts) (*replica.Replica, error) {
	// If persistence is disabled, use the original method
	if !w.config.UsePersistent {
		return w.createReplica(opts)
	}

	w.metricsLogger.Log(opts)
	logger := logging.New("hs" + strconv.Itoa(int(opts.GetID())))

	// Create data directory for this replica
	replicaDataDir := filepath.Join(w.config.DataDir, fmt.Sprintf("replica_%d", opts.GetID()))

	// get private key and certificates (same as original)
	privKey, err := keygen.ParsePrivateKey(opts.GetPrivateKey())
	if err != nil {
		return nil, err
	}
	var certificate tls.Certificate
	var rootCAs *x509.CertPool
	if opts.GetUseTLS() {
		certificate, err = tls.X509KeyPair(opts.GetCertificate(), opts.GetCertificateKey())
		if err != nil {
			return nil, err
		}
		rootCAs = x509.NewCertPool()
		rootCAs.AppendCertsFromPEM(opts.GetCertificateAuthority())
	}

	// prepare modules
	builder := modules.NewBuilder(hotstuff.ID(opts.GetID()), privKey)

	// For now, use non-persistent consensus rules to focus on blockchain persistence
	consensusRules, ok := modules.GetModule[consensus.Rules](opts.GetConsensus())
	if !ok {
		return nil, fmt.Errorf("invalid consensus name: '%s'", opts.GetConsensus())
	}

	strategy := opts.GetByzantineStrategy()
	if strategy != "" {
		if byz, ok := modules.GetModule[byzantine.Byzantine](strategy); ok {
			consensusRules = byz.Wrap(consensusRules)
			logger.Infof("assigned byzantine strategy: %s", strategy)
		} else {
			return nil, fmt.Errorf("invalid byzantine strategy: '%s'", opts.GetByzantineStrategy())
		}
	}

	cryptoImpl, ok := modules.GetModule[modules.CryptoBase](opts.GetCrypto())
	if !ok {
		return nil, fmt.Errorf("invalid crypto name: '%s'", opts.GetCrypto())
	}

	leaderRotation, ok := modules.GetModule[modules.LeaderRotation](opts.GetLeaderRotation())
	if !ok {
		return nil, fmt.Errorf("invalid leader-rotation algorithm: '%s'", opts.GetLeaderRotation())
	}

	// Create non-persistent synchronizer for now
	var viewDuration synchronizer.ViewDuration
	if opts.GetLeaderRotation() == "tree-leader" {
		opts.SetTreeHeightWaitTime()
		builder.Options().SetTree(createTree(opts))
		viewDuration = synchronizer.NewFixedViewDuration(opts.GetInitialTimeout().AsDuration())
	} else {
		viewDuration = synchronizer.NewViewDuration(
			uint64(opts.GetTimeoutSamples()),
			float64(opts.GetInitialTimeout().AsDuration().Nanoseconds())/float64(time.Millisecond),
			float64(opts.GetMaxTimeout().AsDuration().Nanoseconds())/float64(time.Millisecond),
			float64(opts.GetTimeoutMultiplier()),
		)
	}
	sync := synchronizer.New(viewDuration)

	// Create persistent blockchain
	blockchainConfig := blockchain.Config{
		StorageType: blockchain.BadgerStorage,
		DataDir:     replicaDataDir,
		DBName:      "blocks.db",
	}
	persistentBlockchain, err := blockchain.NewBlockChain(blockchainConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistent blockchain: %w", err)
	}

	builder.Add(
		eventloop.New(1000),
		consensus.New(consensusRules),
		consensus.NewVotingMachine(),
		crypto.NewCache(cryptoImpl, 100),
		leaderRotation,
		sync,
		w.metricsLogger,
		persistentBlockchain,
		logger,
	)

	builder.Options().SetSharedRandomSeed(opts.GetSharedSeed())

	if w.measurementInterval > 0 {
		replicaMetrics := metrics.GetReplicaMetrics(w.metrics...)
		builder.Add(replicaMetrics...)
		builder.Add(metrics.NewTicker(w.measurementInterval))
	}

	for _, n := range opts.GetModules() {
		m, ok := modules.GetModuleUntyped(n)
		if !ok {
			return nil, fmt.Errorf("no module named '%s'", n)
		}
		builder.Add(m)
	}

	c := replica.Config{
		ID:          hotstuff.ID(opts.GetID()),
		PrivateKey:  privKey,
		TLS:         opts.GetUseTLS(),
		Certificate: &certificate,
		RootCAs:     rootCAs,
		Locations:   opts.GetLocations(),
		BatchSize:   opts.GetBatchSize(),
		ManagerOptions: []gorums.ManagerOption{
			gorums.WithDialTimeout(opts.GetConnectTimeout().AsDuration()),
		},
	}

	logger.Infof("Created replica with persistent storage in: %s", replicaDataDir)
	return replica.New(c, builder), nil
}

// Override createReplicas to use persistent storage
func (w *PersistentWorker) createReplicas(req *orchestrationpb.CreateReplicaRequest) (*orchestrationpb.CreateReplicaResponse, error) {
	resp := &orchestrationpb.CreateReplicaResponse{Replicas: make(map[uint32]*orchestrationpb.ReplicaInfo)}
	for _, cfg := range req.GetReplicas() {
		r, err := w.createReplicaWithPersistence(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create replica: %w", err)
		}

		// set up listeners and get the ports (same as original)
		replicaListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		replicaPort, err := getPort(replicaListener)
		if err != nil {
			return nil, err
		}
		clientListener, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		clientPort, err := getPort(clientListener)
		if err != nil {
			return nil, err
		}

		r.StartServers(replicaListener, clientListener)
		w.replicas[hotstuff.ID(cfg.GetID())] = r

		resp.Replicas[cfg.GetID()] = &orchestrationpb.ReplicaInfo{
			ID:          cfg.GetID(),
			PublicKey:   cfg.GetPublicKey(),
			ReplicaPort: replicaPort,
			ClientPort:  clientPort,
		}
	}
	return resp, nil
}

// Run overrides the Worker's Run method to handle persistent replicas
func (w *PersistentWorker) Run() error {
	for {
		msg, err := w.recv.ReadAny()
		if err != nil {
			return err
		}

		var res proto.Message
		switch req := msg.(type) {
		case *orchestrationpb.CreateReplicaRequest:
			res, err = w.createReplicas(req) // Use overridden method
		case *orchestrationpb.StartReplicaRequest:
			res, err = w.startReplicas(req)
		case *orchestrationpb.StopReplicaRequest:
			res, err = w.stopReplicas(req)
		case *orchestrationpb.StartClientRequest:
			res, err = w.startClients(req)
		case *orchestrationpb.StopClientRequest:
			res, err = w.stopClients(req)
		case *orchestrationpb.QuitRequest:
			return nil
		}

		if err != nil {
			s, _ := status.FromError(err)
			res = s.Proto()
		}

		err = w.send.WriteAny(res)
		if err != nil {
			return err
		}
	}
}
