package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	cloudbuildpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"cloud.google.com/go/container/apiv1/containerpb"
	"cloud.google.com/go/pubsub"
	run "cloud.google.com/go/run/apiv2"
	runpb "cloud.google.com/go/run/apiv2/runpb"

	// dashboard "cloud.google.com/go/monitoring/dashboard/apiv1"
	// dashboardpb "cloud.google.com/go/monitoring/dashboard/apiv1/dashboardpb"
	"cloud.google.com/go/storage"
	wfexecpb "cloud.google.com/go/workflows/executions/apiv1/executionspb"

	"com.google.gtools/pt-admin/api"
	"com.google.gtools/pt-admin/internal/helper"
	"github.com/gin-gonic/gin"

	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ProvisionWf struct {
	// The url of Cloud Run
	Url            string         `json:"url"`
	ProjectId      string         `json:"projectId"`
	Region         string         `json:"region"`
	VPC            VPC            `json:"vpc"`
	ServiceAccount ServiceAccount `json:"serviceAccount"`
	GKEs           []GKE          `json:"gkes"`
	TaskRequest    TaskRequest    `json:"taskRequest,omitempty"`
}

type VPC struct {
	ProjectId string `json:"projectId"`
	Network   string `json:"network"`
	Mtu       int32  `json:"mtu"`
}

type ServiceAccount struct {
	ProjectId string `json:"projectId"`
	AccountId string `json:"accountId"`
	Key       string `json:"key,omitempty"`
}

type GKE struct {
	ProjectId   string `json:"projectId"`
	Cluster     string `json:"cluster"`
	Location    string `json:"location"`
	Network     string `json:"network"`
	Subnetwork  string `json:"subnetwork"`
	IsMater     string `json:"isMaster"`
	AccountId   string `json:"accountId"`
	Endpoint    string `json:"endpoint,omitempty"`
	Certificate string `json:"certificate,omitempty"`
	OperationId string `json:"operationId,omitempty"`
	Status      string `json:"status,omitempty"`
}

type TaskRequest struct {
	ProjectId  string `json:"projectId"`
	NumOfUsers int    `json:"numOfUsers"`
	Duration   int    `json:"duration"`
	RampUp     int    `json:"rampUp"`
	TargetUrl  string `json:"targetUrl"`
	// locust;jmeter
	Executor string `json:"executor"`
	IsLocal  bool   `json:"isLocal"`
	// Distributed worker: {region: workerNum}
	Worker4Task   map[string]int `json:"worker4Task,omitempty"`
	Script4Task   string         `json:"script4Task"`
	ArchiveBucket string         `json:"archiveBucket"`
}

/////////////////////
// Operations for provison infrastructure
// 0. Build container images for the following testing task
// 1. Create a VPC network if not exist* - do it when install Pt-Admin
// 2. Create a Service Account for the GKE Autopilot cluster*  - do it when install Pt-Admin

// 3. Create a GKE Autopilot cluster
// 4. Retrieve the GKE Autopilot cluster's CA certificate & IP address
// 5. Configure Storage Class in the GKE Autopilot cluster
// 6. Configure PV in the GKE Autopilot cluster
// 7. Binding service sccount with GSA (Workload Identity)
// 8. Deploy pt-operator to the GKE Autopilot cluster

//7. Apply PtTask to the GKE Autopilot cluster
/////////////////////

func CreateVPC(ctx context.Context, c *gin.Context) (*VPC, error) {
	l := log.FromContext(ctx).WithName("CreateVPC")
	ti := &helper.TInfra{}
	var input VPC
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return &VPC{}, err
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		return &VPC{}, err
	}
	l.Info("Unmarshal buf", "buf", input)
	err = ti.CreateVPCNetwork(ctx, input.ProjectId, input.Network, input.Mtu)
	if err != nil {
		return &VPC{}, err
	}
	return &input, nil
}

func CreateServiceAccount(ctx context.Context, c *gin.Context) (*ServiceAccount, error) {
	ti := &helper.TInfra{}
	var input ServiceAccount
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return &ServiceAccount{}, err
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		return &ServiceAccount{}, err
	}
	// Create Service Account
	_, err = ti.CreateServiceAccount(ctx, input.ProjectId, input.AccountId)
	if err != nil {
		return &ServiceAccount{}, err
	}
	// Grant roles to Service Account
	err = ti.SetIamPolicies2SA(ctx, input.ProjectId, input.AccountId)
	if err != nil {
		return &ServiceAccount{}, err
	}

	return &input, nil
}

