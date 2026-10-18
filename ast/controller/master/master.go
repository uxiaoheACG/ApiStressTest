package master

import (
	"context"
	"log"
	"sync"
)

// Run 按 count 控制并发，总共派发 total 个请求任务，等全部结束后返回。
//
// worker 由调用方提供（发送请求 + 落盘），Run 只负责调度与等待：
//   - count <= 0 时按 1 处理；
//   - ctx 被取消时不再派发新请求，已在执行的任务由 worker 自行感知 ctx 退出；
//   - worker 内部 panic 会被捕获并记录，不影响其它请求。
func Run(ctx context.Context, total, count int, worker func(ctx context.Context, i int)) {
	if total <= 0 || worker == nil {
		return
	}
	if count <= 0 {
		count = 1
	}

	sem := make(chan struct{}, count)
	var wg sync.WaitGroup

dispatch:
	for i := 0; i < total; i++ {
		select {
		case <-ctx.Done():
			log.Printf("任务已取消，剩余 %d 个请求不再派发", total-i)
			break dispatch
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("第 %d 个请求 panic: %v", index+1, r)
				}
			}()

			worker(ctx, index)
		}(i)
	}

	// 等所有在飞的请求结束，调用方才能安全地关闭结果文件
	wg.Wait()
}
