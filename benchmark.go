package main

import (
	"time"
)

//Run a benchmark. This will test the thread and buffer options specified in the function.
//It will run the main program a set number of times for each iteration to try and get a.
//accurate reading.
func runBenchmark() {
	//These are our options
	iterations := 3                                      //Number of times to run each benchmark
	threadOptions := []int{1, 2, 3, 4, 5}                //Number of threads to test
	bufferOptions := []int{100, 1000, 2000, 5000, 10000} //Number of buffers to test

	results := make(benchmarkData) //Create our result set which we will print to the screen periodically

	//Pre-create our empty result sets so we can show the full range of options in our
	//output table (ableit with no data initially)
	for _, thisThreads := range threadOptions {
		for _, thisBuffers := range bufferOptions {
			thisSet := benchmarkSet{
				Buffers: thisBuffers,
				Threads: thisThreads,
			}
			results[thisSet] = benchmarkResult{}
		}
	}

	printBenchmarkTable(results, iterations) //Print the first, empty version of our table

	//Here we loop through the original data again, because otherwise we would need to sort our
	//result map and run through it, which is kind of pointless at this stage. We may as well just
	//run it again.
	for _, thisThreads := range threadOptions {
		for _, thisBuffers := range bufferOptions {
			thisSet := benchmarkSet{ //Create a test set identical to what we have in our results map
				Buffers: thisBuffers,
				Threads: thisThreads,
			}
			var thisResults []time.Duration   //Array that will contain the time taken for each iteration
			for i := 0; i < iterations; i++ { //Run through the iterations
				//We need to set some of our globals to match our test parameters
				*threads = thisThreads
				*bufferSize = thisBuffers
				//We need to reset some of our globals that will be maintained from our previous runs
				runningThreads = 0
				lastThread = 0
				readDocs = make(map[string]rollupStat)

				//You can set silent=true here if you do not want to display the individual runs of the benchmarks. I found
				//it nicer to have it off, so that you can see that something is actually happening, rather than long periods
				//of nothingness.

				//silent = true
				start := time.Now()                                  //Start timing
				doMain()                                             //Run the benchmark
				thisResults = append(thisResults, time.Since(start)) //Finish timing
				silent = false

				//Add this interim result to our results so we can print the benchmark table
				results[thisSet] = benchmarkResult{
					Results: thisResults,
				}
				//Show progress to console
				printBenchmarkTable(results, iterations)

			}

			//Once we have run all our iterations, we need to figure out the average duration over all
			//our iterations, and add this to the result set.
			var totalNanos int64
			for _, x := range thisResults {
				totalNanos += x.Nanoseconds()
			}
			averageNanos := totalNanos / int64(iterations)
			results[thisSet] = benchmarkResult{
				Results: thisResults,
				Average: time.Duration(averageNanos),
			}

			//Last but not least, print the result table again. This also means that on the final run, we will have
			//a result table printed instead of the output from the main program (if silent is set to false)
			printBenchmarkTable(results, iterations)
		}
	}
}