func CreateServiceAccountKey(ctx context.Context, c *gin.Context) (*ServiceAccount, error) {
	ti := &helper.TInfra{}
	var input ServiceAccount
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return &input, err
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		return &input, err
	}
	key, err := ti.CreateServiceAccountKey(ctx, input.ProjectId, input.AccountId)
	if err != nil {
		return &input, err
	}
	input.Key = string(key.PrivateKeyData)
	return &input, nil
}

func CreateGKEAutopilotCluster(ctx context.Context, c *gin.Context) ([]GKE, error) {
	ti := &helper.TInfra{}
	var input []GKE
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return []GKE{}, err
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		return []GKE{}, err
	}
	// Return array of GKE with operationId, if operationId is empty, it means the cluster is existed
	var observed []GKE
	for _, gke := range input {
		op, err := ti.CreateAutopilotCluster(ctx, gke.ProjectId, gke.Cluster, gke.Location, gke.Network, gke.Subnetwork, gke.AccountId)
		if err != nil {
			return []GKE{}, err
		}
		if op.Name != "" {
			gke.OperationId = op.Name
			observed = append(observed, gke)
		} else {
			gke.Status = "DONE"
			observed = append(observed, gke)
		}
	}
	return observed, nil

}

func CheckClusterStatus(ctx context.Context, c *gin.Context) ([]GKE, error) {
	l := log.FromContext(ctx).WithName("CheckClusterStatus")
	ti := &helper.TInfra{}
	var input []GKE
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return input, err
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return input, err
	}
	var observed []GKE
	for _, gke := range input {
		if gke.Status != "DONE" {
			op, err := ti.AutopilotClusterStatus(ctx, gke.ProjectId, gke.Location, gke.OperationId)
			if err != nil {
				l.Error(err, "AutopilotClusterStatus")
				return input, err
			}

			if op.Status == containerpb.Operation_DONE {
				gke.Status = "DONE"
			} else {
				gke.Status = "IN_PROGRESS"
			}
			observed = append(observed, gke)
		}
	}

	// Check if all cluster is ready
	isAll := true
	for _, gke := range observed {
		if gke.Status != "DONE" {
			isAll = false
			break
		}
	}
	if isAll {
		return []GKE{}, nil
	} else {
		return observed, nil
	}

}

func RetrieveClusterInfo(ctx context.Context, c *gin.Context) ([]GKE, error) {
	ti := &helper.TInfra{}
	var input []GKE
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return input, err
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		return input, err
	}
	var observed []GKE
	for _, gke := range input {
		ca, err := ti.GetClusterCaCertificate(ctx, gke.ProjectId, gke.Cluster, gke.Location)
		if err != nil {
			return input, err
		}
		gke.Certificate = ca
		ip, err := ti.GetClusterEndpoint(ctx, gke.ProjectId, gke.Cluster, gke.Location)
		if err != nil {
			return input, err
		}
		gke.Endpoint = ip
		observed = append(observed, gke)
	}
	return observed, nil
}

func ApplyManifest(ctx context.Context, c *gin.Context) error {
	l := log.FromContext(ctx).WithName("ApplyManifest")
	var gke GKE
	file := c.Param("file")

	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return err
	}
	err = json.Unmarshal(buf, &gke)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return err
	}

	if file == "pt-operator.yaml" || file == "sc.yaml" {
		nfile, err := replaceEnvs(ctx, file, gke.AccountId, gke.ProjectId, gke.Network, "")
		if err != nil {
			l.Error(err, "replaceEnvs", "file", file)
			return err
		}
		file = nfile
	} else {
		file = "/manifests/" + file
	}
	err = helper.CreateYaml2K8s(ctx, gke.Certificate, gke.Endpoint, file)
	if err != nil {
		l.Error(err, "ApplyYaml2K8s", "file", file)
		return err
	}

	return nil
}

func BindingWorkloadIdentity(ctx context.Context, c *gin.Context) error {
	ti := &helper.TInfra{}
	var sa ServiceAccount
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(buf, &sa)
	if err != nil {
		return err
	}

	return ti.SetIamPolicies2KSA(ctx, sa.ProjectId, "pt-system", "pt-operator-controller-manager")

}

