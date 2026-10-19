package SendAndWrite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"ast/analysis"
	"ast/job"
	"ast/model"
)

const (
	// samplesDirName 任务目录下存放「每种返回类型一份样本」的子目录
	samplesDirName = "samples"

	// flushInterval 计数回写间隔：低速任务也能在界面上看到最新次数
	flushInterval = time.Second

	// sampleChanCap 样本事件队列容量（只承载「新类型」，数量级等于类型数）
	sampleChanCap = 256

	// maxLatencySamples 最多收集多少个请求的耗时用于算分位数；超过则放弃分位统计
	maxLatencySamples = 5_000_000
)

// sampleEvent 一个新出现的返回类型：样本内容 + 落库元信息
type sampleEvent struct {
	stat   analysis.TypeStat
	sample []byte
}

// Sink 单个任务的结果聚合与落库器。
//
// 与旧实现（每个请求写一行 jsonl）的区别：
//   - 热路径只做内存计数（Aggregator），不序列化、不碰磁盘
//   - 每个新出现的返回类型投递一次样本事件，由唯一的写协程落样本 + 写 SQLite
//   - 任务结束时把最终计数批量回写 SQLite（写入量是 O(类型数)，不是 O(请求数)）
//
// 生命周期：Close 必须在所有 worker 退出后调用（由 master.Run 的等待保证）。
type Sink struct {
	j      *job.Job
	jobDir string

	bizCodeField string
	agg          *Aggregator

	db     *sql.DB
	writer *analysis.Writer

	ch   chan sampleEvent
	stop chan struct{}
	done chan struct{}

	closeOnce sync.Once
	closeErr  error

	// latency 每个请求的耗时（毫秒），下标即请求序号；-1 表示该序号未执行。
	// 各 worker 写不同下标，无需加锁。
	latency []int64

	errMsg atomic.Pointer[string]
}

// NewSink 准备任务目录、样本目录与分析数据库，并启动写协程。
func NewSink(j *job.Job, cfg model.TestConfig) (*Sink, error) {
	s := &Sink{
		j:            j,
		jobDir:       j.Dir,
		bizCodeField: cfg.BizCodeField,
		agg:          NewAggregator(j.ID),
		ch:           make(chan sampleEvent, sampleChanCap),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
	s.initLatency(j.Total)

	if err := os.MkdirAll(filepath.Join(j.Dir, samplesDirName), 0o755); err != nil {
		return nil, fmt.Errorf("创建样本目录失败: %w", err)
	}

	db, err := analysis.Open(j.Dir)
	if err != nil {
		return nil, err
	}
	s.db = db
	s.writer = analysis.NewWriter(db, j.ID)
	if err := s.writer.InitMeta(j.Total, j.CreatedAt()); err != nil {
		_ = db.Close()
		return nil, err
	}

	go s.loop()
	return s, nil
}

// Record 记录一次请求结果。
//
// 先做内存计数；只有该返回类型首次出现时才调用 buildSample 生成样本，
// 避免每个请求都做 JSON 序列化。buildSample 收到的 bizCode 是实际参与指纹的值
// （类型过多时可能被改写为 overflow）。
func (s *Sink) Record(in HitInput, buildSample func(fingerprint, bizCode string) []byte) {
	hit := s.agg.Hit(in)
	if !hit.First {
		return
	}

	sample := buildSample(hit.Fingerprint, hit.BizCode)
	if len(sample) == 0 {
		return
	}

	ev := sampleEvent{
		stat: analysis.TypeStat{
			Fingerprint: hit.Fingerprint,
			StatusCode:  in.StatusCode,
			BizCode:     hit.BizCode,
			Kind:        in.Kind,
			ErrKind:     in.ErrKind,
			FirstSeenAt: recordTime(in.At),
			LastSeenAt:  recordTime(in.At),
			SampleFile:  sampleFileName(hit.Fingerprint),
			ReqMethod:   in.ReqMethod,
			ReqURL:      in.ReqURL,
		},
		sample: sample,
	}

	select {
	case s.ch <- ev:
	case <-s.stop:
	}
}

// BizCode 按任务配置的字段路径从响应体提取业务码
func (s *Sink) BizCode(body []byte) string {
	return ExtractBizCode(body, s.bizCodeField)
}

// loop 唯一的写协程：处理新类型事件，并按固定间隔回写计数
func (s *Sink) loop() {
	defer close(s.done)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case ev := <-s.ch:
			s.writeType(ev)
		case <-ticker.C:
			s.flushCounts()
		case <-s.stop:
			s.drain()
			return
		}
	}
}

