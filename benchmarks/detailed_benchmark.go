package main

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BenchmarkResult holds the results of a single benchmark run
type BenchmarkResult struct {
	Mode             string
	CommandsExecuted int
	Failed           int
	Timeouts         int
	Duration         time.Duration
	Throughput       float64
	DatabaseSize     string
}

// BenchmarkSuite holds multiple benchmark results
type BenchmarkSuite struct {
	InMemoryResults   []BenchmarkResult
	PersistentResults []BenchmarkResult
}

func main() {
	fmt.Println("🚀 HotStuff Detailed Performance Benchmark")
	fmt.Println("===========================================")

	// Configuration
	duration := "10s"
	replicas := 4
	clients := 2
	runs := 3

	fmt.Printf("📊 Configuration: Duration=%s, Replicas=%d, Clients=%d, Runs=%d\n\n",
		duration, replicas, clients, runs)

	// Build the binary (from parent directory)
	fmt.Println("🔨 Building HotStuff binary...")
	cmd := exec.Command("make")
	cmd.Dir = ".."
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to build: %v", err)
	}

	suite := &BenchmarkSuite{}

	// Run in-memory benchmarks
	fmt.Println("🧠 Testing IN-MEMORY storage...")
	for i := 1; i <= runs; i++ {
		fmt.Printf("  Run %d...", i)
		result, err := runInMemoryBenchmark(duration, replicas, clients, i)
		if err != nil {
			fmt.Printf(" ❌ Error: %v\n", err)
			continue
		}
		suite.InMemoryResults = append(suite.InMemoryResults, result)
		fmt.Printf(" ✅ %d commands (%.0f cmd/s)\n", result.CommandsExecuted, result.Throughput)
	}

	fmt.Println("")

	// Run persistent storage benchmarks
	fmt.Println("🗄️ Testing PERSISTENT storage...")
	for i := 1; i <= runs; i++ {
		fmt.Printf("  Run %d...", i)
		dataDir := fmt.Sprintf("./detailed_bench_persistent_%d", i)

		// Clean up previous run
		exec.Command("rm", "-rf", dataDir).Run()

		result, err := runPersistentBenchmark(duration, replicas, clients, i, dataDir)
		if err != nil {
			fmt.Printf(" ❌ Error: %v\n", err)
			continue
		}
		suite.PersistentResults = append(suite.PersistentResults, result)
		fmt.Printf(" ✅ %d commands (%.0f cmd/s), DB: %s\n",
			result.CommandsExecuted, result.Throughput, result.DatabaseSize)
	}

	// Clean up
	exec.Command("rm", "-rf", "./detailed_bench_persistent_*").Run()

	// Generate detailed report
	generateReport(suite)
}

func runInMemoryBenchmark(duration string, replicas, clients, runNum int) (BenchmarkResult, error) {
	start := time.Now()

	cmd := exec.Command("../hotstuff", "run",
		"--duration", duration,
		"--replicas", strconv.Itoa(replicas),
		"--clients", strconv.Itoa(clients))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return BenchmarkResult{}, fmt.Errorf("command failed: %v", err)
	}

	elapsed := time.Since(start)

	result := BenchmarkResult{
		Mode:     "InMemory",
		Duration: elapsed,
	}

	if err := parseHotStuffOutput(string(output), &result); err != nil {
		return BenchmarkResult{}, err
	}

	result.Throughput = float64(result.CommandsExecuted) / elapsed.Seconds()

	return result, nil
}

func runPersistentBenchmark(duration string, replicas, clients, runNum int, dataDir string) (BenchmarkResult, error) {
	start := time.Now()

	cmd := exec.Command("../hotstuff", "run",
		"--persistent",
		"--data-dir", dataDir,
		"--duration", duration,
		"--replicas", strconv.Itoa(replicas),
		"--clients", strconv.Itoa(clients))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return BenchmarkResult{}, fmt.Errorf("command failed: %v", err)
	}

	elapsed := time.Since(start)

	result := BenchmarkResult{
		Mode:     "Persistent",
		Duration: elapsed,
	}

	if err := parseHotStuffOutput(string(output), &result); err != nil {
		return BenchmarkResult{}, err
	}

	result.Throughput = float64(result.CommandsExecuted) / elapsed.Seconds()

	// Get database size
	dbSizeCmd := exec.Command("du", "-sh", dataDir+"/replica_1/blocks.db")
	sizeOutput, err := dbSizeCmd.Output()
	if err == nil {
		result.DatabaseSize = strings.Fields(string(sizeOutput))[0]
	} else {
		result.DatabaseSize = "Unknown"
	}

	return result, nil
}

