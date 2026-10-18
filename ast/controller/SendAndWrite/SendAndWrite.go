package SendAndWrite

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ast/controller/clone"
	"ast/controller/master"
	"ast/controller/newCLient"
	"ast/job"
	"ast/model"
)

// maxRunningJobs 同时运行的压测任务上限，避免多任务叠加打爆本机连接与文件句柄
const maxRunningJobs = 4

// CreateChanAndFile 启动一次压测任务：
//
//	建结果目录 → 克隆请求模板 → 注册任务 → 打开结果文件并启动写协程 → 后台并发跑请求
//
// 函数立即返回，任务在后台继续执行；进度和最终结果通过 job.Default（GET /jobs/:id）查询，
// 结束时会写出 <任务目录>/summary.json。返回的 *job.Job 供调用方回给前端。
func CreateChanAndFile(req *http.Request, cfg model.TestConfig) (*job.Job, error) {
	if req == nil {
		return nil, errors.New("请求为空，无法压测")
	}

	id := job.NewID()
	dir := filepath.Join(job.DataRoot, time.Now().Format("2006-01-02_15-04-05")+"_"+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建结果目录失败: %w", err)
	}

	// 请求模板：克隆后的副本带 GetBody，可被并发派生出互不干扰的请求
	tmpl, err := clone.CloneRequest(req)
	if err != nil {
		return nil, fmt.Errorf("复制请求失败: %w", err)
	}
	if tmpl == nil {
		return nil, errors.New("请求为空，无法压测")
	}

	j := job.New(id, dir, cfg.Concurrency.TotalRequests)
	if err := job.Default.Add(j, maxRunningJobs); err != nil {
		_ = os.Remove(dir) // 任务没起来，不留下空目录
		return nil, err
	}

	// 存档本次压测的完整参数，供历史页查看与「重新测试」复用；
	// 存档失败不影响压测本身，只记日志
	if err := job.SaveTestConfig(id, cfg); err != nil {
		log.Printf("[job %s] 保存压测配置失败: %v", id, err)
	}

	sink, err := NewSink(j, cfg)
	if err != nil {
		j.Fail(err)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	j.SetCancel(cancel)

	client := newCLient.NewClient(cfg.Concurrency)
	snap := snapshotRequest(tmpl)

	go run(ctx, cancel, j, sink, client, tmpl, snap, cfg)

	return j, nil
}

// run 执行一次压测任务的完整生命周期：派发请求 → 等待全部结束 → 落盘 → 汇总。
func run(ctx context.Context, cancel context.CancelFunc, j *job.Job, sink *Sink, client *http.Client, tmpl *http.Request, snap reqSnapshot, cfg model.TestConfig) {
	// 先注册，最后执行：收尾逻辑里读 ctx.Err() 判断是否被取消
	defer cancel()

	var runErr error
	defer func() {
		if r := recover(); r != nil {
			runErr = fmt.Errorf("任务 panic: %v", r)
			log.Printf("[job %s] %v", j.ID, runErr)
		}

		// 必须在所有 worker 退出后关闭：把剩余缓冲落盘并关闭文件
		if err := sink.Close(); err != nil {
			log.Printf("[job %s] 关闭结果文件失败: %v", j.ID, err)
			if runErr == nil {
				runErr = err
			}
		}
		client.CloseIdleConnections()

		if runErr != nil {
			j.Fail(runErr)
			return
		}

		sum := buildSummary(j, sink.Latencies())
		if err := writeSummary(j.Dir, sum); err != nil {
			log.Printf("[job %s] 写 %s 失败: %v", j.ID, job.SummaryFileName, err)
		}
		j.Complete(sum, ctx.Err() != nil)

		log.Printf("[job %s] 压测结束（%s）：成功 %d，失败 %d（超时 %d / 非 2xx %d / 其它错误 %d），耗时 %dms，QPS %.1f，结果目录 %s",
			j.ID, j.Snapshot().Status, sum.Success, sum.Failure, sum.Timeout, sum.Unsuccess, sum.Error, sum.ElapsedMs, sum.QPS, j.Dir)
	}()

	j.Start()
	log.Printf("[job %s] 开始压测 %s %s：并发 %d，总请求 %d，超时 %dms，保存模式 %d，结果目录 %s",
		j.ID, tmpl.Method, tmpl.URL.String(), cfg.Concurrency.Count, j.Total,
		cfg.Concurrency.Timeout, cfg.SaveData, j.Dir)

	master.Run(ctx, j.Total, cfg.Concurrency.Count, func(ctx context.Context, i int) {
		sink.Do(ctx, i, tmpl, snap, client)
	})
}
