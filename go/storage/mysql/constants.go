package mysql

import (
	"time"

	"github.com/uber-go/tally"
)

// Histogram buckets are 0s, 1ms, 2ms, 5ms, 10ms, 15ms, 25ms, 50ms, 75ms, 100ms, 200ms, 300ms, 400ms, 500ms, 600ms, 700ms, 800ms, 900ms
// 1s, 2s, 3s, 4s, 5s, 10s, 30s, 1m, 5m, 10m, 30m, 1h
var timespanBuckets = tally.DurationBuckets{
	0 * time.Second,
	1 * time.Millisecond,
	2 * time.Millisecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	15 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	75 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	300 * time.Millisecond,
	400 * time.Millisecond,
	500 * time.Millisecond,
	600 * time.Millisecond,
	700 * time.Millisecond,
	800 * time.Millisecond,
	900 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	3 * time.Second,
	4 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

// MinMySQLResVer is the minimum resource version of an object that is updated directly in MySQL
const MinMySQLResVer = uint64(1) << 62

const (
	_syncDelay                  = "syncDelay"
	_syncDelayHistogram         = "syncDelayHistogram"
	_mysqlQueryLatency          = "mysqlQueryLatency"
	_mysqlQueryLatencyHistogram = "mysqlQueryLatencyHistogram"
	_mysqlQueryCount            = "mysqlQueryCount"
	_mysqlQuerySuccess          = "mysqlQuerySuccess"
	_mysqlQueryFailure          = "mysqlQueryFailure"
)
