# HotStuff Performance Benchmarks

This directory contains benchmarking tools to measure the performance impact of persistent storage in HotStuff.

## Benchmark Scripts

### 1. Simple Benchmark (`simple_benchmark.sh`)

A quick shell script to compare in-memory vs persistent storage throughput.

```bash
./simple_benchmark.sh
```

### 2. Detailed Benchmark (`detailed_benchmark.go`)

A comprehensive Go program that provides detailed metrics and analysis.

```bash
go run detailed_benchmark.go
```

### 3. Advanced Benchmark (`benchmark_persistence.sh`)

A full-featured benchmark with extensive metrics (currently being refined).

## Results

The benchmarks measure:

- Commands executed per run
- Throughput (commands/second)
- Database size impact
- Performance overhead percentage

Typical results show ~20% throughput reduction when using persistent storage, which is a reasonable trade-off for data durability.

## Usage

Run from the benchmarks directory:

```bash
cd benchmarks
./simple_benchmark.sh
```

All benchmarks automatically build the HotStuff binary and clean up after themselves.
