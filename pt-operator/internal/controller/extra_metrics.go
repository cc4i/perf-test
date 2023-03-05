package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"com.google.gtools/pt-operator/internal/helper"
	"github.com/nxadm/tail"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	//TODO:monitor testing result
	t, err := tail.TailFile(
		file, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		l.Error(err, "failed to tail locust-workers.ldjson")
	} else {
		// decode content line by line
		for line := range t.Lines {
			// l.Info(line.Text)
			var llj LocustLdjson
			err = json.Unmarshal([]byte(line.Text), &llj)
			if err != nil {
				l.Error(err, "failed to unmarshal logs")
			} else {
				l.Info("process logs from simulated worker", "client_id", llj.ClientId, "scenario", scenario, "user_count", llj.UserCount)
				updateTotals(scenario, llj)
				updateReponseTimes(scenario, llj)
			}
		}
	}
}

// Monitoring master node for Locust, called bzt.log by default
func MonitorLocustMaster(scenario string, r *PtTaskReconciler, nn types.NamespacedName) {
	//TODO: monitor master node
	l := log.Log
	ctx := context.Background()
	var masterPod corev1.Pod
	var status int
	for {
		if err := r.Get(ctx, nn, &masterPod); err != nil {
			status = 1
			l.Error(err, "failed to get master pod")
		} else {
			l.Info("get master pod", "name", masterPod.Name)
			if masterPod.Status.Phase == corev1.PodRunning {
				status = 0
			}
		}
		if ptMetric, ok := ptTaskMetrics[scenario]; ok {
			ptMetric.MastersStatus = status
		} else {
			ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
				MastersStatus: status,
			}

		}

		pttaskMastersStatus.WithLabelValues(scenario).Set(float64(status))
		if status == 0 {
			pttaskTotalMasters.Set(float64(1))
		} else {
			pttaskTotalMasters.Set(float64(0))
		}
		time.Sleep(5 * time.Second)
	}

}

// Monitoring worker node for Locust
func MonitorLocustLocalWorker(scenario string, ptr *PtTaskReconciler, namespace string) {
	//TODO: monitor worker node by check worker pod
	ctx := context.Background()
	l := log.Log
	var workerPodList corev1.PodList
	rl, _ := labels.NewRequirement("app", selection.Equals, []string{"locust-worker"})

	for {
		numWorker := 0
		if err := ptr.List(ctx, &workerPodList, &client.ListOptions{Namespace: namespace, LabelSelector: labels.NewSelector().Add(*rl)}); err != nil {
			l.Error(err, "failed to list worker pods")
		} else {
			for _, pod := range workerPodList.Items {
				status := 1
				if pod.Status.Phase == corev1.PodRunning {
					status = 0
					numWorker++
				}
				if ptMetric, ok := ptTaskMetrics[scenario]; ok {
					if ptMetric.WorkersStatus == nil {
						ptMetric.WorkersStatus = make(map[string]int)
					}
					ptMetric.WorkersStatus[pod.Name] = status
				} else {
					ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
						WorkersStatus: map[string]int{pod.Name: status},
					}
				}
				ptTaskMetrics[scenario].WorkersStatus[pod.Name] = status
				pttaskWorkersStatus.WithLabelValues(pod.Name, scenario).Set(float64(status))
			}
		}
		pttaskTotalWorkers.WithLabelValues(scenario).Set(float64(numWorker))

		time.Sleep(20 * time.Second)
	}

}

func MonitorLocustDistributionWorker(scenario string, workerId string, caText64 string, gkeEndpoint string) {
	//TODO: monitor worker node by check worker pod
	l := log.Log
	ctx := context.Background()
	name := "locust-worker-" + scenario + "-" + workerId
	var workerPod corev1.Pod

	for {
		numWorker := 0
		if _, k2c, err := helper.Kube2Client(ctx, caText64, gkeEndpoint); err != nil {
			l.Error(err, "failed to get kube2client")
		} else {
			//Get Pod
			status := 1
			if workerPod, err := k2c.CoreV1().Pods("default").Get(ctx, name, metav1.GetOptions{}); err != nil {
			} else {
				if workerPod.Status.Phase == corev1.PodRunning {
					status = 0
					numWorker++
				}
			}
			if ptMetric, ok := ptTaskMetrics[scenario]; ok {
				if ptMetric.WorkersStatus == nil {
					ptMetric.WorkersStatus = make(map[string]int)
				}
				ptMetric.WorkersStatus[workerPod.Name] = status
			} else {
				ptTaskMetrics[scenario] = &PtTaskWorkerMetrics{
					WorkersStatus: map[string]int{workerPod.Name: status},
				}
			}
			ptTaskMetrics[scenario].WorkersStatus[workerPod.Name] = status
			pttaskWorkersStatus.WithLabelValues(scenario, workerPod.Name).Set(float64(status))
		}
		pttaskTotalWorkers.WithLabelValues(scenario).Set(float64(numWorker))

		time.Sleep(20 * time.Second)
	}

}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(pttaskTotalUsers, pttaskTotalReqsPerSec, pttaskTotalErrors,
		pttaskTotalMasters, pttaskMastersStatus, pttaskTotalWorkers, pttaskWorkersStatus,
		pttaskAvgResponseTimes)

}
