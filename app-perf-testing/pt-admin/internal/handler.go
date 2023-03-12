package internal

import (
	"context"

	"com.google.gtools/pt-admin/api"
	"github.com/gin-gonic/gin"
)

func PtTask(ctx context.Context, c *gin.Context) *api.PtTask {

	data := api.PtTask{
		TotalUsers: 100,
		Duration:   10,
		RampUp:     10,
		Type:       api.PtTaskType_LOCAL,
	}
	return &data
}

func CreatePtTask(ctx context.Context, c *gin.Context) {
	//1. Create a VPC network if not exist* - do it when install Pt-Admin
	//2. Create a Service Account for the GKE Autopilot cluster*  - do it when install Pt-Admin

	//3. Create a GKE Autopilot cluster
	//4. Configure Storage Class in the GKE Autopilot cluster
	//5. Configure PV in the GKE Autopilot cluster
	//6. Binding service sccount with GSA (Workload Identity)
	//7. Deploy pt-operator to the GKE Autopilot cluster

	//8. Apply PtTask to the GKE Autopilot cluster
}

func WithdrawPtTask(ctx context.Context, c *gin.Context) {
	//1. Delete PtTask from the GKE Autopilot cluster
	//2. Delete pt-operator from the GKE Autopilot cluster
	//3. Delete GKE Autopilot cluster
}
