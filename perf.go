package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// A tiny section profiler for the harness runs: perfMark closes the section
// that was open and opens the named one. The totals are reported per frame
// at the end of a benchmark, which shows where the frame goes in a way a CPU
// profile cannot, because time blocked inside the graphics driver is not
// sampled.
var (
	perfSections = map[string]time.Duration{}
	perfOpen     string
	perfOpenAt   time.Time
	perfOn       bool
)

func perfMark(name string) {
	if !perfOn {
		return
	}
	now := time.Now()
	if perfOpen != "" {
		perfSections[perfOpen] += now.Sub(perfOpenAt)
	}
	perfOpen, perfOpenAt = name, now
}

// perfReport lists the sections as milliseconds per frame, largest first.
func perfReport(frames int) string {
	perfMark("")
	type kv struct {
		k string
		v time.Duration
	}
	var all []kv
	for k, v := range perfSections {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v })
	var b strings.Builder
	for _, e := range all {
		fmt.Fprintf(&b, " %s=%.2f", e.k, float64(e.v.Microseconds())/1000/float64(max(frames, 1)))
	}
	return b.String()
}
