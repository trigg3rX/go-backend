// github.com/trigg3rX/go-backend/execute/manager/job.go
package manager

import (
    "log"
    "math/rand"
    "time"
    "os"
    "encoding/json"
    
    "github.com/trigg3rX/go-backend/pkg/network"
    "github.com/trigg3rX/go-backend/pkg/types"
   // "github.com/trigg3rX/triggerx-keeper/pkg/execute"
)

// Job represents a scheduled task with its properties
type Job struct {
    types.Job  // Embed the types.Job struct
    // Additional fields specific to manager package
    CurrentRetries    int
    LastExecuted      time.Time
    NextExecutionTime time.Time
    Error            string
}

// Quorum represents a group of nodes that can execute jobs
type Quorum struct {
    QuorumID    string
    NodeCount   int
    ActiveNodes []string
    Status      string
    ChainID     string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

func init() {
    // Initialize random seed
    rand.Seed(time.Now().UnixNano())
}

// initializeQuorums sets up initial quorums for the scheduler
func (js *JobScheduler) initializeQuorums() {
    defaultQuorum := &Quorum{
        QuorumID:    "default",
        NodeCount:   3,
        ActiveNodes: []string{"node1", "node2", "node3"},
        Status:      "active",
        ChainID:     "chain_1",
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }

    js.mu.Lock()
    js.quorums["default"] = defaultQuorum
    js.mu.Unlock()
}

func (js *JobScheduler) selectRandomKeeper() (string, error) {
    js.mu.RLock()
    defer js.mu.RUnlock()

    if len(js.quorums) == 0 {
        return "", fmt.Errorf("no quorums available")
    }

    // For now, just pick the first quorum and a random active node
    for _, quorum := range js.quorums {
        if len(quorum.ActiveNodes) > 0 {
            // Randomly select a keeper from active nodes
            randomIndex := rand.Intn(len(quorum.ActiveNodes))
            return quorum.ActiveNodes[randomIndex], nil
        }
    }

    return "", fmt.Errorf("no active keepers found")
}

// processJob handles the execution of a job
func (js *JobScheduler) processJob(workerID int, job *Job) {
    js.mu.Lock()
    if job.Status == "completed" || job.Status == "failed" {
        js.mu.Unlock()
        return
    }

    job.Status = "processing"
    job.LastExecuted = time.Now()
    
    // Get a random keeper from the quorum
    quorum := js.quorums["default"]
    if len(quorum.ActiveNodes) == 0 {
        job.Status = "failed"
        job.Error = "no active keepers available"
        js.mu.Unlock()
        return
    }
    
    // Select random keeper
    keeperName := quorum.ActiveNodes[rand.Intn(len(quorum.ActiveNodes))]
    js.mu.Unlock()

    // Enhanced logging with worker and job details
    log.Printf("[Worker %d] Starting to process Job %s (Target: %s, ChainID: %s)", 
        workerID, job.JobID, job.TargetFunction, job.ChainID)

        selectedKeeper, err := js.selectRandomKeeper()
    if err != nil {
        log.Printf("Failed to select keeper for job %s: %v", job.JobID, err)
        return
    }

    // Transmit job to selected keeper using network package
    // Note: This assumes you have a network client initialized
    err = js.networkClient.SendJobToKeeper(selectedKeeper, job)
    if err != nil {
        log.Printf("Failed to send job %s to keeper %s: %v", job.JobID, selectedKeeper, err)
    }

    // Simulate job execution with random success/failure
    executionTime := time.Duration(2+rand.Intn(3)) * time.Second
    log.Printf("[Worker %d] Job %s will take approximately %v to complete", 
        workerID, job.JobID, executionTime)
    time.Sleep(executionTime)

    js.mu.Lock()
    defer js.mu.Unlock()

    if rand.Float64() < 0.8 { // 80% success rate
        job.Status = "completed"
        log.Printf("[Worker %d] Successfully completed Job %s", workerID, job.JobID)
    } else {
        job.CurrentRetries++
        if job.CurrentRetries >= job.MaxRetries {
            job.Status = "failed"
            job.Error = "maximum retries exceeded"
            log.Printf("[Worker %d] Job %s failed after %d retries. Error: %s", 
                workerID, job.JobID, job.MaxRetries, job.Error)
        } else {
            job.Status = "pending"
            log.Printf("[Worker %d] Job %s failed, scheduling retry (%d/%d)", 
                workerID, job.JobID, job.CurrentRetries, job.MaxRetries)
        }
    }

    // Load keeper information
    peerInfos := make(map[string]network.PeerInfo)
    if err := js.loadPeerInfo(&peerInfos); err != nil {
        log.Printf("Failed to load peer info: %v", err)
        return
    }

    keeperInfo, exists := peerInfos[keeperName]
    if !exists {
        log.Printf("Keeper %s not found in peer info", keeperName)
        return
    }

    // Connect to the keeper
    peerID, err := js.discovery.ConnectToPeer(keeperInfo)
    if err != nil {
        log.Printf("Failed to connect to keeper %s: %v", keeperName, err)
        return
    }

    // Send job to keeper
    err = js.messaging.SendMessage(keeperName, *peerID, keeperMsg)
    if err != nil {
        log.Printf("Failed to send job to keeper %s: %v", keeperName, err)
        js.mu.Lock()
        job.Status = "failed"
        job.Error = err.Error()
        js.mu.Unlock()
        return
    }

    log.Printf("Job %s sent to keeper %s", job.JobID, keeperName)
}

func (js *JobScheduler) loadPeerInfo(peerInfos *map[string]network.PeerInfo) error {
    file, err := os.Open(network.PeerInfoFilePath)
    if err != nil {
        return err
    }
    defer file.Close()

    decoder := json.NewDecoder(file)
    return decoder.Decode(peerInfos)
}


// GetSystemMetrics returns current system metrics
func (js *JobScheduler) GetSystemMetrics() SystemResources {
    js.mu.RLock()
    defer js.mu.RUnlock()
    return js.resources
}

// GetQueueStatus returns the current status of job queues
func (js *JobScheduler) GetQueueStatus() map[string]interface{} {
    js.mu.RLock()
    js.waitingQueueMu.RLock()
    defer js.mu.RUnlock()
    defer js.waitingQueueMu.RUnlock()

    return map[string]interface{}{
        "active_jobs":     len(js.jobs),
        "waiting_jobs":    len(js.waitingQueue),
        "cpu_usage":       js.resources.CPUUsage,
        "memory_usage":    js.resources.MemoryUsage,
    }
}

// Stop gracefully shuts down the scheduler
func (js *JobScheduler) Stop() {
    js.cancel()
    js.Cron.Stop()
}

// startWorkers initializes worker goroutines
func (js *JobScheduler) startWorkers() {
    for i := 0; i < js.workersCount; i++ {
        workerID := i
        go func(workerID int) {
            log.Printf("🔧 Worker %d initialized and ready to process jobs", workerID)
            for {
                select {
                case job, ok := <-js.jobQueue:
                    if !ok {
                        log.Printf("Worker %d: Job queue closed", workerID)
                        return
                    }
                    if job == nil {
                        log.Printf("Worker %d: Received nil job", workerID)
                        continue
                    }
                    js.processJob(workerID, job)
                case <-js.ctx.Done():
                    log.Printf("Worker %d: Context cancelled, shutting down", workerID)
                    return
                }
            }
        }(workerID)
    }
}