func replaceEnvs(ctx context.Context, file, sa, projectId, network, ptName string) (string, error) {
	l := log.FromContext(ctx).WithName("replaceEnvs")
	buf, err := ioutil.ReadFile("/manifests/" + file)
	if err != nil {
		l.Error(err, "ReadFile")
		return "", err
	}
	newContents := string(buf)
	if sa != "" {
		newContents = strings.Replace(newContents, "${SA_NAME}", sa, -1)
	}
	if projectId != "" {
		newContents = strings.Replace(newContents, "${PROJECT_ID}", projectId, -1)
	}
	if network != "" {
		newContents = strings.Replace(newContents, "${NETWORK}", network, -1)
	}
	if ptName != "" {
		newContents = strings.Replace(newContents, "${PT_TASK_NAME}", ptName, -1)
	}

	// TODO: It's a specific path "/var/tmp/" for Cloud Run
	out := "/var/tmp/" + file + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".yaml"
	err = ioutil.WriteFile(out, []byte(newContents), 0)
	if err != nil {
		l.Error(err, "WriteFile")
		return "", err
	}

	return out, nil
}

func GetPVCStatus(ctx context.Context, c *gin.Context) (map[string]string, error) {
	l := log.FromContext(ctx).WithName("GetPVCStatus")
	var gke GKE
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return map[string]string{}, err
	}
	err = json.Unmarshal(buf, &gke)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return map[string]string{}, err
	}

	pvc, err := helper.StatusOfPVC(ctx, gke.Certificate, gke.Endpoint, "pt-system", "bzt-filestore-pvc")
	if err != nil {
		l.Error(err, "StatusOfPVC")
		return map[string]string{}, err
	}
	if pvc.Status.Phase == "Bound" {
		return map[string]string{"status": "DONE"}, nil
	} else {
		return map[string]string{"status": "IN_PROGRESS"}, nil
	}

}

func GetPtOperatorStatus(ctx context.Context, c *gin.Context) (map[string]string, error) {
	l := log.FromContext(ctx).WithName("GetPtOperatorStatus")
	var gke GKE
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return map[string]string{}, err
	}
	err = json.Unmarshal(buf, &gke)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return map[string]string{}, err
	}

	dep, err := helper.StatusOfDeployment(ctx, gke.Certificate, gke.Endpoint, "pt-system", "pt-operator-controller-manager")
	if err != nil {
		l.Error(err, "StatusOfPod")
		return map[string]string{}, err
	}
	if dep.Status.Replicas == 1 {
		return map[string]string{"status": "DONE"}, nil
	} else {
		return map[string]string{"status": "IN_PROGRESS"}, nil
	}

}

// Record the execution of a workflow
func RecordWorkflow(ctx context.Context, c *gin.Context) error {
	l := log.FromContext(ctx).WithName("RecordWorkflow")
	var pwf ProvisionWf
	// The name of the workflow to be executed
	wf := c.Param("wf")
	eId := c.Param("executionId")
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return err
	}
	err = json.Unmarshal(buf, &pwf)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return err
	}

	// Save executionId to Firestore
	_, err = helper.Insert(ctx, pwf.ProjectId, "pt-transactions", helper.PtTransaction{
		Id:             eId,
		WorkflowName:   wf,
		Input:          pwf,
		StatusWorkflow: "ACTIVE",
	})
	if err != nil {
		l.Error(err, "failed to insert PtTransaction to Firestore")
		return err
	}
	return nil

}

// Execute the workflow to provision all related resources and apply PtTask to run a Performance Test
func ExecWorkflow(ctx context.Context, c *gin.Context) error {
	l := log.FromContext(ctx).WithName("ExecWorkflow")
	ti := &helper.TInfra{}
	var pwf ProvisionWf
	// The name of the workflow to be executed
	wf := c.Param("wf")
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return err
	}
	err = json.Unmarshal(buf, &pwf)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return err
	}
	err = ti.ExecWorkflow(ctx, pwf.ProjectId, wf, buf)
	if err != nil {
		l.Error(err, "ti.ExecWorkflow")
		return err
	}

	return nil

}

func StatusWorkflow(ctx context.Context, c *gin.Context) (map[string]string, error) {
	l := log.FromContext(ctx).WithName("StatusWorkflow")
	ti := &helper.TInfra{}
	ex, err := ti.StatusWorkflow(ctx, c.Param("projectId"), c.Param("region"), c.Param("workflow"), c.Param("executionId"))
	if err != nil {
		l.Error(err, "ti.StatusWorkflow")
		return map[string]string{}, err
	}
	l.Info("StatusWorkflow", "startTime", ex.StartTime.String())
	return map[string]string{"status": wfexecpb.Execution_State_name[int32(ex.State.Number())]}, nil

}

