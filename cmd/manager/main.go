// github.com/trigg3rX/go-backend/cmd/manager/main.go
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"
    "github.com/trigg3rX/go-backend/execute/manager"
)


func main() {
    // Configure logging to show more details
    log.SetOutput(os.Stdout)
    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

    // Initialize the job scheduler with 5 workers
    jobScheduler := manager.NewJobScheduler(5)
    jobScheduler.Cron.Start()
    defer jobScheduler.Stop()

    // Create multiple test jobs with varied properties
    jobs := []struct {
        jobID           string
        timeFrame       int64
        timeInterval    int64
        maxRetries      int
    }{
        {"job_1", 120, 15, 2},
        {"job_2", 60, 5, 3},
        {"job_3", 90, 30, 5},
        {"job_4", 75, 20, 3},
        {"job_5", 45, 10, 2},
        {"job_6", 100, 25, 4},
        {"job_7", 80, 15, 3},
        {"job_8", 70, 20, 2},
        {"job_9", 55, 10, 3},
        {"job_10", 65, 25, 4},
    }

    // Add jobs to the scheduler
    for _, jobConfig := range jobs {
        job := &manager.Job{
            JobID:             jobConfig.jobID,
            ArgType:           "contract_call",
            Arguments:         map[string]interface{}{"function": "transfer", "amount": 100 * i},
            ChainID:           "chain_1",
            ContractAddress:   fmt.Sprintf("0x123...%s", jobConfig.jobID),
            JobCostPrediction: 0.5,
            Stake:             1.0,
            Status:            "pending",
            TargetFunction:    "execute",
            TimeFrame:         jobConfig.timeFrame,
            TimeInterval:      jobConfig.timeInterval,
            UserID:            "system_test",
            CreatedAt:         time.Now(),
            MaxRetries:        jobConfig.maxRetries,
        }

        // Add job to local scheduler
        if err := jobScheduler.AddJob(job); err != nil {
            log.Printf("Failed to add job %s: %v", job.JobID, err)
        } else {
            log.Printf("Added job %s to scheduler with TimeFrame: %ds, Interval: %ds, MaxRetries: %d", 
                job.JobID, job.TimeFrame, job.TimeInterval, job.MaxRetries)
        }

        // Smaller delay to spread out job starts
        time.Sleep(1 * time.Second)
    }
}

    // Wait to allow jobs to process
    time.Sleep(60 * time.Second)

    // Keep the main goroutine alive and log system status periodically
    statusTicker := time.NewTicker(10 * time.Second)
    defer statusTicker.Stop()

    go func() {
        for range statusTicker.C {
            queueStatus := jobScheduler.GetQueueStatus()
            systemMetrics := jobScheduler.GetSystemMetrics()
            
            log.Printf("System Status:")
            log.Printf("  Active Jobs: %d", queueStatus["active_jobs"])
            log.Printf("  Waiting Jobs: %d", queueStatus["waiting_jobs"])
            log.Printf("  CPU Usage: %.2f%%", systemMetrics.CPUUsage)
            log.Printf("  Memory Usage: %.2f%%", systemMetrics.MemoryUsage)
        }
    }()

    // API endpoints (same as before)
    http.HandleFunc("/system/metrics", func(w http.ResponseWriter, r *http.Request) {
        metrics := jobScheduler.GetSystemMetrics()
        json.NewEncoder(w).Encode(metrics)
    })

    // Queue status endpoint
    http.HandleFunc("/queue/status", func(w http.ResponseWriter, r *http.Request) {
        status := jobScheduler.GetQueueStatus()
        json.NewEncoder(w).Encode(status)
    })

    http.HandleFunc("/job/", func(w http.ResponseWriter, r *http.Request) {
        jobID := r.URL.Path[len("/job/"):]
        if jobID == "" {
            http.Error(w, "Job ID required", http.StatusBadRequest)
            return
        }

        details, err := jobScheduler.GetJobDetails(jobID)
        if err != nil {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }

        json.NewEncoder(w).Encode(details)
    })

    // Start HTTP server
    serverAddr := ":8080"
    fmt.Printf("Server starting on %s\n", serverAddr)
    log.Fatal(http.ListenAndServe(serverAddr, nil))
}