// drain 处理完队列里剩余的新类型事件，并做最后一次计数回写
func (s *Sink) drain() {
	for {
		select {
		case ev := <-s.ch:
			s.writeType(ev)
		default:
			s.flushCounts()
			return
		}
	}
}

// writeType 落一份样本文件 + 写一条类型记录
func (s *Sink) writeType(ev sampleEvent) {
	path := filepath.Join(s.jobDir, samplesDirName, ev.stat.SampleFile)
	if err := os.WriteFile(path, ev.sample, 0o644); err != nil {
		s.setErr(fmt.Errorf("写入样本 %s 失败: %w", ev.stat.SampleFile, err))
		ev.stat.SampleFile = "" // 文件没写成，库里不留假路径
	}
	if err := s.writer.InsertType(ev.stat); err != nil {
		s.setErr(err)
	}
}

// flushCounts 把内存里的计数批量回写 SQLite
func (s *Sink) flushCounts() {
	items := s.agg.Snapshot()
	updates := make([]analysis.CountUpdate, 0, len(items))
	for _, item := range items {
		updates = append(updates, analysis.CountUpdate{
			Fingerprint: item.Fingerprint,
			Count:       item.Count,
			LastSeen:    item.LastSeen,
		})
	}
	if err := s.writer.UpdateCounts(updates); err != nil {
		s.setErr(err)
	}
}

// Close 收尾：停止写协程、落完剩余样本与计数、写任务元信息、关闭数据库。
// 必须在所有 worker 退出之后调用，可重复调用。
func (s *Sink) Close(done int64, canceled bool) error {
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done

		if err := s.writer.Finish(done, canceled); err != nil {
			s.closeErr = err
		}
		if err := s.db.Close(); err != nil && s.closeErr == nil {
			s.closeErr = fmt.Errorf("关闭分析数据库失败: %w", err)
		}
		if err := s.Err(); err != nil && s.closeErr == nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}

// Err 返回首个落库错误，没有则返回 nil
func (s *Sink) Err() error {
	if p := s.errMsg.Load(); p != nil {
		return errors.New(*p)
	}
	return nil
}

// Latencies 返回每个请求的耗时（毫秒），-1 表示该序号未执行。
// 只能在所有 worker 退出后读取。
func (s *Sink) Latencies() []int64 { return s.latency }

// TypeCount 当前已出现的返回类型数（供实时展示）
func (s *Sink) TypeCount() int64 { return s.agg.TypeCount() }

// initLatency 预分配耗时收集切片；请求数过大时放弃分位统计，避免占用过多内存
func (s *Sink) initLatency(total int) {
	if total <= 0 || total > maxLatencySamples {
		return
	}
	s.latency = make([]int64, total)
	for i := range s.latency {
		s.latency[i] = -1
	}
}

// setLatency 记录第 index 个请求的耗时（各 worker 写不同下标，无需加锁）
func (s *Sink) setLatency(index int, ms int64) {
	if index < 0 || index >= len(s.latency) {
		return
	}
	s.latency[index] = ms
}

// setErr 记录首个落库错误
func (s *Sink) setErr(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	s.errMsg.CompareAndSwap(nil, &msg)
}