func CreateDashboard(ctx context.Context, c *gin.Context) (string, error) {
	l := log.FromContext(ctx).WithName("CreateDashboard")
	dId := ""
	projectId := c.Param("projectId")
	executionId := c.Param("executionId")
	// var token *oauth2.Token
	url := fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/dashboards", projectId)
	// Get access token
	// curl "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token" \
	// --header "Metadata-Flavor: Google"
	// scopes := []string{
	// 	"https://www.googleapis.com/auth/cloud-platform",
	// }
	// if credentials, err := auth.FindDefaultCredentials(ctx, scopes...); err == nil {
	// 	token, err = credentials.TokenSource.Token()
	// 	if err != nil {
	// 		l.Error(err, "credentials.TokenSource.Token")
	// 		return dId, err
	// 	}
	// }
	aReq, err := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		l.Error(err, "http.NewRequest")
		return dId, err
	}
	aReq.Header.Add("Metadata-Flavor", "Google")
	aClient := &http.Client{}
	aResp, err := aClient.Do(aReq)
	if err != nil {
		l.Error(err, "aClient.Do")
		return dId, err
	}
	defer aResp.Body.Close()
	aBody, err := ioutil.ReadAll(aResp.Body)
	if err != nil {
		l.Error(err, "ioutil.ReadAll")
		return dId, err
	}
	var aRespBody map[string]interface{}
	err = json.Unmarshal(aBody, &aRespBody)
	if err != nil {
		l.Error(err, "json.Unmarshal")
		return dId, err
	}
	// curl -d @my-dashboard.json
	// -H "Authorization: Bearer $(gcloud auth print-access-token)"
	// -H 'Content-Type: application/json'
	// -X POST https://monitoring.googleapis.com/v1/projects/${PROJECT_ID}/dashboards
	// net/http post
	dFile, err := replaceEnvs(ctx, "dashboard.json", "", "", "", executionId)
	if err != nil {
		l.Error(err, "replaceEnvs")
		return dId, err
	}
	dash, err := ioutil.ReadFile(dFile)
	if err != nil {
		l.Error(err, "ioutil.ReadFile")
		return dId, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(dash))
	if err != nil {
		l.Error(err, "http.NewRequest")
		return dId, err
	}
	req.Header.Set("Authorization", "Bearer "+fmt.Sprintf("%v", aRespBody["access_token"]))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	//eg: https://console.cloud.google.com/monitoring/dashboards/builder/db50a4bf-ae14-441f-812d-b01ea471ab42?project=play-api-service
	if err != nil {
		l.Error(err, "client.Do")
		return dId, err
	}
	defer resp.Body.Close()
	l.Info("response Status:", "status", resp.Status)
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return dId, err
	}
	// json, cloud be map style
	var mp map[string]interface{}
	err = json.Unmarshal(buf, &mp)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return dId, err
	}
	//"name": "projects/374299782509/dashboards/fd877f39-6505-4788-86b8-89b49bf43d4a"
	if mp["name"] != nil {
		dName := strings.Split(fmt.Sprintf("%v", mp["name"]), "/")
		dId = dName[len(dName)-1]
		l.Info("Dashboard created", "id", dId)

	}
	// Dashboard URL
	dUrl := "https://console.cloud.google.com/monitoring/dashboards/builder/" + dId + "?project=" + projectId

	// Save dashboard URL to Firestore
	_, err = helper.UpdateDashboardUrl(ctx, projectId, "pt-transactions", executionId, dUrl)
	if err != nil {
		l.Error(err, "failed to update dashboardUrl to Firestore")
	}
	return dUrl, nil
}

