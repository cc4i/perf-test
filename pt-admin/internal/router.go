package internal

import (
	"context"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Router is the router of the API
func Router(ctx context.Context) *gin.Engine {
	r := gin.New()
	// AllowAllOrigins
	r.Use(cors.Default())

	defaultV1(ctx, r)

	// Health check
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	return r
}

// defaultV1 is the default version 1 API
func defaultV1(ctx context.Context, r *gin.Engine) {
	l := log.FromContext(ctx).WithName("defaultV1")

	v1 := r.Group("/v1")
	{
		///////////////////////////////////////////////////////
		// Json
		///////////////////////////////////////////////////////
		// Create a VPC
		v1.POST("/vpc", func(c *gin.Context) {
			l.Info("Create a VPC")
			resp, err := CreateVPC(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Delete a VPC
		v1.DELETE("/vpc", func(c *gin.Context) {
			l.Info("Delete a VPC")
			c.JSON(http.StatusOK, map[string]string{"message": "Delete a VPC"})
		})

		// Create Service Account
		v1.POST("/sa", func(c *gin.Context) {
			l.Info("Create Service Account")
			resp, err := CreateServiceAccount(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Generate a key of service account
		v1.POST("/sa/key", func(c *gin.Context) {
			l.Info("Generate a key of service account")
			resp, err := CreateServiceAccountKey(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Create a GKE Autopilot cluster
		v1.POST("/gkeap", func(c *gin.Context) {
			l.Info("Create a GKE Autopilot cluster")
			resp, err := CreateGKEAutopilotCluster(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Check status of GKE Autopilot cluster
		v1.POST("/gkeap/status", func(c *gin.Context) {
			l.Info("Check status of creating GKE Autopilot cluster")
			resp, err := CheckClusterStatus(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				if len(resp) == 0 {
					c.JSON(http.StatusOK, map[string]string{"status": "DONE"})
				} else {
					c.JSON(http.StatusOK, map[string]string{"status": "IN_PROGRESS"})
				}

			}
		})

		// Retrieve cluster info of GKE Autopilot cluster
		v1.POST("/gkeap/info", func(c *gin.Context) {
			l.Info("Retrieve cluster info of GKE Autopilot cluster")
			resp, err := RetrieveClusterInfo(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Apply a manifest to GKE Autopilot cluster
		v1.POST("/gkeap/manifest/:file", func(c *gin.Context) {
			l.Info("Apply a manifest to GKE Autopilot cluster")
			err := ApplyManifest(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// Binding Workload Identity role to Kuberneretes service account
		v1.POST("/gkeap/bindingsa", func(c *gin.Context) {
			l.Info("Binding Workload Identity role to Kuberneretes service account")
			err := BindingWorkloadIdentity(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// Get the status of PVC
		v1.POST("/gkeap/pvc", func(c *gin.Context) {
			l.Info("Get the status of PVC")
			resp, err := GetPVCStatus(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Get the status of Pt-Operator
		v1.POST("/gkeap/ptoperator", func(c *gin.Context) {
			l.Info("Get the status of Pt-Operator")
			resp, err := GetPtOperatorStatus(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Execeute the workflow to provision everything
		v1.POST("/workflow/provision/:wf", func(c *gin.Context) {
			l.Info("Execeute the workflow of provisioning everything")
			err := ExecWorkflow(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// Record the execution of workflow into Firestore
		v1.PATCH("/workflow/:wf/:executionId", func(c *gin.Context) {
			l.Info("Record the execution of workflow into Firestore")
			err := RecordWorkflow(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})
		v1.PATCH("/workflow/status/:correlationId", func(c *gin.Context) {
			l.Info("Update status of workflow into Firestore")
			err := UpdateWorkflowStatus(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// TODO: Destroy everything we just provisioned for PtTask, expected: ServiceAccount, GCS, Firestore
		v1.POST("/workflow/destroy/:projectId/:correlationId", func(c *gin.Context) {
			l.Info("Execeute the workflow of destroy related resources")
			err := DestroyResources(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// Get the status of the workflow
		v1.GET("/workflow/:projectId/:region/:workflow/:executionId", func(c *gin.Context) {
			l.Info("Get the status of the workflow")
			resp, err := StatusWorkflow(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, resp)
			}
		})

		// Build container images for testing tasks
		v1.POST("/build", func(c *gin.Context) {
			l.Info("Submit a Cloud Build Job")
			err := BuildImage(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// Prepare and apply the PtTask
		v1.POST("/apply/pttask/:executionId", func(c *gin.Context) {
			l.Info("Prepare PtTask and apply into the Kubenetes cluster")
			pt, err := PreparenApplyPtTask(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, pt)
			}
		})
		// Update the status of PtTask
		v1.PATCH("/update/pttask/:correlationId", func(c *gin.Context) {
			l.Info("Update the status of PtTask")
			err := UpdatePtTaskStatus(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

		// Create a Dashboard for PtTask
		v1.POST("/dashboard/pttask/:projectId/:correlationId", func(c *gin.Context) {
			l.Info("Create a Dashboard for PtTask")
			dUrl, err := CreateDashboard(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success", dUrl})
			}
		})

		// Get dir of inside container (just for testing)
		v1.GET("/dir/:f", func(c *gin.Context) {
			pf := c.Param("f")

			if pf == "root" {
				pf = "/"
			} else {
				pf = "/" + strings.ReplaceAll(pf, "-", "/")
			}
			fs, err := ioutil.ReadDir(pf)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				s := make(map[string]map[string]string)
				for i, f := range fs {
					ss := make(map[string]string)
					if f.IsDir() {
						ss["type"] = "dir"
					} else {
						ss["type"] = "file"
					}
					ss["name"] = f.Name()
					ss["mode"] = f.Mode().String()
					ss["size"] = strconv.FormatInt(f.Size(), 10)

					s[strconv.Itoa(i)] = ss
				}
				c.JSON(http.StatusOK, s)
			}
		})

		///////////////////////////////////////////////////////
		// ProtoBuf
		///////////////////////////////////////////////////////
		// Create a new task
		v1.POST("/pttask", func(c *gin.Context) {
			l.Info("Create a new PtTask")
			pt, err := CreatePtTask(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, pt)
			}
		})

		// List all tasks
		v1.GET("/pttask", func(c *gin.Context) {
			l.Info("List all PtTasks")
			pts, err := ListPtTasks(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, pts)
			}
		})

		// Get a task
		v1.GET("/pttask/:correlationId", func(c *gin.Context) {
			correlationId := c.Param("correlationId")
			l.Info("Get a PtTask", "correlationId", correlationId)

			pt, err := GetPtTask(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, pt)
			}
		})

		// Delete a task
		v1.DELETE("/pttask/:correlationId", func(c *gin.Context) {
			correlationId := c.Param("correlationId")
			l.Info("Delete a PtTask", "correlationId", correlationId)

			err := DeletePtTask(ctx, c)
			if err != nil {
				c.JSON(http.StatusBadRequest, []string{err.Error()})
			} else {
				c.JSON(http.StatusOK, []string{"success"})
			}
		})

	}
}
