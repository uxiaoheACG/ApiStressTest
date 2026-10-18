package service

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"ast/job"
)

// Run 启动服务器并阻塞，收到 SIGINT / SIGTERM 后优雅关闭在途请求。
// 返回非 nil 表示监听失败或关闭失败，由调用方决定如何退出。
func Run(engine *gin.Engine, cfg Config) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           engine,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	// ListenAndServe 阻塞在后台，主 goroutine 负责等待退出信号
	errCh := make(chan error, 1)
	go func() {
		log.Printf("服务器启动，监听 %s（mode=%s）", cfg.Addr, gin.Mode())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case err := <-errCh:
		// 监听失败（端口占用等）或服务已自行退出
		return err
	case sig := <-quit:
		log.Printf("收到信号 %s，开始优雅关闭（最多等待 %s）", sig, cfg.ShutdownTimeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Shutdown 会停止接收新连接并等待在途请求结束
	shutdownErr := srv.Shutdown(ctx)
	if shutdownErr != nil {
		log.Printf("优雅关闭超时，强制关闭: %v", shutdownErr)
		_ = srv.Close()
	}

	// 后台压测任务不占用 HTTP 连接，需要单独等它们把已产生的结果写完
	drainJobs(ctx)

	if serveErr := <-errCh; serveErr != nil {
		return serveErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}

	log.Println("服务器已退出")
	return nil
}

// drainJobs 等待后台压测任务落盘完成；等到 ctx 到期则取消任务，并再给短暂收尾时间。
func drainJobs(ctx context.Context) {
	if job.Default.RunningCount() == 0 {
		return
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("仍有 %d 个压测任务在运行，取消并等待收尾", job.Default.RunningCount())
			job.Default.CancelRunning()

			// 取消后各任务会走完 sink.Close：把缓冲落盘并写 summary
			deadline := time.Now().Add(2 * time.Second)
			for job.Default.RunningCount() > 0 && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			return

		case <-ticker.C:
			if job.Default.RunningCount() == 0 {
				return
			}
		}
	}
}
