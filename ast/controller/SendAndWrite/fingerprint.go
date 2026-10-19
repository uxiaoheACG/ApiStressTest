package SendAndWrite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// maxBizCodeLen 业务码最长保留长度，超长值统一记为 "long"
	// （防止 traceId / 时间戳这类每次都不同的值把返回类型数量撑爆）
	maxBizCodeLen = 64

	// maxTypeCount 单个任务允许的最大返回类型数，超过后新类型归入 overflow
	maxTypeCount = 5000

	// aggShardCount 计数聚合的分片数，降低高并发下的锁竞争
	aggShardCount = 32
)

// bizCodeCandidates 未指定字段时，依次尝试这些常见业务码字段
var bizCodeCandidates = []string{
	"code", "errcode", "errCode", "errno", "errNo",
	"error_code", "errorCode", "resultCode", "retCode", "status",
}

var bizCodeReplacer = strings.NewReplacer("|", "_", "\r", " ", "\n", " ")

// ExtractBizCode 从响应体里提取业务码。
//
// field 非空时按点号路径取值（如 "error.code"）；为空时依次尝试常见字段名。
// 只接受标量（字符串/数字/布尔）——对象、数组、非 JSON 内容一律视为取不到，
// 由调用方退化为「只用状态码」归类。
func ExtractBizCode(body []byte, field string) string {
	if len(body) == 0 {
		return ""
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber() // 保留数字原样，避免 1000000 变成 1e+06

	var parsed map[string]any
	if err := decoder.Decode(&parsed); err != nil {
		return ""
	}

	if path := strings.TrimSpace(field); path != "" {
		return normalizeBizValue(lookupPath(parsed, path))
	}

	for _, key := range bizCodeCandidates {
		if value, ok := parsed[key]; ok {
			if code := normalizeBizValue(value); code != "" {
				return code
			}
		}
	}
	return ""
}

// lookupPath 按点号路径取值，支持多层嵌套
func lookupPath(root map[string]any, path string) any {
	var current any = root
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[part]
		if !ok {
			return nil
		}
	}
	return current
}

// normalizeBizValue 把标量业务码转成字符串；非标量返回 ""
func normalizeBizValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return clipBizCode(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case json.Number:
		return clipBizCode(v.String())
	case float64:
		return clipBizCode(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return ""
	}
}

// clipBizCode 规整业务码：去空白、限长、替换会破坏指纹分隔符的字符
func clipBizCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if len(code) > maxBizCodeLen {
		return "long"
	}
	return bizCodeReplacer.Replace(code)
}

// FingerprintOf 拼接指纹：jobId|状态码|业务码。
//
// 没有收到响应（超时、DNS、连接被拒绝等）时状态码为 0，此时业务码位置放
// "err:<错误归类>"，让超时 / DNS / TLS / 连接拒绝等各自成类，而不是全部挤进一类。
func FingerprintOf(jobID string, status int, bizCode, errKind string) string {
	if status <= 0 {
		suffix := "err:unknown"
		if trimmed := strings.TrimSpace(errKind); trimmed != "" {
			suffix = "err:" + trimmed
		}
		return jobID + "|0|" + suffix
	}
	if strings.TrimSpace(bizCode) == "" {
		bizCode = "none"
	}
	return jobID + "|" + strconv.Itoa(status) + "|" + bizCode
}

// sampleFileName 指纹 → 样本文件名（sha256 前 8 字节，避免特殊字符进文件名）
func sampleFileName(fingerprint string) string {
	sum := sha256.Sum256([]byte(fingerprint))
	return hex.EncodeToString(sum[:8]) + ".json"
}

// HitInput 一次请求结果的归类信息
type HitInput struct {
	StatusCode int
	BizCode    string
	Kind       string // success / unsuccess / timeout / error
	ErrKind    string // deadline / dns / tls / refused / reset / eof / other
	ReqMethod  string
	ReqURL     string
	At         time.Time
}

// HitResult 聚合结果
type HitResult struct {
	Fingerprint string
	BizCode     string // 实际参与指纹的业务码（可能被改写为 overflow）
	First       bool   // 是否首次出现该类型
}

