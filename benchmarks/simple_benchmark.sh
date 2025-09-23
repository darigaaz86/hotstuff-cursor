#!/bin/bash

# Simple HotStuff Persistence Throughput Benchmark

set -e

echo "🚀 HotStuff Persistence Throughput Benchmark"
echo "=============================================="

# Configuration
DURATION="8s"
REPLICAS=3
CLIENTS=1
RUNS=3

echo "📊 Configuration: Duration=$DURATION, Replicas=$REPLICAS, Clients=$CLIENTS, Runs=$RUNS"
echo ""

# Clean up function
cleanup() {
    rm -rf ./bench_memory_* ./bench_persistent_* 2>/dev/null || true
}
trap cleanup EXIT

# Build the binary
echo "🔨 Building HotStuff binary..."
make > /dev/null 2>&1

# Arrays to store results
memory_results=()
persistent_results=()

echo "🧠 Testing IN-MEMORY storage..."
for i in $(seq 1 $RUNS); do
    echo "  Run $i..."
    
    # Capture the output and extract the command count
    output=$(./hotstuff run --duration "$DURATION" --replicas "$REPLICAS" --clients "$CLIENTS" 2>&1)
    commands=$(echo "$output" | grep "Done sending commands" | grep -o "executed: [0-9]*" | cut -d' ' -f2)
    
    if [ -n "$commands" ]; then
        memory_results+=($commands)
        echo "    Commands executed: $commands"
    else
        echo "    Error: Could not extract command count"
        memory_results+=(0)
    fi
done

echo ""
echo "🗄️ Testing PERSISTENT storage..."
for i in $(seq 1 $RUNS); do
    echo "  Run $i..."
    
    data_dir="./bench_persistent_run${i}"
    rm -rf "$data_dir" 2>/dev/null || true
    
    # Capture the output and extract the command count
    output=$(./hotstuff run --persistent --data-dir "$data_dir" --duration "$DURATION" --replicas "$REPLICAS" --clients "$CLIENTS" 2>&1)
    commands=$(echo "$output" | grep "Done sending commands" | grep -o "executed: [0-9]*" | cut -d' ' -f2)
    
    if [ -n "$commands" ]; then
        persistent_results+=($commands)
        
        # Get database size
        db_size=$(du -sh "${data_dir}"/replica_*/blocks.db 2>/dev/null | head -1 | cut -f1 || echo "Unknown")
        echo "    Commands executed: $commands, DB size: $db_size"
    else
        echo "    Error: Could not extract command count"
        persistent_results+=(0)
    fi
done

echo ""
echo "📈 RESULTS SUMMARY"
echo "=================="

# Calculate averages using awk (more portable than bc)
calc_avg() {
    local sum=0
    local count=0
    for val in "$@"; do
        if [ "$val" -gt 0 ] 2>/dev/null; then
            sum=$((sum + val))
            count=$((count + 1))
        fi
    done
    if [ $count -gt 0 ]; then
        echo $((sum / count))
    else
        echo 0
    fi
}

memory_avg=$(calc_avg "${memory_results[@]}")
persistent_avg=$(calc_avg "${persistent_results[@]}")

echo "🧠 IN-MEMORY Average: $memory_avg commands"
echo "🗄️ PERSISTENT Average: $persistent_avg commands"

if [ "$memory_avg" -gt 0 ] && [ "$persistent_avg" -gt 0 ]; then
    # Calculate percentage difference
    diff=$((memory_avg - persistent_avg))
    percent=$((diff * 100 / memory_avg))
    
    echo ""
    echo "📊 PERFORMANCE IMPACT:"
    if [ $diff -gt 0 ]; then
        echo "   Persistent storage is ${percent}% slower"
        echo "   Difference: -$diff commands"
    else
        diff=$((-diff))
        percent=$((-percent))
        echo "   Persistent storage is ${percent}% faster"
        echo "   Difference: +$diff commands"
    fi
    
    echo ""
    echo "💡 ANALYSIS:"
    echo "   • Persistent storage adds BadgerDB I/O overhead"
    echo "   • Database files provide crash recovery"
    echo "   • Trade-off: Durability vs Performance"
else
    echo "⚠️ Could not calculate performance impact due to missing data"
fi

echo ""
echo "✅ Benchmark completed!"
