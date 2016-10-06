# elastic-indexrollup
Small utility for rolling up ElasticSearch indexes into a different pattern.

Typical usage would be to take a series of daily indexes and roll them up into a monthly index.

## Example usage

```
./elastic-indexrollup -infilter ^netflow-2016\.*$ -inpattern netflow-2006.01.02 -outpattern netflowrollup-2006.01
```

This would take any index with the name matching the regex `^netflow-2016\.*$`,(all of the NetFlow indexes from 2016) and will roll up all the contained documents into an index matching the GoLang time format of `netflowrollup-2006.01`

This tool does not delete or modify the source indexes in any way.

## Command line parameters

```
  -benchmark
    	Run benchmarks with different sized threads and buffers
  -buffersize int
    	Number of records to insert at any given time (default 1000)
  -infilter string
    	A regex to match against index names
  -inhost string
    	ElasticSearch host to read indexes from. (default "http://localhost:9200")
  -inpattern string
    	The output pattern for indexing the read data, in the Go time format (https://golang.org/pkg/time/#Parse)
  -outhost string
    	(optional) ElasticSearch host to write indexes to. If blank, uses the inhost option
  -outpattern string
    	The output pattern for indexing the read data, in the Go time format (https://golang.org/pkg/time/#Parse)
  -threads int
    	Number of worker threads to process. Each thread will process one day at a time. (default 3)
```

### Input parameters

You must specify two parts to the input filter:

* `-infilter` is the regular expression we are going to match against to figure out which indexes to roll up
* `-inpattern` is the [Go time string](https://golang.org/pkg/time/#Parse) that is used to extract the date from the index name.

There is an optional `-inhost` you can specify in the event that the machine running the rollup is not a member of the ElasticSearch cluster you are reading from.

### Output parameters

You must specify one part to the output filter:

* `-outpattern` is the [Go time string](https://golang.org/pkg/time/#Parse) that you will use to represent the _new_ rolled-up index name. Typically it will be very similar to `-inpattern`, but with a different date format (e.g. omitting the day portion).

There is an optional `-outhost` you can specify in the event that the machine running the rollup is not a member of the ElasticSearch cluster you are writing to.

### Other parameters

* `-threads` is the number of reader threads that will be run in parallel. Each thread processes a single index's records. The default here is 3, but you can fine tune this as required. If you have a lot of nodes in your ElasticSearch cluster, you might be able to bump this up to read more data concurrently. You can use the `-benchmark` flag to help figure this out.
* `-buffersize` is the number of records that will be indexed into ElasticSearch using the [Bulk API](https://www.elastic.co/guide/en/elasticsearch/reference/current/docs-bulk.html). You can fine-tune this based on your cluster's capacity. If you are reading and writing between two different clusters, you may be able to bump this up substantially higher than if you are reading and writing from the same cluster.
* `-benchmark` See next section, "Running a benchmark"

### Running a benchmark

You can pass the command-line argument `-benchmark`, which will repeadly run the rollup (using the normal command line parameters of `-infilter -inpattern`, etc) but using different thread counts and buffer sizes each time.

This command will actually run the complete rollup dozens of times, so you will want to choose a dataset that can be executed reasonably quickly. For example, you may choose to only run 5 indexes, rather than 30. You should run at least 5 indexes, otherwise the benchmark will return inaccurate results when taking the number of threads into account.

While the benchmark is running, you will see the regular output of the
rollup task, and between tasks you will see the benchmark results. The benchmark results will be displayed in their entirety once the benchmark has finished running.

#### Example benchmark

```
./elastic-indexrollup -infilter ^netflow-2016\.08\.0[1-9]$ -inpattern netflow-2006.01.02 -outpattern netflowrollup-2006.01 -benchmark

Running benchmark...
+---------+---------+--------------+--------------+--------------+--------------+
| THREADS | BUFFERS |   AVERAGE    |      1       |      2       |      3       |
+---------+---------+--------------+--------------+--------------+--------------+
|       1 |     100 | 7.032010025s | 7.765402535s | 6.48198033s  | 6.848647211s |
|       1 |    1000 | 3.656595542s | 3.872622917s | 3.731247202s | 3.365916509s |
|       1 |    2000 | 3.506934259s | 3.587168599s | 3.481452909s | 3.452181271s |
|       1 |    5000 | 4.241972637s | 4.014180328s | 4.081102814s | 4.630634771s |
|       1 |   10000 | 4.005334995s | 4.135364095s | 4.190424814s | 3.690216077s |
|       2 |     100 | 6.876088913s | 6.286677899s | 7.946837352s | 6.394751488s |
|       2 |    1000 | 3.908789094s | 4.717301235s | 2.942230651s | 4.066835398s |
|       2 |    2000 | 3.846957962s | 4.16444763s  | 3.662175182s | 3.714251074s |
|       2 |    5000 | 4.308638974s | 4.171708237s | 4.881029867s | 3.87317882s  |
|       2 |   10000 | 3.95621871s  | 4.24775057s  | 3.983209245s | 3.637696317s |
|       3 |     100 | 5.975052925s | 6.691305886s | 5.741787983s | 5.492064908s |
|       3 |    1000 | 3.228477307s | 3.697853403s | 2.580436193s | 3.407142326s |
|       3 |    2000 | 3.279620294s | 3.930161805s | 3.430743679s | 2.477955399s |
|       3 |    5000 | 3.776666504s | 4.105691154s | 3.589109953s | 3.635198405s |
|       3 |   10000 | 4.619631571s | 5.683342255s | 4.187518653s | 3.988033805s |
|       4 |     100 | 5.788315777s | 5.367102497s | 6.528121071s | 5.469723763s |
|       4 |    1000 | 3.755523547s | 3.990404618s | 3.67786306s  | 3.598302965s |
|       4 |    2000 | 3.450045397s | 3.598420238s | 2.7933576s   | 3.958358355s |
|       4 |    5000 | 5.653352342s | 3.906511671s | 6.382200218s | 6.671345137s |
|       4 |   10000 | 4.630328341s | 4.060135206s | 6.262994998s | 3.567854821s |
|       5 |     100 | 6.276991124s | 6.77378284s  | 5.430760657s | 6.626429875s |
|       5 |    1000 | 3.136603223s | 4.01807432s  | 2.811549726s | 2.580185625s |
|       5 |    2000 | 3.80473662s  | 4.549284338s | 2.957112866s | 3.907812657s |
|       5 |    5000 | 4.776529883s | 4.161029094s | 6.085182696s | 4.083377861s |
|       5 |   10000 | 3.650422406s | 4.069182497s | 3.056093682s | 3.82599104s  |
+---------+---------+--------------+--------------+--------------+--------------+
```

In this sample, our index filter is probably too small, as those numbers are not quite large enough to give us meaningful results. However it looks like 3 threads and a buffer size of 2000 will give us a pretty optimal result.


