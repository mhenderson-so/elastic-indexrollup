package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
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

	runningThreads = 0
	readDocs       = make(map[string]rollupStat)
	sentDocs       = make(map[string]int)

	runningMutex sync.Mutex
	readMutex    sync.Mutex
)

func doMain() int {
	flag.Parse()
	if *inputFilter == "" {
		fmt.Println("Input filter (infilter) cannot be blank")
		return 1
	}
	inputPatternRegex, err := regexp.Compile(*inputFilter)
	if err != nil {
		fmt.Println("Input pattern could not be compiled to a regex:", err)
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
	if *outputHost == "" {
		outputHost = inputHost
	}
	if *threads == 0 {
		fmt.Println("Thread count (threads) must be above zero")
		return 1
	}

	fmt.Fprintf(os.Stderr, "Creating read client...")
	inClient, err := elastic.NewSimpleClient(elastic.SetURL(*inputHost))
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "Done\n")

	fmt.Fprintf(os.Stderr, "Creating write client...")
	var outClient *elastic.Client
	outClient, err = elastic.NewSimpleClient(elastic.SetURL(*outputHost))
	if err != nil {
		// Handle error
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "Done\n")

	fmt.Fprintf(os.Stderr, "Creating bulk inserter...")
	bulkInserter, err := outClient.BulkProcessor().
		Name("RollupInserter").
		Workers(2).
		BulkActions(*bufferSize).
		Stats(true).
		Do()
	if err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Done\n")

	fmt.Fprintf(os.Stderr, "Matching indexes...")
	matchingIndexes, err := getElasticIndexes(inClient, inputPatternRegex)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Done\n")

	fmt.Fprintf(os.Stderr, "Setting up channels...")
	foundDocs := make(chan insertDoc)
	var wg sync.WaitGroup
	wg.Add(len(matchingIndexes))
	fmt.Fprintf(os.Stderr, "Done\n")

	var allRead bool

	fmt.Fprintf(os.Stderr, "Setting up readers...")

	go func() {
		defer close(foundDocs)
		for inIdxName, inIdxDate := range matchingIndexes {
			outIdxName := inIdxDate.Format(*outputPattern)
			go rollupIndex(foundDocs, inClient, outClient, inIdxName, outIdxName)
		}
		wg.Wait()
		allRead = true
	}()
	fmt.Fprintf(os.Stderr, "Done\n")

	delay := time.Second
	next := time.After(delay)
	got := 0
	start := time.Now()

	for !allRead {
		select {
		case <-next:
			clearConsole()
			inserterStats := bulkInserter.Stats()
			var anyNotDone bool

			fmt.Fprintf(os.Stderr, "Elapsed: %v\n", time.Since(start))
			fmt.Fprintf(os.Stderr, "%v documents read by %v workers\n", got, runningThreads)
			fmt.Fprintf(os.Stderr, "%v documents flushed to Elastic (%v failed)\n", inserterStats.Indexed, inserterStats.Failed)
			var keys []string
			for k := range readDocs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, idx := range keys {
				thisStat := readDocs[idx]
				leadIn := "  "
				if thisStat.Done {
					leadIn = " *"
				} else {
					anyNotDone = true
				}
				fmt.Fprintf(os.Stderr, "%s %s -> %s: %v\n", leadIn, idx, thisStat.DestinationIndex, thisStat.ReadCount)
			}

			allRead = !anyNotDone
			next = time.After(delay)
		case r := <-foundDocs:
			got++
			p := elastic.NewBulkIndexRequest().Index(r.DestinationIndex).Type(r.Doc.Type).Id(r.Doc.Id).Doc(r.Doc.Source)
			bulkInserter.Add(p)
		}
	}
	fmt.Fprintf(os.Stderr, "Flushing final records...")
	bulkInserter.Flush()
	fmt.Fprintf(os.Stderr, "Done\n")
	fmt.Fprintf(os.Stderr, "Closing inserter...")
	bulkInserter.Close()
	fmt.Fprintf(os.Stderr, "Done\n")

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

func rollupIndex(c chan<- insertDoc, inClient, outClient *elastic.Client, inIndex, outIndex string) error {
	countUpdate := 100
	i := 0

	runningMutex.Lock()
	runningThreads++
	runningMutex.Unlock()
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

//http://stackoverflow.com/a/22896706/69683
func clearConsole() {
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

func main() {
	os.Exit(doMain()) //Exit with the proper code, but this maintains the defers that you don't get running in main()
}

type elasticDailyIndexes map[string]time.Time
type insertDoc struct {
	DestinationIndex string
	Doc              *elastic.SearchHit
}
type rollupStat struct {
	DestinationIndex string
	ReadCount        int
	Done             bool
}
