package job

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// defaultMaxJobs 内存中默认保留的任务数
const defaultMaxJobs = 50

// ErrTooManyJobs 同时运行的任务数已达上限
var ErrTooManyJobs = errors.New("同时运行的压测任务数已达上限，请等待当前任务结束")

// Store 任务注册表：保存最近若干个任务，供进度查询使用
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
	max  int
}

// NewStore 创建注册表，max 为内存中保留的任务数上限（<=0 时使用默认值）
func NewStore(max int) *Store {
	if max <= 0 {
		max = defaultMaxJobs
	}
	return &Store{
		jobs: make(map[string]*Job, max),
		max:  max,
	}
}

// Default 全局任务注册表
var Default = NewStore(defaultMaxJobs)

// NewID 生成 8 位十六进制任务 ID
func NewID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", uint32(time.Now().UnixNano()))
	}
	return hex.EncodeToString(b)
}

// Add 注册任务。运行中的任务数达到 maxRunning 时返回 ErrTooManyJobs（<=0 表示不限制）。
func (s *Store) Add(j *Job, maxRunning int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[j.ID]; ok {
		return fmt.Errorf("任务 ID 冲突: %s", j.ID)
	}

	if maxRunning > 0 {
		running := 0
		for _, x := range s.jobs {
			if x.IsRunning() {
				running++
			}
		}
		if running >= maxRunning {
			return ErrTooManyJobs
		}
	}

	s.jobs[j.ID] = j
	s.evictLocked()
	return nil
}

// Get 按 ID 查询任务
func (s *Store) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

// List 返回所有任务，按创建时间从新到旧
func (s *Store) List() []*Job {
	s.mu.RLock()
	list := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j)
	}
	s.mu.RUnlock()

	sort.Slice(list, func(a, b int) bool {
		return list[a].CreatedAt().After(list[b].CreatedAt())
	})
	return list
}

// RunningCount 返回仍在执行的任务数
func (s *Store) RunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, j := range s.jobs {
		if j.IsRunning() {
			n++
		}
	}
	return n
}

// CancelRunning 取消所有仍在执行的任务（服务退出时调用）
func (s *Store) CancelRunning() {
	s.mu.RLock()
	list := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j)
	}
	s.mu.RUnlock()

	for _, j := range list {
		if j.IsRunning() {
			j.Cancel()
		}
	}
}

// evictLocked 超出保留上限时按创建时间淘汰最旧的已完成任务（运行中的不淘汰）
func (s *Store) evictLocked() {
	if len(s.jobs) <= s.max {
		return
	}

	type item struct {
		id string
		at time.Time
	}

	outdated := make([]item, 0, len(s.jobs))
	for id, j := range s.jobs {
		if j.IsRunning() {
			continue
		}
		outdated = append(outdated, item{id: id, at: j.CreatedAt()})
	}

	sort.Slice(outdated, func(a, b int) bool {
		return outdated[a].at.Before(outdated[b].at)
	})

	for i := 0; i < len(outdated) && len(s.jobs) > s.max; i++ {
		delete(s.jobs, outdated[i].id)
	}
}
