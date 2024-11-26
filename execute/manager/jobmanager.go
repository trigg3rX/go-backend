// github.com/trigg3rX/go-backend/execute/manager/jobmanager.go
package manager

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"
    "github.com/shirou/gopsutil/v3/cpu"
    "github.com/shirou/gopsutil/v3/mem"
    "github.com/robfig/cron/v3"
    "github.com/trigg3rX/go-backend/pkg/network"
    "github.com/trigg3rX/go-backend/pkg/types"
)
var (
    ErrInvalidTimeframe = fmt.Errorf("invalid timeframe specified")
)

// SystemResources tracks system resource usage
type SystemResources struct {
    CPUUsage    float64
    MemoryUsage float64
    MaxCPU      float64
    MaxMemory   float64
}

// WaitingJob represents a job waiting in queue
type WaitingJob struct {
    Job           *types.Job
    EstimatedTime time.Time
}

// JobScheduler enhanced with load balancing
type JobScheduler struct {
    jobs              map[string]*types.Job
    quorums           map[string]*Quorum
    jobQueue          chan *types.Job
    waitingQueue      []WaitingJob
    resources         SystemResources
    Cron              *cron.Cron
    ctx               context.Context
    cancel            context.CancelFunc
    mu                sync.RWMutex
    workersCount      int
    metricsInterval   time.Duration
    waitingQueueMu    sync.RWMutex
    messaging *network.Messaging
	discovery *network.Discovery
}

// NewJobScheduler creates an enhanced scheduler with resource limits
func NewJobScheduler(workersCount int, messaging *network.Messaging, discovery *network.Discovery) *JobScheduler {
    ctx, cancel := context.WithCancel(context.Background())
    cronInstance := cron.New(cron.WithSeconds())
    
    scheduler := &JobScheduler{
        jobs:             make(map[string]*types.Job),
        quorums:          make(map[string]*Quorum),
        jobQueue:         make(chan *types.Job, 1000),
        waitingQueue:     make([]WaitingJob, 0),
        resources: SystemResources{
            MaxCPU:    10.0, // 10% CPU threshold
            MaxMemory: 80.0, // 80% Memory threshold
        },
        Cron:            cronInstance,
        ctx:             ctx,
        cancel:          cancel,
        workersCount:    workersCount,
        metricsInterval: 5 * time.Second,
        messaging: messaging,
        discovery: discovery,
    }

    scheduler.initializeQuorums()
    scheduler.startWorkers()
    go scheduler.monitorResources()
    go scheduler.processWaitingQueue()
    
    return scheduler
}

// monitorResources continuously monitors system resources
func (js *JobScheduler) monitorResources() {
    ticker := time.NewTicker(js.metricsInterval)
    defer ticker.Stop()

    for {
        select {
        case <-js.ctx.Done():
            return
        case <-ticker.C:
            cpuPercent, err := cpu.Percent(time.Second, false)
            if err == nil && len(cpuPercent) > 0 {
                js.resources.CPUUsage = cpuPercent[0]
            }

            memInfo, err := mem.VirtualMemory()
            if err == nil {
                js.resources.MemoryUsage = memInfo.UsedPercent
            }

            // Log current resource usage
            log.Printf("System Resources - CPU: %.2f%%, Memory: %.2f%%",
                js.resources.CPUUsage, js.resources.MemoryUsage)
        }
    }
}

// checkResourceAvailability verifies if system can handle new jobs
func (js *JobScheduler) checkResourceAvailability() bool {
    return js.resources.CPUUsage < js.resources.MaxCPU &&
           js.resources.MemoryUsage < js.resources.MaxMemory
}

// AddJob enhanced with resource checking
func (js *JobScheduler) AddJob(job *types.Job) error {
    if job.TimeFrame <= 0 {
        return ErrInvalidTimeframe
    }

    js.mu.Lock()
    defer js.mu.Unlock()

    // Check system resources
    if !js.checkResourceAvailability() {
        // Calculate estimated time for resource availability
        estimatedTime := js.calculateEstimatedWaitTime()
        
        // Add to waiting queue
        js.waitingQueueMu.Lock()
        js.waitingQueue = append(js.waitingQueue, WaitingJob{
            Job:           job,
            EstimatedTime: estimatedTime,
        })
        js.waitingQueueMu.Unlock()

        log.Printf("System at capacity. Job %s added to waiting queue. Estimated start time: %v",
            job.JobID, estimatedTime)
        return nil
    }

    return js.scheduleJob(job)
}