func parseHotStuffOutput(output string, result *BenchmarkResult) error {
	// Parse: "Done sending commands (executed: 12345, failed: 0, timeouts: 0)"
	re := regexp.MustCompile(`executed: (\d+), failed: (\d+), timeouts: (\d+)`)
	matches := re.FindStringSubmatch(output)

	if len(matches) != 4 {
		return fmt.Errorf("could not parse command counts from output")
	}

	var err error
	result.CommandsExecuted, err = strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("failed to parse executed commands: %v", err)
	}

	result.Failed, err = strconv.Atoi(matches[2])
	if err != nil {
		return fmt.Errorf("failed to parse failed commands: %v", err)
	}

	result.Timeouts, err = strconv.Atoi(matches[3])
	if err != nil {
		return fmt.Errorf("failed to parse timeout commands: %v", err)
	}

	return nil
}

func calculateAverage(results []BenchmarkResult, field string) float64 {
	if len(results) == 0 {
		return 0
	}

	var sum float64
	for _, result := range results {
		switch field {
		case "commands":
			sum += float64(result.CommandsExecuted)
		case "throughput":
			sum += result.Throughput
		case "duration":
			sum += result.Duration.Seconds()
		}
	}

	return sum / float64(len(results))
}

func generateReport(suite *BenchmarkSuite) {
	fmt.Println("\n📈 DETAILED BENCHMARK RESULTS")
	fmt.Println("==============================")

	if len(suite.InMemoryResults) == 0 || len(suite.PersistentResults) == 0 {
		fmt.Println("❌ Insufficient data for comparison")
		return
	}

	// Calculate averages
	memoryAvgCommands := calculateAverage(suite.InMemoryResults, "commands")
	memoryAvgThroughput := calculateAverage(suite.InMemoryResults, "throughput")
	memoryAvgDuration := calculateAverage(suite.InMemoryResults, "duration")

	persistentAvgCommands := calculateAverage(suite.PersistentResults, "commands")
	persistentAvgThroughput := calculateAverage(suite.PersistentResults, "throughput")
	persistentAvgDuration := calculateAverage(suite.PersistentResults, "duration")

	fmt.Printf("\n🧠 IN-MEMORY STORAGE:\n")
	fmt.Printf("   Average Commands:  %.0f\n", memoryAvgCommands)
	fmt.Printf("   Average Throughput: %.2f cmd/s\n", memoryAvgThroughput)
	fmt.Printf("   Average Duration:  %.2f seconds\n", memoryAvgDuration)

	fmt.Printf("\n🗄️ PERSISTENT STORAGE:\n")
	fmt.Printf("   Average Commands:  %.0f\n", persistentAvgCommands)
	fmt.Printf("   Average Throughput: %.2f cmd/s\n", persistentAvgThroughput)
	fmt.Printf("   Average Duration:  %.2f seconds\n", persistentAvgDuration)
	if len(suite.PersistentResults) > 0 {
		fmt.Printf("   Database Size:     %s per replica\n", suite.PersistentResults[0].DatabaseSize)
	}

	// Performance impact analysis
	commandsDiff := memoryAvgCommands - persistentAvgCommands
	commandsPercent := (commandsDiff / memoryAvgCommands) * 100

	throughputDiff := memoryAvgThroughput - persistentAvgThroughput
	throughputPercent := (throughputDiff / memoryAvgThroughput) * 100

	fmt.Printf("\n📊 PERFORMANCE IMPACT:\n")
	fmt.Printf("   Commands Impact:   %.0f commands (%.1f%% reduction)\n", commandsDiff, commandsPercent)
	fmt.Printf("   Throughput Impact: %.2f cmd/s (%.1f%% reduction)\n", throughputDiff, throughputPercent)

	// Detailed breakdown
	fmt.Printf("\n📋 DETAILED BREAKDOWN:\n")
	fmt.Println("   Run | Mode       | Commands | Throughput | Duration")
	fmt.Println("   ----+------------+----------+------------+---------")

	for i, result := range suite.InMemoryResults {
		fmt.Printf("   %3d | %-10s | %8d | %8.2f   | %6.2fs\n",
			i+1, "Memory", result.CommandsExecuted, result.Throughput, result.Duration.Seconds())
	}

	for i, result := range suite.PersistentResults {
		fmt.Printf("   %3d | %-10s | %8d | %8.2f   | %6.2fs\n",
			i+1, "Persistent", result.CommandsExecuted, result.Throughput, result.Duration.Seconds())
	}

	fmt.Printf("\n💡 ANALYSIS:\n")
	fmt.Printf("   • BadgerDB adds I/O overhead: %.1f%% throughput reduction\n", throughputPercent)
	fmt.Printf("   • Persistent storage provides crash recovery and data durability\n")
	fmt.Printf("   • Trade-off consideration: Performance vs Data Safety\n")

	if throughputPercent < 10 {
		fmt.Printf("   • ✅ Low impact: Excellent choice for production use\n")
	} else if throughputPercent < 25 {
		fmt.Printf("   • ⚠️ Moderate impact: Consider for production based on requirements\n")
	} else {
		fmt.Printf("   • ⚠️ High impact: Evaluate if durability benefits justify cost\n")
	}

	fmt.Println("\n✅ Detailed benchmark completed!")
}