// aggEntry 单个指纹的累计统计
type aggEntry struct {
	status    int
	bizCode   string
	kind      string
	errKind   string
	reqMethod string
	reqURL    string
	count     int64
	firstSeen time.Time
	lastSeen  time.Time
}

// aggShard 一个分片的计数表
type aggShard struct {
	mu    sync.Mutex
	items map[string]*aggEntry
}

// AggItem 聚合器快照里的一项
type AggItem struct {
	Fingerprint string
	BizCode     string
	Kind        string
	ErrKind     string
	StatusCode  int
	ReqMethod   string
	ReqURL      string
	Count       int64
	FirstSeen   time.Time
	LastSeen    time.Time
}

// Aggregator 按指纹在内存里累加计数：压测热路径不碰数据库，也不序列化 JSON。
type Aggregator struct {
	jobID     string
	shards    [aggShardCount]aggShard
	typeCount atomic.Int64
}

// NewAggregator 创建聚合器
func NewAggregator(jobID string) *Aggregator {
	a := &Aggregator{jobID: jobID}
	for i := range a.shards {
		a.shards[i].items = make(map[string]*aggEntry)
	}
	return a
}

// Hit 记录一次请求结果，返回指纹与该类型是否首次出现
func (a *Aggregator) Hit(in HitInput) HitResult {
	fp := FingerprintOf(a.jobID, in.StatusCode, in.BizCode, in.ErrKind)
	biz := in.BizCode
	if in.StatusCode > 0 && strings.TrimSpace(biz) == "" {
		biz = "none"
	}

	if a.typeCount.Load() >= maxTypeCount && !a.has(fp) {
		// 类型过多：新类型统一归入 overflow，避免无上限增长
		biz = "overflow"
		fp = FingerprintOf(a.jobID, in.StatusCode, biz, in.ErrKind)
	}

	shard := &a.shards[shardIndex(fp)]
	shard.mu.Lock()
	if entry, ok := shard.items[fp]; ok {
		entry.count++
		entry.lastSeen = in.At
		shard.mu.Unlock()
		return HitResult{Fingerprint: fp, BizCode: biz, First: false}
	}

	shard.items[fp] = &aggEntry{
		status:    in.StatusCode,
		bizCode:   biz,
		kind:      in.Kind,
		errKind:   in.ErrKind,
		reqMethod: in.ReqMethod,
		reqURL:    in.ReqURL,
		count:     1,
		firstSeen: in.At,
		lastSeen:  in.At,
	}
	shard.mu.Unlock()

	a.typeCount.Add(1)
	return HitResult{Fingerprint: fp, BizCode: biz, First: true}
}

// has 判断指纹是否已存在（仅用于超限判断，允许与 Hit 并发）
func (a *Aggregator) has(fp string) bool {
	shard := &a.shards[shardIndex(fp)]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	_, ok := shard.items[fp]
	return ok
}

// Snapshot 导出当前全部计数（任务结束或定期回写时调用）
func (a *Aggregator) Snapshot() []AggItem {
	items := make([]AggItem, 0, a.typeCount.Load())
	for i := range a.shards {
		shard := &a.shards[i]
		shard.mu.Lock()
		for fp, entry := range shard.items {
			items = append(items, AggItem{
				Fingerprint: fp,
				BizCode:     entry.bizCode,
				Kind:        entry.kind,
				ErrKind:     entry.errKind,
				StatusCode:  entry.status,
				ReqMethod:   entry.reqMethod,
				ReqURL:      entry.reqURL,
				Count:       entry.count,
				FirstSeen:   entry.firstSeen,
				LastSeen:    entry.lastSeen,
			})
		}
		shard.mu.Unlock()
	}
	return items
}

// TypeCount 当前已出现的返回类型数
func (a *Aggregator) TypeCount() int64 { return a.typeCount.Load() }

// shardIndex 手写 FNV-1a 取分片下标，避免每次调用都分配 hasher
func shardIndex(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h % aggShardCount
}
