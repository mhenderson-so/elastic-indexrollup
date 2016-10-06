package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"

	elastic "gopkg.in/olivere/elastic.v3"

	"github.com/olekukonko/tablewriter"
)

func consoleOut(format string, a ...interface{}) (int, error) {
	if silent {
		return 0, nil
	}
	return fmt.Fprintf(os.Stderr, format, a...)
}

func printProgressTable(start time.Time, got int, matchingIndexesSorted []string, bulkInserter *elastic.BulkProcessor) bool {
	clearConsole()
	inserterStats := bulkInserter.Stats()
	var anyNotDone bool

	consoleOut("Elapsed: %v\n", time.Since(start))
	workerWord := "workers"
	if runningThreads == 1 {
		workerWord = "worker"
	}

	perSec := float64(got) / time.Since(start).Seconds()

	consoleOut("%v documents read by %v %s (avg %d/sec)\n", got, runningThreads, workerWord, int(perSec))
	consoleOut("%v documents committed to Elastic (%v failed)\n", inserterStats.Indexed, inserterStats.Failed)

	table := tablewriter.NewWriter(os.Stdout)
	tableHeader := []string{
		"Status",
		"Source",
		"Destination",
		"Records",
	}
	table.SetHeader(tableHeader)

	for _, idx := range matchingIndexesSorted {
		thisStat := readDocs[idx]
		if !thisStat.Done {
			anyNotDone = true
		}
		status := "PENDING    "
		if thisStat.ReadCount > 0 {
			status = "IN PROGRESS"

		}
		if thisStat.Done {
			status = "COMPLETE   "
		}
		table.Append([]string{
			status,
			idx,
			thisStat.DestinationIndex,
			fmt.Sprintf("%d", thisStat.ReadCount),
		})
	}

	if !silent {
		table.Render()
	}

	return !anyNotDone
}

func printBenchmarkTable(results benchmarkData, iterations int) {
	var keys benchmarkSets
	for key := range results {
		keys = append(keys, key)
	}
	sort.Sort(keys)

	clearConsole()

	fmt.Println("Running benchmark...")

	table := tablewriter.NewWriter(os.Stdout)
	tableHeader := []string{
		"Threads",
		"Buffers",
		"Average",
	}
	for i := 1; i <= iterations; i++ {
		tableHeader = append(tableHeader, fmt.Sprintf("%d", i))
	}
	table.SetHeader(tableHeader)

	for _, set := range keys {
		result := results[set]
		thisRow := []string{
			fmt.Sprintf("%d", set.Threads),
			fmt.Sprintf("%d", set.Buffers),
			fmt.Sprintf("%v", result.Average),
		}
		for _, t := range result.Results {
			thisRow = append(thisRow, fmt.Sprintf("%v", t))
		}
		table.Append(thisRow)
	}
	table.Render()
}

//http://stackoverflow.com/a/22896706/69683
func clearConsole() {
	if silent {
		return
	}
	clear := make(map[string]func()) //Initialize it
	clear["linux"] = func() {
		cmd := exec.Command("clear") //Linux example, its tested
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
	clear["darwin"] = clear["linux"] //OSX the same as Linux
	clear["windows"] = func() {
		cmd := exec.Command("cls") //Windows example it is untested, but I think its working
		cmd.Stdout = os.Stdout
		cmd.Run()
	}

	value, ok := clear[runtime.GOOS] //runtime.GOOS -> linux, windows, darwin etc.
	if ok {                          //if we defined a clear func for that platform:
		value() //we execute it
	} else { //unsupported platform
		panic("Your platform is unsupported! I can't clear terminal screen :(")
	}
}
