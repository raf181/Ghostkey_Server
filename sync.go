// sync.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// Constants for synchronization
const (
	// How often to broadcast changes to other nodes (milliseconds)
	BroadcastInterval = 500 * time.Millisecond

	// Maximum message size allowed from peer
	MaxMessageSize = 1024 * 1024 // 1MB

	// Time allowed to write a message to the peer
	WriteWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	PongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than PongWait)
	PingInterval = (PongWait * 9) / 10
)

// NodeInfo contains information about a node in the cluster
type NodeInfo struct {
	ID              string    `json:"id"`
	Address         string    `json:"address"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	IsActive        bool      `json:"is_active"`
	WebSocketActive bool      `json:"ws_active"`
}

// SyncEvent represents an event to be synchronized across nodes
type SyncEvent struct {
	Type        string          `json:"type"`         // Type of entity: user, command, device, file
	Action      string          `json:"action"`       // create, update, delete
	EntityID    uint            `json:"entity_id"`    // ID of the entity
	Data        json.RawMessage `json:"data"`         // JSON data of the entity
	OriginNode  string          `json:"origin_node"`  // ID of the node that created the event
	Timestamp   time.Time       `json:"timestamp"`    // When the event was created
	VersionHash string          `json:"version_hash"` // Hash to detect conflicts
}

// Global variables for synchronization
var (
	// nodesMutex protects access to the nodes map
	nodesMutex sync.RWMutex

	// nodes is a map of node IDs to NodeInfo
	nodes = make(map[string]*NodeInfo)

	// eventsMutex protects access to the pendingEvents slice
	eventsMutex sync.RWMutex

	// pendingEvents is a list of events waiting to be broadcast
	pendingEvents []SyncEvent

	// websocket upgrader for handling websocket connections
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all connections in this example
		},
	}

	// Active WebSocket connections and the mutex to protect them
	clients    = make(map[*websocket.Conn]bool)
	clientsMtx sync.Mutex

	// Channel for broadcasting messages to all clients
	broadcast = make(chan SyncEvent)

	// nodeID is the unique identifier for this node
	nodeID string
)

// initSync initializes the synchronization system
func initSync(nodeId string) {
	nodeID = nodeId

	// Register this node
	registerLocalNode()

	// Start the broadcaster goroutine
	go broadcaster()

	// Start the gossip periodically
	go periodicGossip()

	// Connect to all configured nodes
	go connectToNodes()

	log.Printf("Sync system initialized with node ID: %s", nodeID)
}

// registerLocalNode adds this node to the nodes map
func registerLocalNode() {
	nodesMutex.Lock()
	defer nodesMutex.Unlock()

	nodes[nodeID] = &NodeInfo{
		ID:              nodeID,
		Address:         config.ServerInterface,
		LastSeenAt:      time.Now().UTC(),
		IsActive:        true,
		WebSocketActive: false,
	}
}

// connectToNodes attempts to connect to all nodes in the config
func connectToNodes() {
	for _, nodeAddr := range config.GossipNodes {
		go connectToNode(nodeAddr)
	}
}

// connectToNode establishes a WebSocket connection to a node
func connectToNode(nodeAddr string) {
	wsURL := fmt.Sprintf("ws://%s/ws", nodeAddr)
	log.Printf("Attempting to connect to node at %s", wsURL)

	// Replace the host part if it's a relative address
	if nodeAddr[0] == ':' {
		wsURL = fmt.Sprintf("ws://localhost%s/ws", nodeAddr)
	}

	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Printf("Failed to connect to node %s: %v", nodeAddr, err)
		return
	}

	// Add this connection to the clients map
	clientsMtx.Lock()
	clients[c] = true
	clientsMtx.Unlock()

	log.Printf("Connected to node at %s", wsURL)

	// Start reading messages from this connection
	go handleConnection(c)
}

// handleWebSocket handles WebSocket connections
func handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to set websocket upgrade: %v", err)
		return
	}

	// Add this connection to the clients map
	clientsMtx.Lock()
	clients[conn] = true
	clientsMtx.Unlock()

	// Handle the connection
	handleConnection(conn)
}

// handleConnection reads messages from a WebSocket connection
func handleConnection(conn *websocket.Conn) {
	defer func() {
		clientsMtx.Lock()
		delete(clients, conn)
		clientsMtx.Unlock()
		conn.Close()
		log.Println("WebSocket connection closed")
	}()

	// Set read limit, deadline, and handlers
	conn.SetReadLimit(MaxMessageSize)
	conn.SetReadDeadline(time.Now().Add(PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})

	// Read messages from the connection
	for {
		var event SyncEvent
		err := conn.ReadJSON(&event)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Process the received event
		processEvent(event)
	}
}

// broadcaster sends messages to all connected clients
func broadcaster() {
	ticker := time.NewTicker(BroadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case event := <-broadcast:
			// Send the event to all connected clients
			clientsMtx.Lock()
			for client := range clients {
				client.SetWriteDeadline(time.Now().Add(WriteWait))
				if err := client.WriteJSON(event); err != nil {
					log.Printf("Failed to send event to client: %v", err)
					client.Close()
					delete(clients, client)
				}
			}
			clientsMtx.Unlock()

		case <-ticker.C:
			// Check if there are any pending events to broadcast
			eventsMutex.Lock()
			if len(pendingEvents) > 0 {
				events := pendingEvents
				pendingEvents = nil
				eventsMutex.Unlock()

				// Send each event to all clients
				for _, event := range events {
					broadcast <- event
				}
			} else {
				eventsMutex.Unlock()
			}

			// Send ping messages to all clients
			clientsMtx.Lock()
			for client := range clients {
				client.SetWriteDeadline(time.Now().Add(WriteWait))
				if err := client.WriteMessage(websocket.PingMessage, nil); err != nil {
					client.Close()
					delete(clients, client)
				}
			}
			clientsMtx.Unlock()
		}
	}
}

// periodicGossip sends gossip messages periodically
func periodicGossip() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		gossipWithNodes()
	}
}

// gossipWithNodes sends gossip messages to all known nodes
func gossipWithNodes() {
	nodesMutex.RLock()
	defer nodesMutex.RUnlock()

	for id, node := range nodes {
		// Skip ourselves and inactive nodes
		if id == nodeID || !node.IsActive {
			continue
		}

		// If WebSocket is active, we don't need to gossip via HTTP
		if node.WebSocketActive {
			continue
		}

		// Send gossip via HTTP
		go sendGossipToNode(node)
	}
}

// sendGossipToNode sends a gossip message to a node
func sendGossipToNode(node *NodeInfo) {
	// Create the payload
	payload := createGossipPayload()

	// Marshal the payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal gossip payload: %v", err)
		return
	}

	// Create the URL
	url := fmt.Sprintf("http://%s/gossip", node.Address)

	// Send the request with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("Failed to gossip with %s: %v", node.Address, err)
		markNodeInactive(node.ID)
		return
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		log.Printf("Received non-OK status code from %s: %v", node.Address, resp.StatusCode)
		return
	}

	// Parse the response
	var responsePayload GossipPayload
	if err := json.NewDecoder(resp.Body).Decode(&responsePayload); err != nil {
		log.Printf("Failed to decode response from %s: %v", node.Address, err)
		return
	}

	// Process the response
	processGossipResponse(&responsePayload)

	// Mark the node as active
	markNodeActive(node.ID, false)
}

// createGossipPayload creates a gossip payload with all local data
func createGossipPayload() GossipPayload {
	var localVersionVector VersionVector

	// Fetch the local version vector
	db.Model(&Command{}).Pluck("updated_at", &localVersionVector)

	// Fetch local commands
	var localCommands []Command
	db.Find(&localCommands)

	// Fetch local devices
	var localDevices []ESPDevice
	db.Find(&localDevices)

	// Fetch local users
	var localUsers []User
	db.Find(&localUsers)

	// Fetch local file metadata
	var localFiles []FileMetadata
	db.Find(&localFiles)

	// Construct payload
	return GossipPayload{
		VersionVector: localVersionVector,
		Commands:      localCommands,
		ESPDevices:    localDevices,
		Users:         localUsers,
		FileMetadata:  localFiles,
		NodeID:        nodeID,
	}
}

// processGossipResponse processes a gossip response
func processGossipResponse(payload *GossipPayload) {
	// Update the node information
	updateNodeInfo(payload.NodeID, true)

	// Process each entity type
	mergeCommands(payload.Commands)
	mergeDevices(payload.ESPDevices)
	mergeUsers(payload.Users)
	mergeFiles(payload.FileMetadata)
}

// processEvent processes a received sync event
func processEvent(event SyncEvent) {
	// Skip events from ourselves
	if event.OriginNode == nodeID {
		return
	}

	// Process the event based on type
	switch event.Type {
	case "user":
		processUserEvent(event)
	case "device":
		processDeviceEvent(event)
	case "command":
		processCommandEvent(event)
	case "file":
		processFileEvent(event)
	default:
		log.Printf("Unknown event type: %s", event.Type)
	}
}

// processUserEvent handles user-related events
func processUserEvent(event SyncEvent) {
	var user User

	if err := json.Unmarshal(event.Data, &user); err != nil {
		log.Printf("Failed to unmarshal user data: %v", err)
		return
	}

	switch event.Action {
	case "create", "update":
		var existingUser User
		result := db.Where("id = ?", user.ID).First(&existingUser)

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// User doesn't exist, create it
				db.Create(&user)
			} else {
				log.Printf("Error checking for user: %v", result.Error)
			}
		} else {
			// User exists, update if newer
			if user.UpdatedAt.After(existingUser.UpdatedAt) {
				db.Save(&user)
			}
		}

	case "delete":
		db.Delete(&User{}, user.ID)
	}
}

// processDeviceEvent handles device-related events
func processDeviceEvent(event SyncEvent) {
	var device ESPDevice

	if err := json.Unmarshal(event.Data, &device); err != nil {
		log.Printf("Failed to unmarshal device data: %v", err)
		return
	}

	switch event.Action {
	case "create", "update":
		var existingDevice ESPDevice
		result := db.Where("esp_id = ?", device.EspID).First(&existingDevice)

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// Device doesn't exist, create it
				db.Create(&device)
			} else {
				log.Printf("Error checking for device: %v", result.Error)
			}
		} else {
			// Device exists, update if newer
			if device.UpdatedAt.After(existingDevice.UpdatedAt) {
				db.Save(&device)
			}
		}

	case "delete":
		db.Where("esp_id = ?", device.EspID).Delete(&ESPDevice{})
	}
}

// processCommandEvent handles command-related events
func processCommandEvent(event SyncEvent) {
	var command Command

	if err := json.Unmarshal(event.Data, &command); err != nil {
		log.Printf("Failed to unmarshal command data: %v", err)
		return
	}

	switch event.Action {
	case "create", "update":
		var existingCommand Command
		result := db.Where("id = ?", command.ID).First(&existingCommand)

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// Command doesn't exist, create it
				db.Create(&command)
			} else {
				log.Printf("Error checking for command: %v", result.Error)
			}
		} else {
			// Command exists, update if newer
			if command.UpdatedAt.After(existingCommand.UpdatedAt) {
				db.Save(&command)
			}
		}

	case "delete":
		db.Delete(&Command{}, command.ID)
	}
}

// processFileEvent handles file-related events
func processFileEvent(event SyncEvent) {
	var file FileMetadata

	if err := json.Unmarshal(event.Data, &file); err != nil {
		log.Printf("Failed to unmarshal file data: %v", err)
		return
	}

	switch event.Action {
	case "create", "update":
		var existingFile FileMetadata
		result := db.Where("id = ?", file.ID).First(&existingFile)

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// File doesn't exist, create it
				db.Create(&file)
			} else {
				log.Printf("Error checking for file: %v", result.Error)
			}
		} else {
			// File exists, update if newer
			if file.UpdatedAt.After(existingFile.UpdatedAt) {
				db.Save(&file)
			}
		}

	case "delete":
		db.Delete(&FileMetadata{}, file.ID)
	}
}

// createSyncEvent creates a new sync event
func createSyncEvent(entityType string, action string, entityID uint, data interface{}) SyncEvent {
	dataJSON, _ := json.Marshal(data)

	return SyncEvent{
		Type:        entityType,
		Action:      action,
		EntityID:    entityID,
		Data:        dataJSON,
		OriginNode:  nodeID,
		Timestamp:   time.Now().UTC(),
		VersionHash: fmt.Sprintf("%s-%d-%s", nodeID, time.Now().UnixNano(), action),
	}
}

// queueSyncEvent adds an event to the pending events queue
func queueSyncEvent(event SyncEvent) {
	eventsMutex.Lock()
	defer eventsMutex.Unlock()

	pendingEvents = append(pendingEvents, event)
}

// updateNodeInfo updates the information for a node
func updateNodeInfo(id string, isActive bool) {
	nodesMutex.Lock()
	defer nodesMutex.Unlock()

	if node, exists := nodes[id]; exists {
		node.LastSeenAt = time.Now().UTC()
		node.IsActive = isActive
	} else {
		nodes[id] = &NodeInfo{
			ID:              id,
			LastSeenAt:      time.Now().UTC(),
			IsActive:        isActive,
			WebSocketActive: false,
		}
	}
}

// markNodeActive marks a node as active
func markNodeActive(id string, wsActive bool) {
	nodesMutex.Lock()
	defer nodesMutex.Unlock()

	if node, exists := nodes[id]; exists {
		node.LastSeenAt = time.Now().UTC()
		node.IsActive = true
		node.WebSocketActive = wsActive
	}
}

// markNodeInactive marks a node as inactive
func markNodeInactive(id string) {
	nodesMutex.Lock()
	defer nodesMutex.Unlock()

	if node, exists := nodes[id]; exists {
		node.IsActive = false
		node.WebSocketActive = false
	}
}

// getActiveNodes returns a list of active nodes
func getActiveNodes() []*NodeInfo {
	nodesMutex.RLock()
	defer nodesMutex.RUnlock()

	var activeNodes []*NodeInfo

	for _, node := range nodes {
		if node.IsActive {
			activeNodes = append(activeNodes, node)
		}
	}

	return activeNodes
}

// Merge functions for each entity type

func mergeCommands(remoteCommands []Command) {
	for _, remoteCommand := range remoteCommands {
		var localCommand Command
		if err := db.Where("id = ?", remoteCommand.ID).First(&localCommand).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteCommand)
			}
		} else {
			if remoteCommand.UpdatedAt.After(localCommand.UpdatedAt) {
				db.Save(&remoteCommand)
			}
		}
	}
}

func mergeDevices(remoteDevices []ESPDevice) {
	for _, remoteDevice := range remoteDevices {
		var localDevice ESPDevice
		if err := db.Where("esp_id = ?", remoteDevice.EspID).First(&localDevice).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteDevice)
			}
		} else {
			if remoteDevice.UpdatedAt.After(localDevice.UpdatedAt) {
				db.Save(&remoteDevice)
			}
		}
	}
}

func mergeUsers(remoteUsers []User) {
	for _, remoteUser := range remoteUsers {
		var localUser User
		if err := db.Where("id = ?", remoteUser.ID).First(&localUser).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteUser)
			}
		} else {
			if remoteUser.UpdatedAt.After(localUser.UpdatedAt) {
				db.Save(&remoteUser)
			}
		}
	}
}

func mergeFiles(remoteFiles []FileMetadata) {
	for _, remoteFile := range remoteFiles {
		var localFile FileMetadata
		if err := db.Where("id = ?", remoteFile.ID).First(&localFile).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				db.Create(&remoteFile)
			}
		} else {
			if remoteFile.UpdatedAt.After(localFile.UpdatedAt) {
				db.Save(&remoteFile)
			}
		}
	}
}

// PublishEntityChange creates and queues a sync event for an entity change
func PublishEntityChange(entityType string, action string, entityID uint, data interface{}) {
	event := createSyncEvent(entityType, action, entityID, data)
	queueSyncEvent(event)
}