// scheduleJob handles the actual job scheduling
func (js *JobScheduler) scheduleJob(job *types.Job) error {
    // Add to jobs map
    js.jobs[job.JobID] = job
    
    // Create cron spec
    cronSpec := fmt.Sprintf("@every %ds", job.TimeInterval)
    
    // Schedule initial execution
    time.AfterFunc(2*time.Second, func() {
        js.processJob(job)
    })
    
    // Schedule recurring executions
    _, err := js.Cron.AddFunc(cronSpec, func() {
        if time.Since(job.CreatedAt) > time.Duration(job.TimeFrame)*time.Second {
            return
        }
        
        js.mu.RLock()
        currentJob := js.jobs[job.JobID]
        shouldQueue := currentJob.Status != "processing" && 
                      currentJob.Status != "completed" && 
                      currentJob.Status != "failed"
        js.mu.RUnlock()

        if shouldQueue {
            js.jobQueue <- job
        }
    })

    if err != nil {
        return fmt.Errorf("failed to schedule job: %w", err)
    }

    log.Printf("Job %s scheduled successfully", job.JobID)
    return nil
}

// calculateEstimatedWaitTime estimates when resources might be available
func (js *JobScheduler) calculateEstimatedWaitTime() time.Time {
    // Find the job that will finish soonest
    var earliestCompletion time.Time
    now := time.Now()
    earliestCompletion = now.Add(30 * time.Second) // Default wait time

    js.mu.RLock()
    for _, job := range js.jobs {
        if job.Status == "processing" {
            expectedCompletion := job.CreatedAt.Add(time.Duration(job.TimeFrame) * time.Second)
            if earliestCompletion.After(expectedCompletion) {
                earliestCompletion = expectedCompletion
            }
        }
    }
    js.mu.RUnlock()

    return earliestCompletion
}

func (js *JobScheduler) SetResourceLimits(cpuThreshold, memoryThreshold float64) {
    js.mu.Lock()
    defer js.mu.Unlock()
    
    js.resources.MaxCPU = cpuThreshold
    js.resources.MaxMemory = memoryThreshold
}

// GetJobDetails returns detailed information about a specific job
func (js *JobScheduler) GetJobDetails(jobID string) (map[string]interface{}, error) {
    js.mu.RLock()
    defer js.mu.RUnlock()

    job, exists := js.jobs[jobID]
    if !exists {
        return nil, fmt.Errorf("job %s not found", jobID)
    }

    return map[string]interface{}{
        "job_id":            job.JobID,
        "status":            job.Status,
        "created_at":        job.CreatedAt,
        "last_executed":     job.LastExecuted,
        "current_retries":   job.CurrentRetries,
        "time_frame":        job.TimeFrame,
        "time_interval":     job.TimeInterval,
        "error":            job.Error,
    }, nil
}

// processWaitingQueue continuously checks and processes waiting jobs
func (js *JobScheduler) processWaitingQueue() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-js.ctx.Done():
            return
        case <-ticker.C:
            if js.checkResourceAvailability() {
                js.waitingQueueMu.Lock()
                if len(js.waitingQueue) > 0 {
                    // Get next job from queue
                    nextJob := js.waitingQueue[0]
                    js.waitingQueue = js.waitingQueue[1:]
                    js.waitingQueueMu.Unlock()

                    // Schedule the job
                    js.mu.Lock()
                    err := js.scheduleJob(nextJob.Job)
                    js.mu.Unlock()

                    if err != nil {
                        log.Printf("Failed to schedule waiting job %s: %v", nextJob.Job.JobID, err)
                    } else {
                        log.Printf("Successfully scheduled waiting job %s", nextJob.Job.JobID)
                    }
                } else {
                    js.waitingQueueMu.Unlock()
                }
            }
        }
    }
}