func PreparenApplyPtTask(ctx context.Context, c *gin.Context) (*api.PtTask, error) {
	l := log.FromContext(ctx).WithName("PreparePtTask")
	executionId := c.Param("executionId")
	var pwf ProvisionWf
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return &api.PtTask{}, err
	}
	err = json.Unmarshal(buf, &pwf)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return &api.PtTask{}, err
	}
	tr := pwf.TaskRequest

	// Calcaulate the number of pods
	var pt *api.PtTask
	// randomId := strconv.FormatInt(time.Now().UnixNano(), 10)
	name := "pt-task-" + executionId
	scenario := "scenario-" + executionId
	taurusImage := fmt.Sprintf("asia-docker.pkg.dev/%s/%s-pt-images/taurus-base", tr.ProjectId, tr.ProjectId)
	locustImage := fmt.Sprintf("asia-docker.pkg.dev/%s/%s-pt-images/locust-worker", tr.ProjectId, tr.ProjectId)

	// gzPath, _ := save2gcs(ctx, tr.ProjectId, tr.Script4Task)
	if tr.Executor == "locust" {
		l.Info("Locust PtTask")
		if tr.IsLocal {
			l.Info("Local Locust PtTask")
			pt = &api.PtTask{
				Kind:       "PtTask",
				APIVersion: "perftest.com.google.gtools/v1",
				Metadata: api.MyObjectMeta{
					Name:      name,
					Namespace: "pt-system",
					Annotations: map[string]string{
						"pttask/executionId": executionId,
					},
				},
				Spec: api.PtTaskSpec{
					Type: "Local",
					Execution: []api.PtTaskExecution{
						{
							Executor:    tr.Executor,
							Concurrency: int(tr.NumOfUsers),
							HoldFor:     strconv.Itoa(tr.Duration) + "m",
							RampUp:      strconv.Itoa(tr.RampUp) + "s",
							Scenario:    scenario,
							Master:      true,
							Workers:     int(tr.NumOfUsers / 1000),
						},
					},
					Scenarios: map[string]api.PtTaskScenario{
						scenario: {
							Script:         "locustfile.py",
							DefaultAddress: tr.TargetUrl,
						},
					},
					Images: map[string]api.PtTaskImages{
						scenario: {
							MasterImage: taurusImage,
							WorkerImage: locustImage,
						},
					},
					TestingOutput: api.PtTaskTestingOutput{
						LogDir: "/taurus-logs",
						Bucket: tr.ArchiveBucket,
					},
				},
			}
		} else {
			l.Info("Distributed Locust PtTask")
			// TODO: add distributed locust?!
		}
	} else if tr.Executor == "jmeter" {
		l.Info("Jmeter PtTask, still under development")
	}

	for _, gke := range pwf.GKEs {
		if gke.IsMater == "true" {

			//Get GKE credential
			ti := &helper.TInfra{}
			ca, err := ti.GetClusterCaCertificate(ctx, gke.ProjectId, gke.Cluster, gke.Location)
			if err != nil {
				return pt, err
			}
			gke.Certificate = ca
			ip, err := ti.GetClusterEndpoint(ctx, gke.ProjectId, gke.Cluster, gke.Location)
			if err != nil {
				return pt, err
			}
			gke.Endpoint = ip

			// Write yaml file
			buf, err := yaml.Marshal(pt)
			if err != nil {
				l.Error(err, "json.Marshal", "pt", pt)
				return pt, err
			}
			file := fmt.Sprintf("/var/tmp/pt-task-%s.yaml", executionId)
			err = ioutil.WriteFile(file, buf, 0644)
			if err != nil {
				l.Error(err, "ioutil.WriteFile", "file", file)
				return pt, err
			}
			l.Info("WriteFile", "file", file, "buf", string(buf))
			err = helper.CreateYaml2K8s(ctx, gke.Certificate, gke.Endpoint, file)
			if err != nil {
				l.Error(err, "CreateYaml2K8s")
				return pt, err
			}
		}
	}

	// Save PtTask to Firestore
	_, err = helper.UpdatePtTask(ctx, tr.ProjectId, "pt-transactions", executionId, *pt)
	if err != nil {
		l.Error(err, "failed to update PtTask to Firestore")
	}
	return pt, nil
}

