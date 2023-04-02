package helper

import (
	"context"

	"cloud.google.com/go/firestore"
	"com.google.gtools/pt-admin/api"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PtTransaction struct {
	// Execution Id of the Workflow
	Id string `json:"id"`
	// Name of the Workflow
	WorkflowName string `json:"workflowName"`
	// Input of the Workflow
	Input interface{} `json:"input"`
	// Status of the Workflow
	StatusWorkflow string `json:"statusWorkflow"`
	// PtTask
	PtTask api.PtTask `json:"ptTask,omitempty"`
	// Status of Performance Testing Task
	StatusPtTask string `json:"statusPtTask,omitempty"`
	// Dashboard URL of the Performance Testing Task
	DashboardUrl string `json:"dashboardUrl,omitempty"`
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

	ret, err := client.Collection(collection).Doc(ptt.Id).Create(ctx, ptt)
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
	orgin.DashboardUrl = dUrl

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

	// Update the value
	return client.Collection(collection).Doc(id).Set(ctx, orgin)

}

func UpdateStatusPtTask(ctx context.Context, projectId, collection, id, status string) (*firestore.WriteResult, error) {
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
	orgin.StatusPtTask = status

	// Update the value
	return client.Collection(collection).Doc(id).Set(ctx, orgin)

}
