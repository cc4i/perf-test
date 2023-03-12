package helper

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Build a Pod as Locust Master as per PtTask
func BuildMasterPod4Locust(ns string, img string, id string, scenario string, trsConf string) *corev1.Pod {
	gsec := int64(30)
	name := "locust-master-" + scenario

	labels := map[string]string{
		"module":   "performance-testing",
		"app":      "locust-master",
		"scenario": scenario,
	}

	master := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &gsec,
			RestartPolicy:                 corev1.RestartPolicyNever,
			InitContainers: []corev1.Container{
				{
					Name:  "taurus-init",
					Image: "asia-docker.pkg.dev/play-api-service/test-images/busybox:1.28",
					Command: []string{
						"sh",
						"-c",
						"cat <<EOF >>/taurus-configs/taurus.yaml\n" + trsConf,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "taurus-config",
							MountPath: "/taurus-configs",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:            name,
					Image:           img,
					ImagePullPolicy: corev1.PullAlways,
					Command: []string{
						"/usr/local/bin/bzt",
					},
					//CMD ["/usr/local/bin/bzt", "/taurus-configs/taurus.yaml", "-o", "modules.console.disable=true", "-o", "settings.artifacts-dir=/taurus-logs/%Y-%m-%d_%H-%M-%S.%f"]
					Args: []string{
						"/taurus-configs/taurus.yaml",
						"-o",
						"modules.console.disable=true",
						"-o",
						"settings.artifacts-dir=/taurus-logs/" + id + "/" + scenario,
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "tcp",
							ContainerPort: 5557,
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2048Mi"),
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"grep",
									" at /bzt-configs",
									"/taurus-logs/" + id + "/" + scenario + "/bzt.log",
								},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       30,
						FailureThreshold:    100,
					},

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "taurus-config",
							MountPath: "/taurus-configs",
						},
						{
							Name:      "bzt-pvc",
							MountPath: "/taurus-logs",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "taurus-config",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "bzt-pvc",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "bzt-filestore-pvc",
						},
					},
				},
			},
		},
	}
	return &master
}

// svcType could be "ClusterIP" or "LoadBlancer". "LoadBlancer" is to expose endpoint publicly.
func BuildMasterService4Locust(ns string, svcType corev1.ServiceType, scenario string) *corev1.Service {
	name := "locust-master-" + scenario + "-svc"

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Type: svcType,
			Selector: map[string]string{
				"app":      "locust-master",
				"scenario": scenario,
			},
			Ports: []corev1.ServicePort{
				{
					Name: "tcp",
					Port: 5557,
					TargetPort: intstr.IntOrString{
						IntVal: 5557,
					},
				},
			},
		},
	}
	return &svc
}

// Build a Pod as Locust Worker as per PtTask
func BuildLocusterWorker4Locust(ns string, img string, masterHost string, masterPort string, scenario string, workerId string) *corev1.Pod {
	gsec := int64(30)
	name := "locust-worker-" + scenario + "-" + workerId

	labels := map[string]string{
		"module":   "performance-testing",
		"app":      "locust-worker",
		"scenario": scenario,
	}

	worker := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &gsec,
			RestartPolicy:                 corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  name,
					Image: img,
					Command: []string{
						"locust",
					},
					Args: []string{
						"--worker",
						"--master-host",
						masterHost,
						"--master-port",
						masterPort,
						"--loglevel",
						"DEBUG",
						"--exit-code-on-error",
						"0",
						"--logfile",
						"/tmp/worker.log",
					},
					ImagePullPolicy: corev1.PullAlways,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2048Mi"),
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"grep",
									"locust.main: Connected to locust master",
									"/tmp/worker.log",
								},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       30,
						FailureThreshold:    100,
					},
				},
			},
		},
	}
	return &worker
}