// Build images from content of script and base image, and go through the following processes:
//  1. extract script files from tgz
//  2. add docker file
//  3. tar the files to tgz
//  4. save the tgz to gcs
//  5. submit a cloudbuild job to build the image
func BuildImage(ctx context.Context, c *gin.Context) error {
	l := log.FromContext(ctx)
	var tr TaskRequest
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return err
	}
	err = json.Unmarshal(buf, &tr)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return err
	}

	taurusImage := fmt.Sprintf("asia-docker.pkg.dev/%s/%s-pt-images/taurus-base", tr.ProjectId, tr.ProjectId)
	locustImage := fmt.Sprintf("asia-docker.pkg.dev/%s/%s-pt-images/locust-worker", tr.ProjectId, tr.ProjectId)
	tgzFile := fmt.Sprintf("/var/tmp/pt-scripts-%d.tgz", time.Now().UnixNano())
	destDir := fmt.Sprintf("/var/tmp/%d", time.Now().UnixNano())
	tgz2Gcs := fmt.Sprintf("/var/tmp/pt-scripts-to-gcs-%d.tgz", time.Now().UnixNano())

	l.Info("decode string", "txt", tr.Script4Task)
	txt, err := base64.StdEncoding.DecodeString(tr.Script4Task)
	if err != nil {
		l.Error(err, "failed to decode text")
		return err
	}
	l.Info("extract script files from tgz")
	err = ioutil.WriteFile(tgzFile, []byte(txt), 0644)
	if err != nil {
		l.Error(err, "ioutil.WriteFile")
		return err
	}
	err = untarzFile(ctx, tgzFile, destDir+"/scripts")
	if err != nil {
		l.Error(err, "untarzFile", "tgzFile", tgzFile)
		return err
	}

	///////////////////////////////
	// Build Locust image
	l.Info("add docker file for Locust")
	err = copyFile(ctx, "/manifests/Dockerfile.locust", destDir+"/Dockerfile")
	if err != nil {
		return err
	}

	l.Info("tar the files to tgz")
	err = tarzFile(ctx, destDir, tgz2Gcs)
	if err != nil {
		l.Error(err, "tarzFile", "tgz2Gcs", tgz2Gcs)
		return err
	}

	l.Info("save the tgz to gcs", "src", tgz2Gcs)
	obj, err := save2gcs(ctx, tr.ProjectId, tgz2Gcs)
	if err != nil {
		l.Error(err, "save2gcs", "tgz2Gcs", tgz2Gcs)
		return err
	}
	l.Info("save2gcs was done", "obj", obj)

	l.Info("submit a cloudbuild job to build the image for Locust")
	err = submitCloudBuildJob(ctx, tr.ProjectId, obj, locustImage)
	if err != nil {
		l.Error(err, "submitCloudBuildJob", "image", locustImage)
		return err
	}

	///////////////////////////////
	// Build Taurus image
	l.Info("add docker file for Taurus")
	err = copyFile(ctx, "/manifests/Dockerfile.taurus", destDir+"/Dockerfile")
	if err != nil {
		return err
	}

	l.Info("tar the files to tgz")
	err = tarzFile(ctx, destDir, tgz2Gcs)
	if err != nil {
		l.Error(err, "tarzFile", "tgz2Gcs", tgz2Gcs)
		return err
	}

	l.Info("save the tgz to gcs")
	obj, err = save2gcs(ctx, tr.ProjectId, tgz2Gcs)
	if err != nil {
		l.Error(err, "save2gcs", "tgz2Gcs", tgz2Gcs)
		return err
	}

	l.Info("submit a cloudbuild job to build the image for Taurus")
	err = submitCloudBuildJob(ctx, tr.ProjectId, obj, taurusImage)
	if err != nil {
		l.Error(err, "submitCloudBuildJob", "image", taurusImage)
		return err
	}
	/////////////////////
	return nil
}

func copyFile(ctx context.Context, src string, dst string) error {
	l := log.FromContext(ctx)
	input, err := ioutil.ReadFile(src)
	if err != nil {
		l.Error(err, "ioutil.ReadFile", "dockerFile", src)
		return err
	}

	err = os.MkdirAll(filepath.Dir(dst), 0755)
	if err != nil {
		l.Error(err, "os.MkdirAll", "dockerFile", dst)
		return err
	}
	err = ioutil.WriteFile(dst, input, 0644)
	if err != nil {
		l.Error(err, "ioutil.WriteFile", "dockerFile", dst)
		return err
	}
	return nil
}

func tarzFile(ctx context.Context, srcDir string, tgzFile string) error {
	l := log.FromContext(ctx)
	var buf bytes.Buffer
	err := os.Chdir(srcDir)
	if err != nil {
		l.Error(err, "os.Chdir", "srcDir", srcDir)
		return err
	}

	// tar > gzip > buf
	zr := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zr)

	// walk through every file in the folder
	filepath.Walk("./", func(file string, fi os.FileInfo, err error) error {
		// generate tar header
		header, err := tar.FileInfoHeader(fi, file)
		if err != nil {
			return err
		}

		// must provide real name
		// (see https://golang.org/src/archive/tar/common.go?#L626)
		header.Name = filepath.ToSlash(file)
		fmt.Println(header.Name)

		// write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		// if not a dir, write file content
		if !fi.IsDir() {
			data, err := os.Open(file)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, data); err != nil {
				return err
			}
		}
		return nil
	})

	// produce tar
	if err := tw.Close(); err != nil {
		return err
	}
	// produce gzip
	if err := zr.Close(); err != nil {
		return err
	}
	//
	fileToWrite, err := os.OpenFile(tgzFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.FileMode(0644))
	if err != nil {
		l.Error(err, "os.OpenFile", "file", tgzFile)
		return nil
	}
	if _, err := io.Copy(fileToWrite, &buf); err != nil {
		l.Error(err, "io.Copy")
		return err
	}
	return nil
}

