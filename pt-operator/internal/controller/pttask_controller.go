/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	perftestv1 "com.google.gtools/pt-operator/api/v1"
	"com.google.gtools/pt-operator/internal/helper"
)

// PtTaskReconciler reconciles a PtTask object
type PtTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=perftest.com.google.gtools,resources=pttasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=perftest.com.google.gtools,resources=pttasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=perftest.com.google.gtools,resources=pttasks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PtTask object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PtTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	var pTask perftestv1.PtTask
	if err := r.Get(ctx, req.NamespacedName, &pTask); err != nil {
		l.Info("unable to fetch PtTask")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	} else {

		if pTask.Status.Id == "" {
			id := uuid.New()
			pTask.Status.Id = id.String()
			pTask.Status.Phases = make(map[string]string)
			l.Info("Set Id to ptTask", "Id", pTask.Status.Id)
			if err = r.Client.Status().Update(context.Background(), &pTask); err != nil {
				l.Info("failed to update status of ptTask")
				return ctrl.Result{}, err
			}
		}

		for _, x := range pTask.Spec.Execution {

			l.Info("Process ptTask in progress")
			switch x.Executor {
			case "locust":
				l.Info("Provisioning for Locust")
				trsConf, _ := yaml.Marshal(pTask.Spec)
				// l.Info("BO:")
				l.Info(string(trsConf))
				// l.Info("EO:")
				if ph, err := do4Locust(ctx, r, req, &pTask, x.Scenario, x.Workers); err != nil {
					l.Error(err, "failed to provision/execute Locust", "phase", ph)
					return ctrl.Result{}, err
				} else {
					if err := r.Get(ctx, req.NamespacedName, &pTask); err != nil {
						l.Info("unable to fetch PtTask")
						return ctrl.Result{}, client.IgnoreNotFound(err)
					}
					pTask.Status.Phases[x.Scenario] = ph
					if err = r.Client.Status().Update(context.Background(), &pTask); err != nil {
						l.Error(err, "failed to update status of ptTask", "phase", ph)
						return ctrl.Result{}, err
					}
				}

			case "jmeter":
				l.Info("Provisioning for JMeter")
			default:
				l.Info("The testing framework wasn't supported yet", "framework", pTask.Spec.Execution[0].Executor)
			}
		}

	}

	return ctrl.Result{}, nil
}

// func convert2Text(pTask perftestv1.PtTask) string {
// 	return ""
// }

func do4Locust(ctx context.Context, client *PtTaskReconciler, req ctrl.Request, pTask *perftestv1.PtTask, scenario string, workerNum int) (string, error) {
	l := log.FromContext(ctx)
	masterImage := "asia-docker.pkg.dev/play-api-service/test-images/taurus-base"
	workerImage := "asia-docker.pkg.dev/play-api-service/test-images/locust-worker"

	// 1. Create Locust master Pod
	phase := "initial"

	// 2. Create Locust master service for Pod
	phase = "privision_master"
	trsConf, _ := yaml.Marshal(pTask.Spec)
	mp := helper.BuildMasterPod4Locust(masterImage, pTask.Status.Id, scenario, string(trsConf))
	mpNN := types.NamespacedName{
		Name:      mp.Name,
		Namespace: mp.Namespace,
	}
	var xMp = corev1.Pod{}
	if err := client.Get(ctx, mpNN, &xMp); err != nil {
		if err := client.Create(ctx, mp); err != nil {
			l.Error(err, "failed to create Pod for master node of Locust")
			return phase, err
		}
	} else {
		l.Info("master node was existed", "name", mp.Name, "namespace", mp.Namespace)

	}

	ms := helper.BuildMasterService4Locust(corev1.ServiceTypeClusterIP, scenario)
	msNN := types.NamespacedName{
		Name:      ms.Name,
		Namespace: ms.Namespace,
	}
	var xMs = corev1.Service{}
	if err := client.Get(ctx, msNN, &xMs); err != nil {
		if err := client.Create(ctx, ms); err != nil {
			l.Error(err, "failed to create Service for master node of Locust")
			return phase, err
		}
	} else {
		l.Info("master svc was existed", "name", xMs.Name, "namespace", xMs.Namespace)
	}

	// 4. Create Locust worker Pods as demand if locust master is ready
	svcHost := ""
	svcPort := ""
	isReady := false
	for {
		isReady, svcHost, svcPort = checkLocustMaster(ctx, client, req, mpNN, msNN)
		if !isReady {
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
	l.Info("master service for Locust is ready", "host", svcHost, "port", svcPort)

	phase = "privision_worker"
	// Creat multiple workers
	for i := 1; i < workerNum+1; i++ {
		wk := helper.BuildLocusterWorker4Locust(workerImage, svcHost, svcPort, scenario, strconv.Itoa(i))
		var xWk = corev1.Pod{}
		wkNN := types.NamespacedName{
			Name:      wk.Name,
			Namespace: wk.Namespace,
		}
		if err := client.Get(ctx, wkNN, &xWk); err != nil {
			if err := client.Create(ctx, wk); err != nil {
				l.Error(err, "failed to create Pod for worker node of Locust", "worker", wk.ObjectMeta.Name)
				return phase, err
			}
		} else {
			l.Info("worker node was existed", "name", wk.Name, "namespace", wk.Namespace)

		}

	}

	// 5. Kick off monitoring and keep update along the way
	// TODO
	// go MonitorLocustTesting(scenario, "/taurus-logs/"+pTask.Status.Id+"/"+scenario)

	// 6. Achieve testing logs/reports
	// TODO

	// 7. Mark PtTask was done
	// TODO

	// 8. Singal to clean up all related resources except metadata store & GCS
	// TODO
	return phase, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PtTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&perftestv1.PtTask{}).
		Complete(r)
}

// Checking status of master whether it should launch worker nodes
func checkLocustMaster(ctx context.Context, client *PtTaskReconciler, req ctrl.Request, masterNN types.NamespacedName, masterSNN types.NamespacedName) (bool, string, string) {
	l := log.FromContext(ctx)

	var masterPod corev1.Pod
	var masterSvc corev1.Service
	if err := client.Get(ctx, masterNN, &masterPod); err != nil {
		l.Error(err, "unable to fetch Pod for master node", "name", masterNN.Name, "namespace", masterNN.Namespace)
	} else {
		for _, cs := range masterPod.Status.ContainerStatuses {
			if *cs.Started && cs.Ready {
				if err := client.Get(ctx, masterSNN, &masterSvc); err != nil {
					l.Error(err, "unable to fetch Service for master node", "name", masterSNN.Name, "namespace", masterSNN.Namespace)
				} else {
					if masterSvc.Spec.Type == corev1.ServiceTypeClusterIP {
						return true, masterSvc.Spec.ClusterIP, strconv.Itoa(int(masterSvc.Spec.Ports[0].Port))
					} else if masterSvc.Spec.Type == corev1.ServiceTypeLoadBalancer {
						if masterSvc.Status.LoadBalancer.Ingress != nil {
							return true, masterSvc.Status.LoadBalancer.Ingress[0].IP, strconv.Itoa(int(masterSvc.Spec.Ports[0].Port))
						}
					}
				}
			}
		}
	}
	return false, "", ""

}
