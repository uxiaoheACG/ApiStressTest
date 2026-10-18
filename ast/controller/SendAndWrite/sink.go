package SendAndWrite

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"ast/job"
	"ast/model"
)

// resultKind 结果文件的类别，同时用作 resultFiles 的下标
type resultKind int

const (
	kindSuccess resultKind = iota // 成功响应（success.jsonl）
	kindFailure                   // 失败/超时响应（failures.jsonl）
	kindCount                     // 类别总数
)

// resultFiles 各个结果文件名
var resultFiles = [kindCount]string{"success.jsonl", "failures.jsonl"}

const (
	// bufSize 每个结果文件的写缓冲大小
	bufSize = 256 << 10

	// flushThreshold 缓冲累积到该字节数就主动落盘
	flushThreshold = 64 << 10

	// flushInterval 兜底落盘间隔：低速率时也能及时把数据写到磁盘，方便 tail -f
	flushInterval = 200 * time.Millisecond

	// chanFactor / chanMin / chanMax 结果 channel 容量：min(chanFactor*并发数, chanMax)，下限 chanMin
	chanFactor = 4
	chanMin    = 1024
	chanMax    = 65536

	// maxLatencySamples 最多收集多少个请求的耗时用于算分位数；超过则放弃分位统计（不额外占内存）
	maxLatencySamples = 5_000_000
)

// record 一条待落盘记录（line 已包含结尾换行）
type record struct {
	kind resultKind
	line []byte
}

// Sink 单个任务的结果落盘器。
//
// 并发模型：任意数量的 worker 调用 Push 投递已序列化的一行数据，由唯一的写协程
// 用 bufio 批量写入文件。因此热路径上没有锁、每次请求也不产生系统调用，
// 磁盘写入被合并成大块，压测吞吐不受落盘影响。
//
// 生命周期：Close 必须在所有 worker 退出后调用（由 master.Run 的等待保证）。
type Sink struct {
	j        *job.Job
	saveMode int
	active   bool

	files [kindCount]*os.File
	bufs  [kindCount]*bufio.Writer

	ch   chan record
	stop chan struct{}
	done chan struct{}

	closeOnce sync.Once
	closeErr  error

	// latency 每个请求的耗时（毫秒），下标即请求序号；-1 表示该序号未执行。
	// 各 worker 写不同下标，无需加锁。
	latency []int64

	errMsg atomic.Pointer[string]
}

// NewSink 为任务创建结果文件并启动写协程。
//
// saveMode 对应 TestConfig.SaveData：
//
//	0 只统计不落盘 | 1 只写失败详情 | 2 成功简写 + 失败详情 | 3 成功失败都写详情
func NewSink(j *job.Job, cfg model.TestConfig) (*Sink, error) {
	s := &Sink{
		j:        j,
		saveMode: cfg.SaveData,
		ch:       make(chan record, chanCap(cfg.Concurrency.Count)),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	s.initLatency(j.Total)

	for k := kindSuccess; k < kindCount; k++ {
		if !s.needFile(k) {
			continue
		}
		path := filepath.Join(j.Dir, resultFiles[k])
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_ = s.closeFiles()
			return nil, fmt.Errorf("创建结果文件 %s 失败: %w", resultFiles[k], err)
		}
		s.files[k] = f
		s.bufs[k] = bufio.NewWriterSize(f, bufSize)
	}

	s.active = s.bufs[kindSuccess] != nil || s.bufs[kindFailure] != nil
	if s.active {
		go s.loop()
	} else {
		// 不落盘（saveMode=0）时没有写协程，done 直接关闭，Close 不会阻塞
		close(s.done)
	}

	return s, nil
}

// Push 投递一条已序列化的记录。Sink 已关闭或不需要落盘时直接丢弃，不会阻塞调用方。
func (s *Sink) Push(kind resultKind, line []byte) {
	if !s.active || len(line) == 0 {
		return
	}
	select {
	case s.ch <- record{kind: kind, line: line}:
	case <-s.stop:
	}
}

// Close 停止写协程、把缓冲落盘并关闭文件。必须在所有 worker 退出后调用，可重复调用。
func (s *Sink) Close() error {
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done
		s.closeErr = s.closeFiles()
		if err := s.Err(); err != nil && s.closeErr == nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}

// Err 返回首个写盘错误，没有则返回 nil
func (s *Sink) Err() error {
	if p := s.errMsg.Load(); p != nil {
		return errors.New(*p)
	}
	return nil
}

// Latencies 返回每个请求的耗时（毫秒），-1 表示该序号未执行。
// 只能在所有 worker 退出后读取。
func (s *Sink) Latencies() []int64 { return s.latency }

// loop 唯一的写协程：从 channel 取记录写入 bufio，按阈值或定时落盘
func (s *Sink) loop() {
	defer close(s.done)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case rec := <-s.ch:
			s.write(rec)
		case <-ticker.C:
			s.flushAll()
		case <-s.stop:
			s.drain()
			s.flushAll()
			return
		}
	}
}

// drain 把 channel 里剩余的记录写空
func (s *Sink) drain() {
	for {
		select {
		case rec := <-s.ch:
			s.write(rec)
		default:
			return
		}
	}
}

// write 把一行记录写入对应文件的缓冲，缓冲到阈值时落盘
func (s *Sink) write(rec record) {
	if rec.kind < 0 || rec.kind >= kindCount {
		return
	}
	buf := s.bufs[rec.kind]
	if buf == nil {
		return
	}
	if _, err := buf.Write(rec.line); err != nil {
		s.setErr(err)
		return
	}
	if buf.Buffered() >= flushThreshold {
		if err := buf.Flush(); err != nil {
			s.setErr(err)
		}
	}
}

// flushAll 把所有缓冲落盘
func (s *Sink) flushAll() {
	for k := kindSuccess; k < kindCount; k++ {
		if s.bufs[k] == nil {
			continue
		}
		if err := s.bufs[k].Flush(); err != nil {
			s.setErr(err)
		}
	}
}

// closeFiles 落盘、Sync 并关闭所有文件，返回首个错误
func (s *Sink) closeFiles() error {
	var first error
	s.flushAll()
	for k := kindSuccess; k < kindCount; k++ {
		f := s.files[k]
		if f == nil {
			continue
		}
		if err := f.Sync(); err != nil && first == nil {
			first = err
		}
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
		s.files[k] = nil
		s.bufs[k] = nil
	}
	return first
}

// setErr 记录首个写盘错误
func (s *Sink) setErr(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	s.errMsg.CompareAndSwap(nil, &msg)
}

// needFile 判断当前保存模式是否需要写该类结果文件
func (s *Sink) needFile(kind resultKind) bool {
	switch s.saveMode {
	case 0: // 只统计
		return false
	case 1: // 只写失败
		return kind == kindFailure
	default: // 2 / 3 以及未识别的取值，都写
		return true
	}
}

// detailSuccess 成功请求是否也写详细记录（SaveData=3）
func (s *Sink) detailSuccess() bool {
	return s.saveMode == 3
}

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

// chanCap 计算结果 channel 容量
func chanCap(count int) int {
	n := count * chanFactor
	if n < chanMin {
		n = chanMin
	}
	if n > chanMax {
		n = chanMax
	}
	return n
}