func untarzFile(ctx context.Context, srcFile string, destDir string) error {
	l := log.FromContext(ctx)
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		l.Error(err, "os.MkdirAll", "destDir", destDir)
		return err
	}
	err = os.Chdir(destDir)
	if err != nil {
		l.Error(err, "os.Chdir", "destDir", destDir)
		return err
	}

	f, err := os.Open(srcFile)
	if err != nil {
		l.Error(err, "os.Open", "srcFile", srcFile)
		return err
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		l.Error(err, "gzip.NewReader", "srcFile", srcFile)
		return err
	}
	defer gzf.Close()

	tarReader := tar.NewReader(gzf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			l.Error(err, "tarReader.Next")
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(header.Name, os.FileMode(header.Mode)); err != nil {
				l.Error(err, "os.MkdirAll", "dir", header.Name)
				return err
			}
		case tar.TypeReg:
			targetFile, err := os.OpenFile(header.Name, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				l.Error(err, "os.OpenFile", "file", header.Name)
				return err
			}
			if _, err := io.Copy(targetFile, tarReader); err != nil {
				l.Error(err, "io.Copy", "file", header.Name)
				return err
			}
			targetFile.Close()
		default:
			l.Info("Unable to extract file type in file", "type", header.Typeflag, "file", header.Name)
		}
	}
	return nil
}

func submitCloudBuildJob(ctx context.Context, projectId string, obj string, image string) error {
	l := log.FromContext(ctx)
	c, err := cloudbuild.NewClient(ctx)
	if err != nil {
		l.Error(err, "unable to create cloudbuild client")
		return err
	}
	defer c.Close()

	req := &cloudbuildpb.CreateBuildRequest{
		Parent:    fmt.Sprintf("projects/%s/locations/global", projectId),
		ProjectId: projectId,
		Build: &cloudbuildpb.Build{
			Source: &cloudbuildpb.Source{
				Source: &cloudbuildpb.Source_StorageSource{
					StorageSource: &cloudbuildpb.StorageSource{
						Bucket: projectId + "_cloudbuild",
						Object: obj,
					},
				},
			},
			Steps: []*cloudbuildpb.BuildStep{
				{
					Name: "gcr.io/cloud-builders/docker",
					Args: []string{
						"build",
						"-t",
						image,
						".",
					},
				},
				{
					Name: "gcr.io/cloud-builders/docker",
					Args: []string{
						"push",
						image,
					},
				},
			},
		},
	}
	_, err = c.CreateBuild(ctx, req)
	if err != nil {
		l.Error(err, "unable to create cloudbuild job")
		return err
	}
	return nil
}

func save2gcs(ctx context.Context, projectId string, srcFile string) (string, error) {
	// save bytes to gcs and return the path
	l := log.FromContext(ctx)
	bucket := projectId + "_cloudbuild"
	dstFile := "source/pt-scripts-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".tgz"
	fh, err := os.Open(srcFile)
	if err != nil {
		l.Error(err, "unable to open source file", "src", srcFile)
		return dstFile, err
	}
	l.Info("file opened", "file", srcFile)

	client, err := storage.NewClient(ctx)
	if err != nil {
		l.Error(err, "unable to create GCS client")
		return dstFile, err
	}
	wc := client.Bucket(bucket).Object(dstFile).NewWriter(ctx)
	if _, err = io.Copy(wc, fh); err != nil {
		l.Error(err, "unable to copy file to GCS", "src", srcFile, "dst", dstFile)
		return dstFile, err
	}
	l.Info("file copied to GCS", "src", srcFile, "dst", dstFile)

	if err := wc.Close(); err != nil {
		l.Error(err, "unable to close GCS writer", "src", srcFile, "dst", dstFile)
		return dstFile, err
	}
	l.Info("file saved to GCS", "file", dstFile)
	return dstFile, nil
}

