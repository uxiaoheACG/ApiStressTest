package model

// TestConfig 压测任务的整体配置
type TestConfig struct {
	URL           string `json:"url" binding:"required,url"`
	Method        string `json:"method" binding:"required"`
	SaveData      int    `json:"saveData" binding:"oneof=0 1 2 3"`
	SavePath      string `json:"savePath,omitempty"`
	Body          RequestBody       `json:"body"`
	Header        map[string]string `json:"header"`
	Params        map[string]string `json:"params"`
	Authorization Authorization     `json:"authorization"`
	Concurrency   ConcurrencyConfig `json:"concurrency" binding:"required"`
}

// RequestBody 请求体配置
type RequestBody struct {
	// Type 可选值：json（默认）/ form / text / raw / multipart
	Type string `json:"type" binding:"omitempty,oneof=json form text raw multipart"`
	Data any    `json:"data"`
}

// MultipartData body.type=multipart 时 body.data 的结构
type MultipartData struct {
	// Fields 普通文本字段（内容按 utf-8 原样发送，不带单独的 Content-Type）
	Fields map[string]string `json:"fields"`
	// TextParts 需要自定义 Content-Type 的文本字段（例如以 application/json 发一段 JSON）
	TextParts []MultipartTextPart `json:"textParts,omitempty"`
	// Files 文件字段，内容以 base64 传递，由后端拼进 multipart/web-form-data
	Files []MultipartFile `json:"files"`
}

// MultipartTextPart 一个可以指定 Content-Type 的文本字段
type MultipartTextPart struct {
	// Field 表单字段名
	Field string `json:"field"`
	// Value 字段值
	Value string `json:"value"`
	// ContentType 该部分的 Content-Type，为空时按普通表单字段处理
	ContentType string `json:"contentType,omitempty"`
}

// MultipartFile 一个上传文件
type MultipartFile struct {
	// Field 表单字段名
	Field string `json:"field"`
	// Filename 提交给服务端的文件名
	Filename string `json:"filename"`
	// ContentType 如 image/png，为空时按 application/octet-stream
	ContentType string `json:"contentType"`
	// ContentBase64 文件内容（标准 base64）
	ContentBase64 string `json:"contentBase64"`
}

// Authorization 认证配置
type Authorization struct {
	Type  string `json:"type" binding:"omitempty,oneof=bearer basic apikey"`
	Token string `json:"token"`
}

// ConcurrencyConfig 并发配置
type ConcurrencyConfig struct {
	Timeout       int `json:"timeout" binding:"required,min=1"`
	Count         int `json:"count" binding:"required,min=1"`
	TotalRequests int `json:"totalRequests" binding:"required,min=1"`
	Rate          int `json:"rate" binding:"omitempty,min=1"` //每秒多少并发，最少为1
}
