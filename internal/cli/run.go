package cli

import (
	"bufio"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/evm"
	"github.com/relab/hotstuff/internal/config"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/profiling"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/metrics"
	"github.com/relab/hotstuff/rpc"
	"github.com/relab/hotstuff/txpool"
	"github.com/relab/iago"
	"github.com/spf13/viper"
)

func runController() {
	cfg, err := config.NewViper()
	checkf("viper config error: %v", err)

	cuePath := viper.GetString("cue")
	if cuePath != "" {
		cfg, err = config.NewCue(cuePath, cfg)
		checkf("config error when loading %s: %v", cuePath, err)
	}

	if cfg.RandomTree {
		tree.Shuffle(cfg.TreePositions)
	}

	// If the config is set to run locally, `hosts` will be nil (empty)
	// and when passed to iago.NewSSHGroup, thus iago will not generate
	// an SSH group.
	var hosts []string
	if !cfg.IsLocal() {
		hosts = cfg.AllHosts()
	}

	g, err := iago.NewSSHGroup(hosts, cfg.SshConfig)
	checkf("failed to connect to remote hosts: %v", err)

	if cfg.Exe == "" {
		cfg.Exe, err = os.Executable()
		checkf("failed to get executable path: %v", err)
	}

	// TODO: Generate DeployConfig from ExperimentConfig type.
	sessions, err := orchestration.Deploy(g, orchestration.DeployConfig{
		ExePath:             cfg.Exe,
		LogLevel:            cfg.LogLevel,
		CPUProfiling:        cfg.CpuProfile,
		MemProfiling:        cfg.MemProfile,
		Tracing:             cfg.Trace,
		Fgprof:              cfg.FgProfProfile,
		Metrics:             cfg.Metrics,
		MeasurementInterval: cfg.MeasurementInterval,
	})
	checkf("failed to deploy workers: %v", err)

	errors := make(chan error)
	remoteWorkers := make(map[string]orchestration.RemoteWorker)
	for host, session := range sessions {
		remoteWorkers[host] = orchestration.NewRemoteWorker(
			protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()),
		)
		go stderrPipe(session.Stderr(), errors)
	}

	if cfg.Worker || len(hosts) == 0 {
		worker, wait := localWorker(cfg.Output, cfg.Metrics, cfg.MeasurementInterval, cfg.Persistent, cfg.DataDir, cfg.RPC, cfg.RPCAddr, cfg.RPCCORS)
		defer wait()
		remoteWorkers["localhost"] = worker
	}

	experiment, err := orchestration.NewExperiment(
		cfg,
		remoteWorkers,
		logging.New("ctrl"),
	)
	checkf("config error: %v", err)

	err = experiment.Run()
	checkf("failed to run experiment: %v", err)

	for _, session := range sessions {
		err := session.Close()
		checkf("failed to close ssh command session: %v", err)
	}
	for range sessions {
		err = <-errors
		checkf("failed to read from remote's standard error stream %v", err)
	}

	err = orchestration.FetchData(g, cfg.Output)
	checkf("failed to fetch data: %v", err)

	err = g.Close()
	checkf("failed to close ssh connections: %v", err)
}

func checkf(format string, args ...any) {
	for _, arg := range args {
		if err, _ := arg.(error); err != nil {
			log.Fatalf(format, args...)
		}
	}
}

