package helper

import (
	"context"

	"cloud.google.com/go/firestore"
	"com.google.gtools/pt-admin/api"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PtTransaction struct {
	// Generated Id
	CorrelationId string `json:"correlationId"`
	// Execution Id of the Workflow
	ExecutionId string `json:"executionId"`
	// Name of the Workflow
	WorkflowName string `json:"workflowName"`
	// Input of the Workflow
	Input interface{} `json:"input"`
	// Status of the Workflow
	WorkflowStatus string `json:"workflowStatus"`
	// PtTask
	PtTask api.PtTask `json:"ptTask,omitempty"`
	// Status of Performance Testing Task
	PtTaskStatus string `json:"ptTaskStatus,omitempty"`

	// Created time
	Created *timestamppb.Timestamp `json:"created,omitempty"`
	// Finished time
	Finished *timestamppb.Timestamp `json:"finished,omitempty"`
	// last updated time
	LastUpdated *timestamppb.Timestamp `json:"last_updated,omitempty"`
	// Link to metrics dashboard
	MetricsLink *string `json:"metrics_link,omitempty"`
	// Link to logs
	LogsLink *string `json:"logs_link,omitempty"`
	// Link to download the test results
	DownloadLink *string `json:"download_link,omitempty"`

	// !!!Using pointer to avoid sync.Mutex due to generate through proto!!!
	OriginTaskRequest  *api.PtTaskRequest  `json:"originTaskRequest,omitempty"`
	OriginLaunchedTask *api.PtLaunchedTask `json:"originLaunchedTask,omitempty"`
}

type ProvisionWf struct {
	// The url of Cloud Run
	Url            string         `json:"url"`
	ProjectId      string         `json:"projectId"`
	Region         string         `json:"region"`
	VPC            VPC            `json:"vpc"`
	ServiceAccount ServiceAccount `json:"serviceAccount"`
	GKEs           []GKE          `json:"gkes"`
	TaskRequest    TaskRequest    `json:"taskRequest,omitempty"`
	CorrelationId  string         `json:"correlationId,omitempty"`

	// !!!Using pointer to avoid sync.Mutex due to generate through proto!!!
	OriginTaskRequest  *api.PtTaskRequest  `json:"originTaskRequest,omitempty"`
	OriginLaunchedTask *api.PtLaunchedTask `json:"originLaunchedTask,omitempty"`
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

// Store the value into the collection in Firestore
func Insert(ctx context.Context, projectId string, collection string, ptt PtTransaction) (*firestore.WriteResult, error) {
	l := log.FromContext(ctx).WithName("Insert")
	l.Info("Insert a value into a collection", "collection", collection, "value", ptt)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	ret, err := client.Collection(collection).Doc(ptt.CorrelationId).Create(ctx, ptt)
	if err != nil {
		l.Error(err, "failed to add doc")
		return nil, err
	}
	return ret, nil
}

// Read the value from the collection in Firestore
func Read(ctx context.Context, projectId string, collection string, id string) (*PtTransaction, error) {
	l := log.FromContext(ctx).WithName("Read")
	l.Info("Read a value from a collection", "collection", collection)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	snapshot, err := client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		l.Error(err, "failed to get doc")
		return nil, err
	}
	var pt PtTransaction
	err = snapshot.DataTo(&pt)
	if err != nil {
		l.Error(err, "failed to convert data")
		return nil, err
	}
	return &pt, nil
}

func ReadAll(ctx context.Context, projectId string, collection string) ([]PtTransaction, error) {
	l := log.FromContext(ctx).WithName("ReadAll")
	l.Info("Read all values from a collection", "collection", collection)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	ds, err := client.Collection(collection).Documents(ctx).GetAll()
	if err != nil {
		l.Error(err, "failed to get docs")
		return nil, err
	}
	var pts []PtTransaction
	for _, d := range ds {
		var pt PtTransaction
		err = d.DataTo(&pt)
		if err != nil {
			l.Error(err, "failed to convert data")
		}
		pts = append(pts, pt)
	}
	return pts, nil
}

// Update dashboard url of the collection in Firestore
func UpdateDashboardUrl(ctx context.Context, projectId, collection, id, dUrl string) (*firestore.WriteResult, error) {
	l := log.FromContext(ctx).WithName("UpdateDashboardUrl")
	l.Info("Update dashboard url", "collection", collection, "id", id, "url", dUrl)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	snapshot, err := client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		l.Error(err, "failed to get doc")
		return nil, err
	}
	var orgin PtTransaction
	err = snapshot.DataTo(&orgin)
	if err != nil {
		l.Error(err, "failed to convert data")
		return nil, err
	}
	orgin.MetricsLink = &dUrl
	orgin.LastUpdated = timestamppb.Now()

	// Update the value
	return client.Collection(collection).Doc(id).Set(ctx, orgin)

}

func UpdatePtTask(ctx context.Context, projectId, collection, id string, ptTask api.PtTask) (*firestore.WriteResult, error) {
	l := log.FromContext(ctx).WithName("UpdatePtTask")
	l.Info("Update PtStatus", "collection", collection, "id", id, "pt", ptTask)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	snapshot, err := client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		l.Error(err, "failed to get doc")
		return nil, err
	}
	var orgin PtTransaction
	err = snapshot.DataTo(&orgin)
	if err != nil {
		l.Error(err, "failed to convert data")
		return nil, err
	}
	orgin.PtTask = ptTask
	orgin.LastUpdated = timestamppb.Now()
	// Update the value
	return client.Collection(collection).Doc(id).Set(ctx, orgin)

}

// Update the status of the collection in Firestore, Could be done in pt-operator or call back from pt-operator
func UpdatePtTaskStatus(ctx context.Context, projectId, collection, id, status string) (*firestore.WriteResult, error) {
	l := log.FromContext(ctx).WithName("UpdateStatusPtTask")
	l.Info("Update status of PtStatus", "collection", collection, "id", id, "status", status)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	snapshot, err := client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		l.Error(err, "failed to get doc")
		return nil, err
	}
	var orgin PtTransaction
	err = snapshot.DataTo(&orgin)
	if err != nil {
		l.Error(err, "failed to convert data")
		return nil, err
	}
	orgin.PtTaskStatus = status
	orgin.LastUpdated = timestamppb.Now()

	// Update the value
	return client.Collection(collection).Doc(id).Set(ctx, orgin)

}

func UpdateWorkflowStatus(ctx context.Context, projectId, collection, id, status string) (*firestore.WriteResult, error) {
	l := log.FromContext(ctx).WithName("UpdateWorkflowStatus")
	l.Info("Update status of Workflow", "collection", collection, "id", id, "status", status)
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		l.Error(err, "firestore new error")
	}
	defer client.Close()

	snapshot, err := client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		l.Error(err, "failed to get doc")
		return nil, err
	}
	var orgin PtTransaction
	err = snapshot.DataTo(&orgin)
	if err != nil {
		l.Error(err, "failed to convert data")
		return nil, err
	}
	orgin.WorkflowStatus = status
	orgin.LastUpdated = timestamppb.Now()

	// Update the value
	return client.Collection(collection).Doc(id).Set(ctx, orgin)
}
