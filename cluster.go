// cluster.go
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getClusterStatus returns information about the cluster status
func getClusterStatus(c *gin.Context) {
	// Get active nodes
	activeNodes := getActiveNodes()

	// Check if cluster mode is enabled
	clusterEnabled := config.ClusterEnabled

	// Get other relevant information
	response := gin.H{
		"node_id":         config.NodeID,
		"cluster_enabled": clusterEnabled,
		"nodes":           activeNodes,
		"node_count":      len(activeNodes),
		"status":          "healthy",
		"synchronized":    true,
	}

	c.JSON(http.StatusOK, response)
}

// Update hook functions to publish entity changes when entities are created, updated, or deleted

// After registering a user, publish the change
func publishUserChange(user User, action string) {
	if config.ClusterEnabled {
		PublishEntityChange("user", action, user.ID, user)
	}
}

// After registering a device, publish the change
func publishDeviceChange(device ESPDevice, action string) {
	if config.ClusterEnabled {
		PublishEntityChange("device", action, device.ID, device)
	}
}

// After adding a command, publish the change
func publishCommandChange(command Command, action string) {
	if config.ClusterEnabled {
		PublishEntityChange("command", action, command.ID, command)
	}
}

// After file operations, publish the change
func publishFileChange(file FileMetadata, action string) {
	if config.ClusterEnabled {
		PublishEntityChange("file", action, file.ID, file)
	}
}
