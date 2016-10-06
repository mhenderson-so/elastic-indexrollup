package main

import (
	"time"
)

func runBenchmark() {
	iterations := 3
	threadOptions := []int{1, 2, 3, 4, 5}
	bufferOptions := []int{100, 1000, 2000, 5000, 10000}

	results := make(benchmarkData)

	for _, thisThreads := range threadOptions {
		for _, thisBuffers := range bufferOptions {
			thisSet := benchmarkSet{
				Buffers: thisBuffers,
				Threads: thisThreads,
			}
			results[thisSet] = benchmarkResult{}
		}
	}

	printBenchmarkTable(results, iterations)

	for _, thisThreads := range threadOptions {
		for _, thisBuffers := range bufferOptions {
			thisSet := benchmarkSet{
				Buffers: thisBuffers,
				Threads: thisThreads,
			}
			var thisResults []time.Duration
			for i := 0; i < iterations; i++ {
				*threads = thisThreads
				*bufferSize = thisBuffers
				runningThreads = 0
				lastThread = 0
				readDocs = make(map[string]rollupStat)

				//silent = true
				start := time.Now()
				doMain()
				thisResults = append(thisResults, time.Since(start))
				//silent = false

				results[thisSet] = benchmarkResult{
					Results: thisResults,
				}

				printBenchmarkTable(results, iterations)

			}

			var totalNanos int64
			for _, x := range thisResults {
				totalNanos += x.Nanoseconds()
			}
			averageNanos := totalNanos / int64(iterations)
			results[thisSet] = benchmarkResult{
				Results: thisResults,
				Average: time.Duration(averageNanos),
			}
			printBenchmarkTable(results, iterations)
		}
	}

	printBenchmarkTable(results, iterations)
}