func localWorker(globalOutput string, enableMetrics []string, interval time.Duration, persistent bool, dataDir string, rpcEnabled bool, rpcAddr string, rpcCors bool) (worker orchestration.RemoteWorker, wait func()) {
	// set up an output dir
	output := ""
	if globalOutput != "" {
		output = filepath.Join(globalOutput, "local")
		err := os.MkdirAll(output, 0o755)
		checkf("failed to create local output directory: %v", err)
	}

	// start profiling
	stopProfilers, err := startLocalProfiling(output)
	checkf("failed to start local profiling: %v", err)

	// set up a local worker
	controllerPipe, workerPipe := net.Pipe()
	c := make(chan struct{})
	go func() {
		var logger metrics.Logger
		if output != "" {
			f, err := os.OpenFile(filepath.Join(output, "measurements.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			checkf("failed to create output file: %v", err)
			defer func() { checkf("failed to close output file: %v", f.Close()) }()

			wr := bufio.NewWriter(f)
			defer func() { checkf("failed to flush writer: %v", wr.Flush()) }()

			logger, err = metrics.NewJSONLogger(wr)
			checkf("failed to create JSON logger: %v", err)
			defer func() { checkf("failed to close logger: %v", logger.Close()) }()
		} else {
			logger = metrics.NopLogger()
		}

		baseWorker := orchestration.NewWorker(
			protostream.NewWriter(workerPipe),
			protostream.NewReader(workerPipe),
			logger,
			enableMetrics,
			interval,
		)

		// Initialize Layer 1 blockchain components
		var l1Blockchain *blockchain.L1Blockchain
		var rpcServer *rpc.Server

		// Create core blockchain components
		stateDB := evm.NewInMemoryStateDB()
		txPoolConfig := txpool.DefaultConfig()
		signer := txpool.NewEIP155Signer(big.NewInt(1337))
		txPool := txpool.NewTxPool(txPoolConfig, signer)
		executor := evm.NewExecutor(evm.ExecutionConfig{
			GasLimit: 8000000,
			BaseFee:  big.NewInt(1000000000),
			ChainID:  big.NewInt(1337),
		})

		// Create Layer 1 blockchain with automatic transaction processing
		l1Blockchain = blockchain.NewL1Blockchain(blockchain.L1BlockchainConfig{
			StateDB:  stateDB,
			Executor: executor,
			TxPool:   txPool,
		})
		log.Println("Layer 1 blockchain initialized with automatic block production")

		// Start RPC server if enabled (RPC just provides interface, doesn't handle consensus)
		if rpcEnabled {
			// Create RPC service that interfaces with the blockchain
			rpcService := rpc.NewSimpleRPCServiceWithBlockchain(stateDB, executor, txPool, l1Blockchain)
			handler := rpc.NewHandler(rpcService)
			rpcServer = rpc.NewServer(handler, rpcAddr)

			// Start RPC server
			go func() {
				if err := rpcServer.Start(); err != nil {
					log.Printf("RPC server error: %v", err)
				}
			}()
			log.Printf("JSON-RPC server started on %s (interface to Layer 1 blockchain)", rpcAddr)
		}

		var err error
		if persistent {
			// Create data directory for persistent storage
			if err := os.MkdirAll(dataDir, 0755); err != nil {
				log.Fatalf("Failed to create data directory %s: %v", dataDir, err)
			}
			config := orchestration.PersistentWorkerConfig{
				DataDir:       dataDir,
				UsePersistent: true,
			}
			persistentWorker := orchestration.NewPersistentWorker(config, baseWorker)
			err = persistentWorker.Run()
		} else {
			err = baseWorker.Run()
		}

		if err != nil {
			log.Fatal(err)
		}

		// Stop Layer 1 blockchain
		if l1Blockchain != nil {
			l1Blockchain.Close()
		}

		// Stop RPC server if it was started
		if rpcServer != nil {
			rpcServer.Stop()
		}
		close(c)
	}()

	wait = func() {
		<-c
		checkf("failed to stop local profilers: %v", stopProfilers())
	}

	return orchestration.NewRemoteWorker(
		protostream.NewWriter(controllerPipe), protostream.NewReader(controllerPipe),
	), wait
}

func stderrPipe(r io.Reader, errChan chan<- error) {
	_, err := io.Copy(os.Stderr, r)
	errChan <- err
}

func startLocalProfiling(output string) (stop func() error, err error) {
	var (
		cpuProfile    string
		memProfile    string
		trace         string
		fgprofProfile string
	)
	if output == "" {
		return func() error { return nil }, nil
	}
	if viper.GetBool("cpu-profile") {
		cpuProfile = filepath.Join(output, "cpuprofile")
	}
	if viper.GetBool("mem-profile") {
		memProfile = filepath.Join(output, "memprofile")
	}
	if viper.GetBool("trace") {
		trace = filepath.Join(output, "trace")
	}
	if viper.GetBool("fgprof-profile") {
		fgprofProfile = filepath.Join(output, "fgprofprofile")
	}
	stop, err = profiling.StartProfilers(cpuProfile, memProfile, trace, fgprofProfile)
	return
}
