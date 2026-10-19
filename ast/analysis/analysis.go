// Package analysis 用 SQLite 保存一次压测任务的「返回类型」聚合结果。
//
// 与旧实现（每个请求写一行 jsonl）的区别：压测热路径只在内存里累加计数，
// 每个「指纹」（jobId + HTTP 状态码 + 业务码）只落一行记录 + 一份样本文件，
// 因此写入量是 O(返回类型数) 而不是 O(请求数)。
//
// 纯 Go 实现（modernc.org/sqlite），不依赖 CGO，Windows 上无需 gcc。
package analysis

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// FileName 每个任务目录下的分析数据库文件名
const FileName = "result.db"

// 结果分类，与压测过程里的判定保持一致
const (
	KindSuccess   = "success"   // 2xx
	KindUnsuccess = "unsuccess" // 有响应但非 2xx
	KindTimeout   = "timeout"   // 请求超时
	KindError     = "error"     // 其它网络/协议错误
)

// TypeStat 一种返回类型的聚合统计
type TypeStat struct {
	Fingerprint string `json:"fingerprint"`
	StatusCode  int    `json:"statusCode"`
	BizCode     string `json:"bizCode"`
	Kind        string `json:"kind"`
	ErrKind     string `json:"errKind"`
	Count       int64  `json:"count"`
	FirstSeenAt string `json:"firstSeenAt"`
	LastSeenAt  string `json:"lastSeenAt"`
	SampleFile  string `json:"sampleFile"`
	ReqMethod   string `json:"reqMethod"`
	ReqURL      string `json:"reqUrl"`
}

// Meta 任务级元信息
type Meta struct {
	JobID      string `json:"jobId"`
	Total      int    `json:"total"`
	Done       int64  `json:"done"`
	Canceled   bool   `json:"canceled"`
	CreatedAt  string `json:"createdAt"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

// Report 一次任务的完整分析结果
type Report struct {
	Meta
	TypeCount int        `json:"typeCount"`
	Types     []TypeStat `json:"types"`
}

// CountUpdate 一次计数回写（任务结束或定期 flush 时使用）
type CountUpdate struct {
	Fingerprint string
	Count       int64
	LastSeen    time.Time
}

// schemaStatements 建表语句，逐条执行以保证兼容性
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS request_types (
		job_id        TEXT NOT NULL,
		fingerprint   TEXT NOT NULL,
		status_code   INTEGER NOT NULL,
		biz_code      TEXT NOT NULL DEFAULT '',
		kind          TEXT NOT NULL,
		err_kind      TEXT NOT NULL DEFAULT '',
		hit_count     INTEGER NOT NULL DEFAULT 0,
		first_seen_at TEXT NOT NULL,
		last_seen_at  TEXT NOT NULL,
		sample_file   TEXT NOT NULL DEFAULT '',
		req_method    TEXT NOT NULL DEFAULT '',
		req_url       TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (job_id, fingerprint)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_request_types_hits
		ON request_types (job_id, hit_count DESC)`,
	`CREATE TABLE IF NOT EXISTS job_meta (
		job_id      TEXT PRIMARY KEY,
		total       INTEGER NOT NULL,
		done        INTEGER NOT NULL DEFAULT 0,
		canceled    INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT ''
	)`,
}

// timeLayout 与 SendAndWrite/record.go 的记录时间格式保持一致
const timeLayout = time.RFC3339Nano

// Open 打开（不存在则创建）任务目录下的分析数据库并建表。
//
// 只给单个写协程使用：内部把连接池限制为 1，避免 SQLite 的并发写锁竞争。
func Open(jobDir string) (*sql.DB, error) {
	path := filepath.Join(jobDir, FileName)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开分析数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1)

	// WAL 让读不阻塞写；NORMAL 在压测这种「可丢最后一批」的场景下足够
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("设置 PRAGMA 失败: %w", err)
		}
	}

	for _, stmt := range schemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("创建表失败: %w", err)
		}
	}

	return db, nil
}

// Writer 分析数据的写入句柄，必须只被单个协程调用
type Writer struct {
	db    *sql.DB
	jobID string
}

// NewWriter 创建写入句柄
func NewWriter(db *sql.DB, jobID string) *Writer {
	return &Writer{db: db, jobID: jobID}
}

