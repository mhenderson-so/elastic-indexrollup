package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	elastic "gopkg.in/olivere/elastic.v3"
)

var (
	inputFilter   = flag.String("infilter", "", "A regex to match against index names")
	inputPattern  = flag.String("inpattern", "", "The output pattern for indexing the read data, in the Go time format (https://golang.org/pkg/time/#Parse)")
	outputPattern = flag.String("outpattern", "", "The output pattern for indexing the read data, in the Go time format (https://golang.org/pkg/time/#Parse)")
	inputHost     = flag.String("inhost", "http://localhost:9200", "ElasticSearch host to read indexes from.")
	outputHost    = flag.String("outhost", "", "(optional) ElasticSearch host to write indexes to. If blank, uses the inhost option")
	threads       = flag.Int("threads", 3, "Number of worker threads to process. Each thread will process one day at a time.")
	bufferSize    = flag.Int("buffersize", 1000, "Number of records to insert at any given time")
	benchmark     = flag.Bool("benchmark", false, "Run benchmarks with different sized threads and buffers")

	silent = false

	runningThreads = 0
	lastThread     = 0
	readDocs       = make(map[string]rollupStat)

	delay = time.Second

	runningMutex sync.Mutex
	readMutex    sync.Mutex
)

func doMain() int {
	//Standard flag validation
	if *inputFilter == "" {
		fmt.Println("Input filter (infilter) cannot be blank")
		return 1
	}
	inputPatternRegex, err := regexp.Compile(*inputFilter)
	if err != nil {
		fmt.Println("Input filter could not be compiled to a regex:", err)
		return 1
	}
	if *inputPattern == "" {
		fmt.Println("Input pattern (inpattern) cannot be blank")
		return 1
	}

	if *outputPattern == "" {
		fmt.Println("Output pattern (outpattern) cannot be blank")
		return 1
	}
	if *inputHost == "" {
		fmt.Println("Input host (inhost) cannot be blank")
		return 1
	}
	if *outputHost == "" { //Output host defaults to input host if not specified
		outputHost = inputHost
	}
	if *threads == 0 {
		fmt.Println("Thread count (threads) must be above zero")
		return 1
	}

	consoleOut("Creating read client...")
	inClient, err := elastic.NewSimpleClient(elastic.SetURL(*inputHost)) //Simple client for scrolling through read data
	if err != nil {
		fmt.Println(err)
		return 1
	}
	consoleOut("Done\n")

	consoleOut("Creating write client...")
	var outClient *elastic.Client
	outClient, err = elastic.NewSimpleClient(elastic.SetURL(*outputHost)) //This client is used for the bulk processor
	if err != nil {
		fmt.Println(err)
		return 1
	}
	consoleOut("Done\n")

	consoleOut("Creating bulk inserter...")
	bulkInserter, err := outClient.BulkProcessor(). //This is our bulk processing service which will just accept docs and do the rest on its own
							Name("RollupInserter").   //Random name for the processor
							Workers(2).               //Number of processor workers. Haven't played around with this to see if it makes any difference
							BulkActions(*bufferSize). //Buffer x records as specified by command flags
							Stats(true).              //Collect stats
							Do()                      //Go
	if err != nil {
		fmt.Println(err)
		return 1
	}
	consoleOut("Done\n")

	//Find the indexes we need to roll up, based on the regex supplied on the command line
	consoleOut("Matching indexes...")
	matchingIndexes, err := getElasticIndexes(inClient, inputPatternRegex)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	var matchingIndexesSorted []string
	for k := range matchingIndexes {
		matchingIndexesSorted = append(matchingIndexesSorted, k)
	}
	sort.Strings(matchingIndexesSorted)
	consoleOut("Done\n")

	consoleOut("Setting up readers...")
	var allRead bool //This bool controls whether we keep our channels open and keep waiting for data
	foundDocs := make(chan insertDoc)
	for i, inIdxName := range matchingIndexesSorted {
		outIdxName := matchingIndexes[inIdxName].Format(*outputPattern)
		//todo (mhenderson): This probably doesn't need to be channeled, because we are just throwing data
		//                   into our bulk processing service. Originally this was a bit more complicated,
		//                   which is why the channels are here. And they just sort of got left over.
		go rollupIndex(i+1, foundDocs, inClient, outClient, inIdxName, outIdxName)
	}
	consoleOut("Done\n")

	next := time.After(delay)
	got := 0
	start := time.Now()

	for !allRead {
		select {
		case <-next:
			allRead = printProgressTable(start, got, matchingIndexesSorted, bulkInserter)
			next = time.After(delay)
		case r := <-foundDocs:
			//See previous todo, this channel probably doesn't need to exist
			got++
			p := elastic.NewBulkIndexRequest(). //Index the document
								Index(r.DestinationIndex). //Destination index
								Type(r.Doc.Type).          //Document type
								Id(r.Doc.Id).              //Document ID to prevent doubleups
								Doc(r.Doc.Source)          //Original JSON document
			bulkInserter.Add(p)
		}
	}

	//Print the final debug statements
	consoleOut("Flushing final records...")
	bulkInserter.Flush()
	consoleOut("Done\n")
	consoleOut("Closing inserter...")
	bulkInserter.Close()
	consoleOut("Done\n")

	//Show the final stats
	stats := bulkInserter.Stats()
	consoleOut("Number of times flush has been invoked: %d\n", stats.Flushed)
	consoleOut("Number of times workers committed reqs: %d\n", stats.Committed)
	consoleOut("Number of requests indexed            : %d\n", stats.Indexed)
	consoleOut("Number of requests reported as created: %d\n", stats.Created)
	consoleOut("Number of requests reported as updated: %d\n", stats.Updated)
	consoleOut("Number of requests reported as success: %d\n", stats.Succeeded)
	consoleOut("Number of requests reported as failed : %d\n", stats.Failed)
	consoleOut("Total time elapsed: %v\n", time.Since(start))

	return 0
}

