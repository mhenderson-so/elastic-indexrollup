package main

import (
	"time"

	elastic "gopkg.in/olivere/elastic.v3"
)

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

type benchmarkData map[benchmarkSet]benchmarkResult
type benchmarkSet struct {
	Threads int
	Buffers int
}
type benchmarkSets []benchmarkSet
type benchmarkResult struct {
	Results []time.Duration
	Average time.Duration
}

func (s benchmarkSets) Len() int {
	return len(s)
}
func (s benchmarkSets) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s benchmarkSets) Less(i, j int) bool {
	if s[i].Threads == s[j].Threads {
		return s[i].Buffers < s[j].Buffers
	}
	return s[i].Threads < s[j].Threads
}