func DestroyResources(ctx context.Context, c *gin.Context) error {
	l := log.FromContext(ctx)
	projectId := c.Param("projectId")
	executionId := c.Param("executionId")
	ti := &helper.TInfra{}

	trans, err := helper.Read(ctx, projectId, "pt-transactions", executionId)
	if err != nil {
		l.Error(err, "unable to read transaction", "projectId", projectId, "executionId", executionId)
		return err
	}
	l.Info("transaction read", "projectId", projectId, "executionId", executionId, "trans.id", trans.Id)

	// TODO: delete all resources created by for PT
	// delete GKE Autopilot cluster
	var input ProvisionWf
	buf, err := json.Marshal(trans.Input)
	if err != nil {
		l.Error(err, "unable to marshal transaction input", "projectId", projectId, "executionId", executionId)
	}
	err = json.Unmarshal(buf, &input)
	if err != nil {
		l.Error(err, "unable to unmarshal transaction input", "projectId", projectId, "executionId", executionId)
	}

	for _, gke := range input.GKEs {
		l.Info("deleting GKE cluster", "cluster", gke.Cluster, "location", gke.Location)
		_, err = ti.DeleteAutopilotCluster(ctx, gke.ProjectId, gke.Cluster, gke.Location)
		if err != nil {
			l.Error(err, "ti.DeleteAutopilotCluster", "cluster", gke.Cluster, "location", gke.Location)
			return err
		}
	}
	//TODO: delete Service Account ?

	//TODO: delete VPC ?

	return nil
}

func ProtoCreatePtTask(ctx context.Context, c *gin.Context) error {
	l := log.FromContext(ctx).WithName("ProtoCreatePtTask")
	l.Info("Reveived request from client to create a PtTask")

	var ptr api.PtTaskRequest
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		l.Error(err, "io.ReadAll")
		return err
	}
	err = json.Unmarshal(buf, &ptr)
	if err != nil {
		l.Error(err, "json.Unmarshal", "buf", string(buf))
		return err
	}

	// Get URL of CloudRun service
	runService := os.Getenv("K_SERVICE")
	projectId := os.Getenv("PROJECT_ID")
	location := os.Getenv("LOCATION")
	cloudrun, err := run.NewServicesClient(ctx)
	if err != nil {
		l.Error(err, "unable to create cloudrun client")
		return err
	}
	req := &runpb.GetServiceRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/services/%s", projectId, location, runService),
	}
	resp, err := cloudrun.GetService(ctx, req)
	if err != nil {
		l.Error(err, "unable to get cloudrun service")
		return err
	}

	// Construct the input for the workflow
	network := "pt-vpc-" + randString()
	pwf := ProvisionWf{
		Url:       resp.Uri,
		ProjectId: projectId,
		GKEs: []GKE{
			{
				AccountId:  "pt-service-account",
				ProjectId:  projectId,
				Cluster:    "pt-cluster-" + randString(),
				IsMater:    "true",
				Location:   location,
				Network:    network,
				Subnetwork: network,
			},
		},
		VPC: VPC{
			Mtu:       1460,
			Network:   network,
			ProjectId: projectId,
		},
		ServiceAccount: ServiceAccount{
			AccountId: "pt-service-account",
			ProjectId: projectId,
		},
		TaskRequest: TaskRequest{
			ProjectId:     projectId,
			ArchiveBucket: "pt-results-archive",
			Executor:      "locust",
			NumOfUsers:    int(ptr.TotalUsers),
			Duration:      int(ptr.Duration),
			RampUp:        int(ptr.RampUp),
			TargetUrl:     *ptr.TargetUrl,
			Script4Task:   string(ptr.Scripts.Data),
		},
	}
	ibuf, err := json.Marshal(pwf)
	if err != nil {
		l.Error(err, "json.Marshal")
		return err
	}

	// Send message to PubSub and trigger provisoning workflow
	topic := "pt-provision-wf"
	client, err := pubsub.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "unable to create pubsub client")
		return err
	}
	ret := client.Topic(topic).Publish(ctx, &pubsub.Message{
		Data: ibuf,
	})
	_, err = ret.Get(ctx)
	if err != nil {
		l.Error(err, "unable to publish message to pubsub")
		return err
	}

	return nil
}

// Generate a random string of a-z chars with len = 6
func randString() string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
