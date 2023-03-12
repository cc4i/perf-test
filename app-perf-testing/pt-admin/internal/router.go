package internal

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
)

// Router is the router of the API
func Router(ctx context.Context, l logr.Logger) *gin.Engine {
	r := gin.New()
	defaultV1(ctx, r)

	// Health check
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	return r
}

// defaultV1 is the default version 1 API
func defaultV1(ctx context.Context, r *gin.Engine) {
	v1 := r.Group("/v1")
	{
		// Create a new task
		v1.POST("/pttask", func(c *gin.Context) {
			c.String(http.StatusOK, "Create a new task")
		})

		// List all tasks
		v1.GET("/pttask", func(c *gin.Context) {
			// c.String(http.StatusOK, "List all tasks")
			d := PtTask(ctx, c)
			c.ProtoBuf(http.StatusOK, d)
		})

		// Get a task
		v1.GET("/pttask/:taskId", func(c *gin.Context) {
			taskId := c.Param("taskId")
			c.String(http.StatusOK, "The detail of task: "+taskId)
		})

		// Delete a task
		v1.DELETE("/pttask/:taskId", func(c *gin.Context) {
			taskId := c.Param("taskId")
			c.String(http.StatusOK, "Delete the task: "+taskId)
		})

	}
}
