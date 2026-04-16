package quadrant

import (
	"sync"
	"time"
)

// ComputeProgress holds real-time progress for an active quadrant computation.
// Stored in-memory (not persisted); reset on restart.
type ComputeProgress struct {
	Exchange  string    `json:"exchange"`    // "ASHARE" / "HKEX"
	Status    string    `json:"status"`      // "idle" / "running" / "success" / "failed" / "timeout"
	Current   int       `json:"current"`     // 已完成数
	Total     int       `json:"total"`       // 总数 (estimated before first callback)
	Percent   float64   `json:"percent"`     // 0-100
	UpdatedAt time.Time `json:"updated_at"`  // 最后更新时间
	TaskLogID string    `json:"task_log_id"` // 关联的任务日志ID
	ErrorMsg  string    `json:"error_msg,omitempty"`
	Message   string    `json:"message,omitempty"` // 阶段描述，如「正在拉取全市场快照...」
}

// progressState holds all in-memory progress data, keyed by exchange.
var (
	progressMu   sync.RWMutex
	progressMap map[string]*ComputeProgress // key: "ASHARE" or "HKEX"
)

func init() {
	progressMap = make(map[string]*ComputeProgress)
	// Initialize both exchanges as idle
	progressMap["ASHARE"] = &ComputeProgress{Exchange: "ASHARE", Status: "idle", UpdatedAt: time.Now()}
	progressMap["HKEX"] = &ComputeProgress{Exchange: "HKEX", Status: "idle", UpdatedAt: time.Now()}
}

// GetProgress returns a snapshot of current progress for all exchanges.
// Exchanges that have never received a progress update return idle status.
func GetProgress() map[string]ComputeProgress {
	progressMu.RLock()
	defer progressMu.RUnlock()

	result := make(map[string]ComputeProgress, len(progressMap))
	for k, v := range progressMap {
		result[k] = *v
	}
	return result
}

// UpdateProgress sets (or initializes) the progress for a given exchange.
// Thread-safe. Returns the updated progress for convenience.
func UpdateProgress(exchange string, p ComputeProgress) ComputeProgress {
	progressMu.Lock()
	defer progressMu.Unlock()

	p.UpdatedAt = time.Now()
	if p.Total > 0 {
		p.Percent = float64(p.Current) / float64(p.Total) * 100
	} else if p.Status == "running" && p.Total == 0 {
		// Total not yet known — show indeterminate
		p.Percent = 0
	}
	progressMap[exchange] = &p
	return p
}

// SetProgressTerminal updates progress to a terminal state (success/failed/timeout).
// Preserves Current/Total/TaskLogID from the last running state.
func SetProgressTerminal(exchange, status, errorMsg string) {
	progressMu.Lock()
	defer progressMu.Unlock()

	existing, ok := progressMap[exchange]
	if ok && existing != nil {
		existing.Status = status
		existing.ErrorMsg = errorMsg
		existing.UpdatedAt = time.Now()
		if status == "success" && existing.Total > 0 {
			existing.Current = existing.Total
			existing.Percent = 100
		}
	} else {
		// No prior state (e.g., trigger failed before any progress)
		progressMap[exchange] = &ComputeProgress{
			Exchange: exchange,
			Status:   status,
			ErrorMsg: errorMsg,
			UpdatedAt: time.Now(),
		}
	}
}

// IsRunning returns true if either exchange is currently in "running" state.
func IsRunning() bool {
	progressMu.RLock()
	defer progressMu.RUnlock()

	for _, v := range progressMap {
		if v != nil && v.Status == "running" {
			return true
		}
	}
	return false
}
