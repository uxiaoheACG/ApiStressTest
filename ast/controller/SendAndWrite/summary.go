package SendAndWrite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"ast/job"
)

// buildSummary 汇总任务统计。latencies 中 -1 表示该序号未执行（任务被取消）。
// typeCount 是本次出现过的返回类型数（由 Sink 的指纹聚合得出）。
// 只能在所有 worker 退出后调用。
func buildSummary(j *job.Job, latencies []int64, typeCount int64) *job.Summary {
	snap := j.Snapshot()

	samples := make([]int64, 0, len(latencies))
	for _, v := range latencies {
		if v >= 0 {
			samples = append(samples, v)
		}
	}
	sort.Slice(samples, func(a, b int) bool { return samples[a] < samples[b] })

	return &job.Summary{
		JobID:       j.ID,
		Total:       j.Total,
		Success:     snap.Progress.Success,
		Failure:     snap.Progress.Failure,
		Timeout:     snap.Progress.Timeout,
		Error:       snap.Progress.Error,
		Unsuccess:   snap.Progress.Unsuccess,
		StatusCodes: snap.Progress.StatusCodes,
		QPS:         snap.Progress.QPS,
		ElapsedMs:   snap.ElapsedMs,
		Latency:     latencyStat(samples),
		TypeCount:   int(typeCount),
	}
}

// latencyStat 由已排序的耗时样本计算 min / p50 / p95 / p99 / max
func latencyStat(sorted []int64) job.Latency {
	if len(sorted) == 0 {
		return job.Latency{}
	}
	return job.Latency{
		Min: sorted[0],
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		P99: percentile(sorted, 99),
		Max: sorted[len(sorted)-1],
	}
}

// percentile 按 nearest-rank 方式取分位值，sorted 必须已升序排列
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := (len(sorted)*p+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// writeSummary 把汇总结果写入 <任务目录>/summary.json
func writeSummary(dir string, sum *job.Summary) error {
	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, job.SummaryFileName), data, 0o644)
}
