package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// DataRoot 压测结果输出根目录（相对进程工作目录）；任务目录与历史查询共用同一路径
const DataRoot = "./Data"

// SummaryFileName 每个任务目录下的汇总文件名
const SummaryFileName = "summary.json"

// dirTimeLayout 任务目录名的时间前缀格式，与创建任务时使用的格式保持一致
const dirTimeLayout = "2006-01-02_15-04-05"

// History 一个已落盘任务的索引信息（GET /history 的元素）。
// 数据全部来自磁盘，因此进程重启后依然可查，但不含实时进度、也无法取消。
type History struct {
	ID        string    `json:"id"`
	Dir       string    `json:"dir"`
	StartedAt time.Time `json:"startedAt"` // 任务创建时间，取自目录名
	Summary   *Summary  `json:"summary,omitempty"`
	Note      string    `json:"note,omitempty"` // 目录存在但结果不完整时的说明
}

// LoadHistory 扫描 root 下的任务目录并读取各自的 summary.json，按创建时间从新到旧返回。
// limit > 0 时最多返回 limit 条，limit <= 0 表示不限制。
// root 不存在视为「还没有历史」，返回空列表而非错误。
func LoadHistory(root string, limit int) ([]History, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []History{}, nil
		}
		return nil, fmt.Errorf("读取结果目录 %s 失败: %w", root, err)
	}

	list := make([]History, 0, len(entries))
	for _, e := range entries {
		// 只认目录；跳过 Data 下可能出现的零散文件
		if !e.IsDir() {
			continue
		}
		h, ok := loadHistoryEntry(root, e.Name())
		if !ok {
			continue
		}
		list = append(list, h)
	}

	sort.Slice(list, func(a, b int) bool {
		return list[a].StartedAt.After(list[b].StartedAt)
	})

	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// loadHistoryEntry 读取单个任务目录。目录名不是 `<时间>_<jobID>` 形式时返回 false（跳过）。
func loadHistoryEntry(root, name string) (History, bool) {
	startedAt, dirID, ok := parseDirName(name)
	if !ok {
		return History{}, false
	}

	h := History{
		ID:        dirID,
		Dir:       filepath.Join(root, name),
		StartedAt: startedAt,
	}

	data, err := os.ReadFile(filepath.Join(h.Dir, SummaryFileName))
	if err != nil {
		// 任务异常中断（没走到写 summary 的收尾逻辑）时目录里没有 summary.json
		if os.IsNotExist(err) {
			h.Note = "缺少 " + SummaryFileName + "，任务可能未正常结束"
		} else {
			h.Note = "读取 " + SummaryFileName + " 失败: " + err.Error()
		}
		return h, true
	}

	var sum Summary
	if err := json.Unmarshal(data, &sum); err != nil {
		h.Note = "解析 " + SummaryFileName + " 失败: " + err.Error()
		return h, true
	}

	// summary 里记录的 jobId 更权威，缺失时才回退到目录名中的 ID
	if sum.JobID != "" {
		h.ID = sum.JobID
	}
	h.Summary = &sum
	return h, true
}

// parseDirName 解析任务目录名，格式为 `<2006-01-02_15-04-05>_<jobID>`
func parseDirName(name string) (time.Time, string, bool) {
	if len(name) <= len(dirTimeLayout) {
		return time.Time{}, "", false
	}

	startedAt, err := time.ParseInLocation(dirTimeLayout, name[:len(dirTimeLayout)], time.Local)
	if err != nil {
		return time.Time{}, "", false
	}

	// 时间前缀后应紧跟分隔符 _
	if name[len(dirTimeLayout)] != '_' {
		return time.Time{}, "", false
	}

	id := name[len(dirTimeLayout)+1:]
	if id == "" {
		return time.Time{}, "", false
	}
	return startedAt, id, true
}
