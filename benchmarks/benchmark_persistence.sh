#!/bin/bash

# HotStuff Persistence Throughput Benchmark
# Compares performance between in-memory and persistent storage

set -e

echo "🚀 HotStuff Persistence Throughput Benchmark"
echo "=============================================="

# Configuration
DURATION="10s"
REPLICAS=4
CLIENTS=2
RUNS=3

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Clean up function
cleanup() {
    echo "🧹 Cleaning up test directories..."
    rm -rf ./benchmark_memory_* ./benchmark_persistent_* 2>/dev/null || true
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Build the binary
echo "🔨 Building HotStuff binary..."
make > /dev/null 2>&1

# Function to extract metrics from output
extract_metrics() {
    local output_file="$1"
    local commands_executed=$(grep "Done sending commands" "$output_file" | grep -o "executed: [0-9]*" | cut -d' ' -f2)
    local failed=$(grep "Done sending commands" "$output_file" | grep -o "failed: [0-9]*" | cut -d' ' -f2)
    local timeouts=$(grep "Done sending commands" "$output_file" | grep -o "timeouts: [0-9]*" | cut -d' ' -f2)
    
    echo "$commands_executed,$failed,$timeouts"
}

# Function to run benchmark
run_benchmark() {
    local mode="$1"
    local data_dir="$2"
    local persistent_flag="$3"
    local run_num="$4"
    
    echo "  Run $run_num: $mode mode..."
    
    local output_file="benchmark_${mode}_run${run_num}.log"
    local start_time=$(date +%s.%N)
    
    if [ "$persistent_flag" = "--persistent" ]; then
        ./hotstuff run $persistent_flag --data-dir "$data_dir" --duration "$DURATION" --replicas "$REPLICAS" --clients "$CLIENTS" > "$output_file" 2>&1
    else
        ./hotstuff run --duration "$DURATION" --replicas "$REPLICAS" --clients "$CLIENTS" > "$output_file" 2>&1
    fi
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc -l)
    
    local metrics=$(extract_metrics "$output_file")
    local commands_executed=$(echo "$metrics" | cut -d',' -f1)
    local failed=$(echo "$metrics" | cut -d',' -f2)
    local timeouts=$(echo "$metrics" | cut -d',' -f3)
    
    local throughput=$(echo "scale=2; $commands_executed / $duration" | bc -l)
    
    echo "$commands_executed,$failed,$timeouts,$duration,$throughput"
    
    # Clean up log file
    rm -f "$output_file"
}

# Arrays to store results
declare -a memory_commands=()
declare -a memory_throughput=()
declare -a persistent_commands=()
declare -a persistent_throughput=()
declare -a persistent_db_sizes=()

echo ""
echo "📊 Running benchmarks..."
echo "Configuration: Duration=$DURATION, Replicas=$REPLICAS, Clients=$CLIENTS, Runs=$RUNS"
echo ""

# Run in-memory benchmarks
echo -e "${BLUE}🧠 Testing IN-MEMORY storage...${NC}"
for i in $(seq 1 $RUNS); do
    result=$(run_benchmark "memory" "" "" "$i")
    commands=$(echo "$result" | cut -d',' -f1)
    throughput=$(echo "$result" | cut -d',' -f5)
    memory_commands+=($commands)
    memory_throughput+=($throughput)
    echo "    Commands: $commands, Throughput: ${throughput} cmd/s"
done

echo ""

# Run persistent storage benchmarks
echo -e "${GREEN}🗄️  Testing PERSISTENT storage...${NC}"
for i in $(seq 1 $RUNS); do
    data_dir="./benchmark_persistent_run${i}"
    rm -rf "$data_dir" 2>/dev/null || true
    
    result=$(run_benchmark "persistent" "$data_dir" "--persistent" "$i")
    commands=$(echo "$result" | cut -d',' -f1)
    throughput=$(echo "$result" | cut -d',' -f5)
    persistent_commands+=($commands)
    persistent_throughput+=($throughput)
    
    # Measure database size
    db_size=$(du -sh "${data_dir}"/replica_*/blocks.db 2>/dev/null | head -1 | cut -f1 || echo "0B")
    persistent_db_sizes+=($db_size)
    
    echo "    Commands: $commands, Throughput: ${throughput} cmd/s, DB Size: $db_size"
done

echo ""
echo "📈 BENCHMARK RESULTS"
echo "===================="

# Calculate averages
calc_average() {
    local arr=("$@")
    local sum=0
    local count=${#arr[@]}
    
    for val in "${arr[@]}"; do
        sum=$(echo "$sum + $val" | bc -l)
    done
    
    echo "scale=2; $sum / $count" | bc -l
}

memory_avg_commands=$(calc_average "${memory_commands[@]}")
memory_avg_throughput=$(calc_average "${memory_throughput[@]}")
persistent_avg_commands=$(calc_average "${persistent_commands[@]}")
persistent_avg_throughput=$(calc_average "${persistent_throughput[@]}")

echo ""
echo -e "${BLUE}🧠 IN-MEMORY Results (Average):${NC}"
echo "   Commands Executed: $memory_avg_commands"
echo "   Throughput: $memory_avg_throughput cmd/s"

echo ""
echo -e "${GREEN}🗄️  PERSISTENT Results (Average):${NC}"
echo "   Commands Executed: $persistent_avg_commands"
echo "   Throughput: $persistent_avg_throughput cmd/s"
echo "   Database Size: ${persistent_db_sizes[0]} per replica"

echo ""
echo -e "${YELLOW}📊 PERFORMANCE IMPACT:${NC}"

# Calculate performance impact
throughput_diff=$(echo "scale=2; $memory_avg_throughput - $persistent_avg_throughput" | bc -l)
throughput_percent=$(echo "scale=2; ($throughput_diff / $memory_avg_throughput) * 100" | bc -l)

if (( $(echo "$throughput_diff > 0" | bc -l) )); then
    echo -e "   Throughput Impact: ${RED}-$throughput_diff cmd/s (-${throughput_percent}%)${NC}"
    echo "   → Persistent storage is slower by ${throughput_percent}%"
else
    throughput_diff=$(echo "$throughput_diff * -1" | bc -l)
    throughput_percent=$(echo "$throughput_percent * -1" | bc -l)
    echo -e "   Throughput Impact: ${GREEN}+$throughput_diff cmd/s (+${throughput_percent}%)${NC}"
    echo "   → Persistent storage is faster by ${throughput_percent}%"
fi

# Commands comparison
commands_diff=$(echo "scale=0; $memory_avg_commands - $persistent_avg_commands" | bc -l)
if (( $(echo "$commands_diff > 0" | bc -l) )); then
    echo "   Commands Difference: -$commands_diff commands"
else
    commands_diff=$(echo "$commands_diff * -1" | bc -l)
    echo "   Commands Difference: +$commands_diff commands"
fi

echo ""
echo "🎯 SUMMARY:"
echo "============"
echo "• Persistent storage adds database I/O overhead"
echo "• Each replica stores blocks in separate BadgerDB files"
echo "• Database files persist across restarts (data durability)"
echo "• Trade-off: Slight performance cost for data persistence"

echo ""
echo "✅ Benchmark completed successfully!"