func getElasticIndexes(client *elastic.Client, indexRegex *regexp.Regexp) (elasticDailyIndexes, error) {
	filteredIndexes := make(elasticDailyIndexes) //make our map of filtered indexes
	allIndexes, err := client.IndexNames()       //fetch all indexes from elastic server
	if err != nil {
		return filteredIndexes, err //At this stage, filteredIndexes is empty so we can return it with the error
	}
	for _, idx := range allIndexes { //We need to filter our indexes to only those that match the pattern provided
		if indexRegex.MatchString(idx) { //If we have a matching pattern
			thisIndexDate, err := time.Parse(*inputPattern, idx) //Decode the date
			if err == nil {
				filteredIndexes[idx] = thisIndexDate //Add this pattern to our map
			}
		}
	}
	return filteredIndexes, nil //Return all the matched patterns
}

//This is our really basic thread scheduling function. It checks two things:
// - Are we at our limit of threads to be running?
// - Was the last thread to be run the one before this one?
//It's simple and not the best, but it's good enough for this one off task.
func okToStart(threadNo int) bool {
	runningMutex.Lock()
	freeThreads := runningThreads < *threads //Are we at our limit of threads?
	myTurn := lastThread == threadNo-1       //Was the last thread run the one before this thread?
	ok := freeThreads && myTurn
	if ok { //If all checks out OK, then increase the running thread counter and set the last thread to this one
		runningThreads++
		lastThread = threadNo
	}
	runningMutex.Unlock()
	return ok
}

func rollupIndex(threadNo int, c chan<- insertDoc, inClient, outClient *elastic.Client, inIndex, outIndex string) error {
	countUpdate := 100
	i := 0

	for !okToStart(threadNo) {
		time.Sleep(100 * time.Millisecond)
	}

	readMutex.Lock()
	readDocs[inIndex] = rollupStat{
		DestinationIndex: outIndex,
		ReadCount:        0,
		Done:             false,
	}
	readMutex.Unlock()

	defer func() {
		runningMutex.Lock()
		runningThreads--
		runningMutex.Unlock()
		readMutex.Lock()
		docStat := readDocs[inIndex]
		docStat.Done = true
		docStat.ReadCount = i
		readDocs[inIndex] = docStat
		readMutex.Unlock()
	}()

	scroll := inClient.Scroll(inIndex).Size(*bufferSize)
	for {
		results, err := scroll.Do()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		for _, doc := range results.Hits.Hits {
			i++
			c <- insertDoc{
				DestinationIndex: outIndex,
				Doc:              doc,
			}

			if i%countUpdate == 0 {
				readMutex.Lock()
				docStat := readDocs[inIndex]
				docStat.ReadCount += countUpdate
				readDocs[inIndex] = docStat
				readMutex.Unlock()
			}
		}
	}
}

func main() {
	flag.Parse()
	if *benchmark {
		runBenchmark()
	} else {
		os.Exit(doMain()) //Exit with the proper code, but this maintains the defers that you don't get running in main()
	}
}
