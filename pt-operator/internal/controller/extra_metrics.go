package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/nxadm/tail"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
	TotalUsers int
	// Totoal RPS ::GuageVec
	TotalReqsPerSec int
	// Total occured errors :: CounterVec
	TotalErrors int
	// The status of master node (0-live/1-unknown) ::GuageVec
	MastersStatus int
	// Total worker ::Guage
	TotalWorkers int
	// The status of worker node: (worker name)->(0-live/1-unknown) ::GuageVec
	WorkersStatus map[string]int

	// Average response time per endpoint: (endpoint)->(response time) :: GuageVec
	AvgResponseTimes map[string]float64
}

var (
	// Data in memory: (scenario)->(*PtTaskWorkerMetrics)
	ptTaskMetrics = make(map[string]*PtTaskWorkerMetrics)
	// Totoal users/concurrency
	pttaskTotalUsers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_users",
			Help: "Number of total users for a PT task",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	// Totoal RPS
	pttaskTotalReqsPerSec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_request_per_sec",
			Help: "Number of total RPS",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	// Total occured errors
	pttaskTotalErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gtools_perftest_pttask_total_errors",
			Help: "Number of total errors",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	// Total number of master node
	pttaskTotalMasters = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_masters",
			Help: "Number of master node",
		},
	)
	// The status of master node
	pttaskMastersStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_master_status",
			Help: "The status of master node",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	// Total number of workers
	pttaskTotalWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_total_workers",
			Help: "Number of worker node",
		},
		[]string{
			"scenario", //each scenario
		},
	)
	// The status of worker node
	pttaskWorkersStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_worker_status",
			Help: "The status of worker node",
		},
		[]string{
			"worker",   //name of worker node
			"scenario", //each scenario
		},
	)
	// Average response time per endpoint
	pttaskAvgResponseTimes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gtools_perftest_pttask_avg_response_time",
			Help: "Average response time per endpoint",
		},
		[]string{
			"endpoint", //Which endpoint
			"worker",   //name of worker node
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

		if ptMetric, ok := ptTaskMetrics[scenario]; ok {
			if ptMetric.AvgResponseTimes == nil {
				ptMetric.AvgResponseTimes = make(map[string]float64)
			}
			ptMetric.AvgResponseTimes[stat.Name] = avg
		} else {

			ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
				AvgResponseTimes: map[string]float64{
					stat.Name: avg,
				},
			}
		}
		pttaskAvgResponseTimes.WithLabelValues(stat.Name, llj.ClientId, scenario).Set(avg)
	}
}

func updateTotals(scenario string, llj LocustLdjson) {
	if ptMetric, ok := ptTaskMetrics[scenario]; ok {
		ptMetric.TotalUsers = llj.UserCount
		ptMetric.TotalReqsPerSec = totalVals(llj.StatsTotal.NumReqsPerSec)
	} else {
		ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
			TotalUsers:      llj.UserCount,
			TotalReqsPerSec: totalVals(llj.StatsTotal.NumReqsPerSec),
		}
	}

	pttaskTotalUsers.WithLabelValues(scenario).Set(float64(totalVals(llj.StatsTotal.NumReqsPerSec)))
	pttaskTotalReqsPerSec.WithLabelValues(scenario).Set(float64(totalVals(llj.StatsTotal.NumReqsPerSec)))

}

func totalVals(m map[string]int) int {
	total := 0
	for _, v := range m {
		total = total + v
	}
	return total
}

// Monitoring testing result through Locust log file, called locust-workers.ldjson by default
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

// Monitoring master node for Locust, called bzt.log by default
func monitorLocustMaster(scenario string, file string) {
	l := log.Log
	t, err := tail.TailFile(
		file, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		l.Error(err, "failed to tail locust.log")
	} else {
		// decode content line by line
		for line := range t.Lines {
			if strings.HasSuffix(line.Text, " at /bzt-configs") {
				if ptMetric, ok := ptTaskMetrics[scenario]; ok {
					ptMetric.MastersStatus = 0
				} else {
					ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
						MastersStatus: 0,
					}
				}
				pttaskMastersStatus.WithLabelValues(scenario).Set(0)
			}
		}
	}

}

// Monitoring worker node for Locust
func monitorLocustLocalWorker(scenario string, r *PtTaskReconciler, nn types.NamespacedName) {
	//TODO: monitor worker node by check worker pod
	ctx := context.Background()
	var workerPod corev1.Pod
	if err := r.Get(ctx, nn, &workerPod); err != nil {

	} else {
		if workerPod.Status.Phase == corev1.PodRunning {

		} else {
			if ptMetric, ok := ptTaskMetrics[scenario]; ok {
				ptMetric.WorkersStatus[workerPod.Name] = 0
			} else {
				ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
					WorkersStatus: map[string]int{workerPod.Name: 0},
				}
			}
			ptTaskMetrics[scenario].WorkersStatus[workerPod.Name] = 0
		}
	}
}

func monitorLocustRemoteWorker(scenario string) {
	//TODO: monitor worker node by check worker pod
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(pttaskTotalUsers, pttaskTotalReqsPerSec, pttaskTotalErrors,
		pttaskTotalMasters, pttaskMastersStatus, pttaskTotalWorkers, pttaskWorkersStatus,
		pttaskAvgResponseTimes)

}