// InitMeta 写入任务元信息（任务开始时就写，便于中途查看）
func (w *Writer) InitMeta(total int, createdAt time.Time) error {
	_, err := w.db.Exec(
		`INSERT INTO job_meta (job_id, total, done, canceled, created_at, finished_at)
		 VALUES (?, ?, 0, 0, ?, '')
		 ON CONFLICT(job_id) DO UPDATE SET total = excluded.total`,
		w.jobID, total, createdAt.Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("写入任务元信息失败: %w", err)
	}
	return nil
}

// InsertType 插入一种新的返回类型（首次出现时调用，计数为 1）
func (w *Writer) InsertType(st TypeStat) error {
	_, err := w.db.Exec(
		`INSERT INTO request_types
			(job_id, fingerprint, status_code, biz_code, kind, err_kind, hit_count,
			 first_seen_at, last_seen_at, sample_file, req_method, req_url)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(job_id, fingerprint) DO UPDATE SET
			hit_count = hit_count + 1,
			last_seen_at = excluded.last_seen_at`,
		w.jobID, st.Fingerprint, st.StatusCode, st.BizCode, st.Kind, st.ErrKind, 1,
		st.FirstSeenAt, st.LastSeenAt, st.SampleFile, st.ReqMethod, st.ReqURL,
	)
	if err != nil {
		return fmt.Errorf("写入返回类型失败: %w", err)
	}
	return nil
}

// UpdateCounts 批量回写计数，在一个事务里完成
func (w *Writer) UpdateCounts(updates []CountUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := w.db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(
		`UPDATE request_types SET hit_count = ?, last_seen_at = ?
		 WHERE job_id = ? AND fingerprint = ?`,
	)
	if err != nil {
		return fmt.Errorf("准备计数更新语句失败: %w", err)
	}
	defer stmt.Close()

	for _, item := range updates {
		if _, err := stmt.Exec(item.Count, item.LastSeen.Format(timeLayout), w.jobID, item.Fingerprint); err != nil {
			return fmt.Errorf("更新计数失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交计数更新失败: %w", err)
	}
	return nil
}

// Finish 标记任务结束，写入完成数与是否被取消
func (w *Writer) Finish(done int64, canceled bool) error {
	canceledFlag := 0
	if canceled {
		canceledFlag = 1
	}
	_, err := w.db.Exec(
		`UPDATE job_meta SET done = ?, canceled = ?, finished_at = ? WHERE job_id = ?`,
		done, canceledFlag, time.Now().Format(timeLayout), w.jobID,
	)
	if err != nil {
		return fmt.Errorf("更新任务元信息失败: %w", err)
	}
	return nil
}

// Load 读取任务目录下的分析结果。
// 数据库文件不存在时返回的错误满足 os.IsNotExist（旧版本任务没有该文件）。
func Load(jobDir, jobID string) (Report, error) {
	path := filepath.Join(jobDir, FileName)
	if _, err := os.Stat(path); err != nil {
		return Report{}, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return Report{}, fmt.Errorf("打开分析数据库失败: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	report := Report{Meta: Meta{JobID: jobID}, Types: []TypeStat{}}

	var canceled int
	err = db.QueryRow(
		`SELECT job_id, total, done, canceled, created_at, finished_at FROM job_meta WHERE job_id = ?`,
		jobID,
	).Scan(&report.JobID, &report.Total, &report.Done, &canceled, &report.CreatedAt, &report.FinishedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// 任务刚开始、还没写元信息：返回空报告
		return report, nil
	case err != nil:
		return Report{}, fmt.Errorf("读取任务元信息失败: %w", err)
	}
	report.Canceled = canceled == 1

	rows, err := db.Query(
		`SELECT fingerprint, status_code, biz_code, kind, err_kind, hit_count,
		        first_seen_at, last_seen_at, sample_file, req_method, req_url
		 FROM request_types WHERE job_id = ? ORDER BY hit_count DESC, fingerprint ASC`,
		jobID,
	)
	if err != nil {
		return Report{}, fmt.Errorf("读取返回类型失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var st TypeStat
		if err := rows.Scan(
			&st.Fingerprint, &st.StatusCode, &st.BizCode, &st.Kind, &st.ErrKind, &st.Count,
			&st.FirstSeenAt, &st.LastSeenAt, &st.SampleFile, &st.ReqMethod, &st.ReqURL,
		); err != nil {
			return Report{}, fmt.Errorf("解析返回类型失败: %w", err)
		}
		report.Types = append(report.Types, st)
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("遍历返回类型失败: %w", err)
	}

	report.TypeCount = len(report.Types)
	return report, nil
}
