package controller

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/nxadm/tail"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Structure for each line of Locust logs (locust-workers.ldjson)
type LocustLdjson struct {
	Stats            []LocustLdjsonStats `json:"stats"`
	StatsTotal       LocustLdjsonStats   `json:"stats_total"`
	Errors           interface{}         `json:"errors,omitempty"`
	UserClassesCount map[string]int      `json:"user_classes_count,omitempty"`
	UserCount        int                 `json:"user_count"`
	ClientId         string              `json:"client_id"`
}

// Subset of LocustLdjson
type LocustLdjsonStats struct {
	Name                 string         `json:"name,omitempty"`
	Method               string         `json:"method,omitempty"`
	LastRequestTimestamp float64        `json:"last_request_timestamp,omitempty"`
	StartTime            float64        `json:"start_time,omitempty"`
	NumRequests          int            `json:"num_requests,omitempty"`
	NumNoneRequests      int            `json:"num_none_requests,omitempty"`
	NumFailures          int            `json:"num_failures,omitempty"`
	TotalResponseTime    float64        `json:"total_response_time,omitempty"`
	MaxResponseTime      float64        `json:"max_response_time,omitempty"`
	MinResponseTime      float64        `json:"min_response_time,omitempty"`
	TotalContentLength   int            `json:"total_content_length,omitempty"`
	ResponseTimes        map[string]int `json:"response_times,omitempty"`
	NumReqsPerSec        map[string]int `json:"num_reqs_per_sec,omitempty"`
	NumFailPerSec        map[string]int `json:"num_fail_per_sec,omitempty"`
}

// Local memory data for metrics
type PtTaskWorkerMetrics struct {
	// Totoal users/concurrency ::GuageVec
	TotalUsers map[string]int
	// Totoal RPS ::GuageVec
	TotalReqsPerSec map[string]int
	// Total errors occured :: CounterVec
	TotalErrors map[string]int
	// Total master ::Guage
	TotalMasters int
	// The status of master node ::GuageVec
	MastersStatus map[string]string
	// Total worker ::Guage
	TotalWorkers int
	// The status of worker ndoe ::GuageVec
	WorkersStatus map[string]string

	// Average response time per endpoint [endpoint -> response time] :: GuageVec
	AvgResponseTimes map[string]float64
}

var (
	ptTaskMetrics    = make(map[string]*PtTaskWorkerMetrics)
	pttaskTotalUsers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_users",
			Help: "Number of total users for a PT task",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	pttaskTotalReqsPerSec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_request_per_sec",
			Help: "Number of total RPS",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	pttaskTotalErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gtools_perftest_pttask_total_errors",
			Help: "Number of total errors",
		},
		[]string{
			"scenario", //each scenario
		},
	)

	pttaskTotalMasters = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_masters",
			Help: "Number of master node",
		},
	)
	pttaskMastersStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_master_status",
			Help: "The status of master node",
		},
		[]string{
			"name",     // name of master node
			"scenario", //each scenario
		},
	)
	pttaskTotalWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_workers",
			Help: "Number of worker node",
		},
		[]string{
			"name",     // name of worker node
			"scenario", //each scenario
		},
	)
	pttaskWorkersStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_worker_status",
			Help: "The status of worker node",
		},
		[]string{
			"name",     // name of worker node
			"scenario", //each scenario
		},
	)

	pttaskAvgResponseTimes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_avg_response_time",
			Help: "Average response time per endpoint",
		},
		[]string{
			"endpoint", // Which endpoint
			"scenario", //each scenario
		},
	)
)

func updateReponseTimes(scenario string, llj LocustLdjson) {
	for _, stat := range llj.Stats {
		sum := float64(0)
		count := 0
		for t, ct := range stat.ResponseTimes {
			rp, _ := strconv.ParseFloat(t, 64)
			sum = sum + (rp * float64(ct))
			count = count + ct
		}
		avg := sum / float64(count)

		if ptMetric, ok := ptTaskMetrics[llj.ClientId]; ok {
			if ptMetric.AvgResponseTimes == nil {
				ptMetric.AvgResponseTimes = make(map[string]float64)
			}
			ptMetric.AvgResponseTimes[stat.Name] = avg
		} else {

			ptTaskMetrics[llj.ClientId] = &PtTaskWorkerMetrics{
				AvgResponseTimes: map[string]float64{
					stat.Name: avg,
				},
			}
		}

		pttaskAvgResponseTimes.WithLabelValues(stat.Name, scenario).Set(avg)
	}

}

func updateTotals(scenario string, llj LocustLdjson) {
	if ptMetric, ok := ptTaskMetrics[llj.ClientId]; ok {
		if ptMetric.TotalUsers == nil {
			ptMetric.TotalUsers = make(map[string]int)
		}
		ptMetric.TotalUsers[scenario] = llj.UserCount
		if ptMetric.TotalReqsPerSec == nil {
			ptMetric.TotalReqsPerSec = make(map[string]int)
		}
		ptMetric.TotalReqsPerSec[scenario] = totalVals(llj.StatsTotal.NumReqsPerSec)
	} else {
		ptTaskMetrics[llj.ClientId] = &PtTaskWorkerMetrics{
			TotalUsers: map[string]int{
				scenario: llj.UserCount,
			},
			TotalReqsPerSec: map[string]int{
				scenario: totalVals(llj.StatsTotal.NumReqsPerSec),
			},
		}
	}

	totalU := 0
	totalRps := 0
	for _, v := range ptTaskMetrics {
		totalU = totalU + v.TotalUsers[scenario]
		totalRps = totalRps + v.TotalReqsPerSec[scenario]

	}
	pttaskTotalUsers.WithLabelValues(scenario).Set(float64(totalU))
	pttaskTotalReqsPerSec.WithLabelValues(scenario).Set(float64(totalRps))

}

func totalVals(m map[string]int) int {
	total := 0
	for _, v := range m {
		total = total + v
	}
	return total
}

// Monitoring testing result throung Locust log file, called locust-workers.ldjson by default
func MonitorLocustTesting(scenario string, file string) {
	l := log.Log
	t, err := tail.TailFile(
		file, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		l.Error(err, "failed to tail locust-workers.ldjson")
	} else {
		// decode content line by line
		for line := range t.Lines {
			l.Info(line.Text)
			var llj LocustLdjson
			err = json.Unmarshal([]byte(line.Text), &llj)
			if err != nil {
				l.Error(err, "failed to unmarshal logs")
			} else {
				l.Info("client_id", llj.ClientId)
				updateTotals(scenario, llj)
				updateReponseTimes(scenario, llj)
			}
		}
	}
}

// Monitoring master node for Locust
func monitorLocustMaster(file string) {
	l := log.Log
	t, err := tail.TailFile(
		file, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		l.Error(err, "failed to tail locust.log")
	} else {
		// decode content line by line
		for line := range t.Lines {
			if strings.HasSuffix(line.Text, " at /bzt-configs") {

			}
		}
	}

}

// Monitoring worker node for Locust
func monitorLocustWorker() {

}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(pttaskTotalUsers, pttaskTotalReqsPerSec, pttaskTotalErrors,
		pttaskTotalMasters, pttaskMastersStatus, pttaskTotalWorkers, pttaskWorkersStatus,
		pttaskAvgResponseTimes)

}
