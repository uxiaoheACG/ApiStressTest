package job

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	// SuccessFileName 成功记录文件名（NDJSON，一行一条）
	SuccessFileName = "success.jsonl"

	// FailureFileName 失败/超时记录文件名（NDJSON，一行一条）
	FailureFileName = "failures.jsonl"

	// defaultRecordLimit 明细分页的默认条数
	defaultRecordLimit = 100

	// maxRecordLineBytes 单行明细的最大长度（失败记录含请求/响应体，正常不超过几十 KB）
	maxRecordLineBytes = 8 << 20
)

// jobIDPattern 任务 ID 只允许 8 位小写十六进制（NewID 的生成格式），
// 用它限制拼进磁盘路径的输入，避免路径穿越。
var jobIDPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// ValidJobID 校验任务 ID 格式
func ValidJobID(id string) bool { return jobIDPattern.MatchString(id) }

// RecordPage 一类结果明细的分页结果
type RecordPage struct {
	Kind   string            `json:"kind"`   // success / failures
	File   string            `json:"file"`   // 实际读取的文件名
	Offset int               `json:"offset"` // 起始行号（从 0 开始）
	Limit  int               `json:"limit"`  // 本次最多返回条数
	Total  int               `json:"total"`  // 该文件的有效记录总数
	Lines  []json.RawMessage `json:"lines"`  // 本页记录，保持落盘时的原始结构
	Note   string            `json:"note,omitempty"`
}

// FindDir 按 jobID 定位结果目录：优先使用内存中的任务，其次扫描 root 下的任务目录。
// 后者保证进程重启后仍能读历史任务的明细。
func FindDir(root, id string) (string, bool) {
	if j, ok := Default.Get(id); ok {
		if info, err := os.Stat(j.Dir); err == nil && info.IsDir() {
			return j.Dir, true
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		_, dirID, ok := parseDirName(e.Name())
		if ok && dirID == id {
			return filepath.Join(root, e.Name()), true
		}
	}
	return "", false
}

// LoadRecords 从任务目录里分页读取一类结果明细。
//
// 仅用于旧版本任务：新版本把结果按「返回类型」聚合进 result.db，
// 不再写 success.jsonl / failures.jsonl。
// kind 取 success（success.jsonl）或 failures/failure（failures.jsonl）。
// 文件不存在时返回空列表 + Note，而不是错误。
// 非法 JSON 行（进程被强杀导致的半行）会被跳过，不参与分页。
func LoadRecords(dir, kind string, offset, limit int) (RecordPage, error) {
	fileName := FailureFileName
	switch kind {
	case "success":
		kind = "success"
		fileName = SuccessFileName
	case "failure", "failures":
		kind = "failures"
	default:
		return RecordPage{}, fmt.Errorf("不支持的明细类型: %q（可选 success / failures）", kind)
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultRecordLimit
	}

	page := RecordPage{
		Kind:   kind,
		File:   fileName,
		Offset: offset,
		Limit:  limit,
		Lines:  []json.RawMessage{},
	}

	f, err := os.Open(filepath.Join(dir, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			page.Note = "该任务没有生成 " + fileName + "（新版本任务不再按请求写明细文件，请查看类型分析）"
			return page, nil
		}
		return page, fmt.Errorf("打开 %s 失败: %w", fileName, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), maxRecordLineBytes)

	valid := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || !json.Valid(line) {
			continue
		}
		page.Total++
		if valid >= offset && len(page.Lines) < limit {
			buf := make([]byte, len(line))
			copy(buf, line)
			page.Lines = append(page.Lines, json.RawMessage(buf))
		}
		valid++
	}
	if err := scanner.Err(); err != nil {
		return page, fmt.Errorf("读取 %s 失败: %w", fileName, err)
	}

	return page, nil
}
