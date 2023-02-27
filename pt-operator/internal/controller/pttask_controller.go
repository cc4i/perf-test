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
	"time"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
		id := uuid.New()
		pTask.Status.Id = id.String()
		r.Update(ctx, &pTask)
		if len(pTask.Status.Phases) == 0 {
			for _, x := range pTask.Spec.Execution {
				switch x.Executor {
				case "locust":
					l.Info("Provisioning for Locust")
					do4Locust(ctx, r, req, &pTask, x.Scenario, x.Workers)
				case "jmeter":
					l.Info("Provisioning for JMeter")
				default:
					l.Info("The testing framework wasn't supported yet", "framework", pTask.Spec.Execution[0].Executor)
				}
			}

		}

	}

	return ctrl.Result{}, nil
}

func do4Locust(ctx context.Context, client *PtTaskReconciler, req ctrl.Request, pTask *perftestv1.PtTask, scenario string, workerNum int) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// 1. Create Locust master Pod
	pTask.Status.Phases[scenario] = "initial"
	client.Update(ctx, pTask)
	mp := helper.BuildMasterPod4Locust("", pTask.Status.Id, scenario)
	if err := client.Create(ctx, mp); err != nil {
		l.Error(err, "failed to create Pod for master node of Locust")
		return ctrl.Result{}, err
	}
	// 2. Create Locust master service for Pod
	pTask.Status.Phases[scenario] = "privision_master"
	client.Update(ctx, pTask)
	ms := helper.BuildMasterService4Locust(corev1.ServiceTypeClusterIP, scenario)
	if err := client.Create(ctx, ms); err != nil {
		l.Error(err, "failed to create Pod for master node of Locust")
		return ctrl.Result{}, err
	}

	master := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: mp.Name,
			Name:      mp.Namespace,
		},
	}

	// 4. Create Locust worker Pods as demand if locust master is ready
	for !checkLocustMaster(ctx, client, req, master) {
		time.Sleep(1 * time.Second)
	}
	pTask.Status.Phases[scenario] = "privision_worker"
	client.Update(ctx, pTask)
	// TODO: 1. svc ip 2.loop to creat multiple workers
	for i := 0; i < workerNum; i++ {
		if err := client.Create(ctx, helper.BuildLocusterWorker4Locust("", "", "", scenario)); err != nil {
			l.Error(err, "failed to create Pod for worker node of Locust", "worker", "")
		}
	}

	// 5. Kick off monitoring and keep update along the way
	// TODO
	go MonitorLocustTesting(scenario, "/taurus-logs/"+pTask.Status.Id+"/"+scenario)

	// 6. Achieve testing logs/reports
	// TODO

	// 7. Mark PtTask was done
	// TODO

	// 8. Singal to clean up all related resources except metadata store & GCS
	// TODO
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PtTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&perftestv1.PtTask{}).
		Complete(r)
}

// Checking status of master whether it should launch worker nodes
func checkLocustMaster(ctx context.Context, client *PtTaskReconciler, req ctrl.Request, master reconcile.Request) bool {
	l := log.FromContext(ctx)

	var masterPod corev1.Pod
	if err := client.Get(ctx, master.NamespacedName, &masterPod); err != nil {
		l.Error(err, "unable to fetch Pod for master node")
	} else {
		for _, cs := range masterPod.Status.ContainerStatuses {
			if *cs.Started && cs.Ready {
				// Provision workers
				return true
			}
		}
	}
	return false

}
