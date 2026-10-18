package service

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
)

// DefaultConfigFile 默认配置文件路径，相对进程工作目录。
const DefaultConfigFile = "serverConfig.yaml"

// Config 服务器启动配置：集中管理监听地址、运行模式与超时参数。
type Config struct {
	Addr              string        // 监听地址，如 ":3010" 或 "127.0.0.1:3010"
	Mode              string        // gin 运行模式：debug / release / test
	ReadHeaderTimeout time.Duration // 读取请求头超时，防止慢速连接占满资源（0 表示不限制）
	IdleTimeout       time.Duration // keep-alive 空闲超时（0 表示不限制）
	ShutdownTimeout   time.Duration // 收到退出信号后等待在途请求结束的最长时间
}

// fileConfig 对应 serverConfig.yaml 的字段，全部用指针以区分「文件中未配置」与「配置为空」。
// 时长字段写成字符串（如 "10s"），与 Go 的 time.ParseDuration 格式一致。
type fileConfig struct {
	Addr              *string `yaml:"addr"`
	Mode              *string `yaml:"mode"`
	ReadHeaderTimeout *string `yaml:"readHeaderTimeout"`
	IdleTimeout       *string `yaml:"idleTimeout"`
	ShutdownTimeout   *string `yaml:"shutdownTimeout"`
}

// DefaultConfig 返回内置默认配置：监听 :3010、debug 模式。
func DefaultConfig() Config {
	return Config{
		Addr:              ":3010",
		Mode:              gin.DebugMode,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
	}
}

// LoadConfig 按「内置默认值 → serverConfig.yaml → 环境变量」的顺序加载启动配置，
// 环境变量优先级最高，便于容器/CI 临时覆盖文件。
//
// 支持的环境变量：
//
//	AST_CONFIG            配置文件路径（默认 serverConfig.yaml，相对工作目录）
//	AST_PORT / PORT       监听端口，如 9090 或 127.0.0.1:9090
//	GIN_MODE              debug / release / test
//	AST_SHUTDOWN_TIMEOUT  优雅关闭等待时间，如 15s
//
// 配置内容非法（YAML 语法错误、mode 取值错误、时长格式错误等）时返回 error，由调用方终止启动。
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	path := firstNonEmpty(os.Getenv("AST_CONFIG"), DefaultConfigFile)
	if err := loadFile(path, &cfg); err != nil {
		// 显式指定了路径却读不到，或文件存在但内容有问题 → 报错；仅默认路径不存在时回退默认配置
		if os.Getenv("AST_CONFIG") != "" || !os.IsNotExist(err) {
			return cfg, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
		log.Printf("未找到配置文件 %s，使用默认配置", path)
	} else {
		log.Printf("已加载配置文件 %s", path)
	}

	if port := firstNonEmpty(os.Getenv("AST_PORT"), os.Getenv("PORT")); port != "" {
		cfg.Addr = normalizeAddr(port)
	}

	if mode := os.Getenv(gin.EnvGinMode); mode != "" {
		cfg.Mode = strings.TrimSpace(mode)
	}

	if raw := os.Getenv("AST_SHUTDOWN_TIMEOUT"); raw != "" {
		d, err := parseDuration("AST_SHUTDOWN_TIMEOUT", raw)
		if err != nil {
			return cfg, err
		}
		cfg.ShutdownTimeout = d
	}

	if !validMode(cfg.Mode) {
		return cfg, fmt.Errorf("非法的 mode=%q，可选值: %s / %s / %s",
			cfg.Mode, gin.DebugMode, gin.ReleaseMode, gin.TestMode)
	}

	return cfg, nil
}

// loadFile 读取 YAML 配置文件，只覆盖文件中确实配置了的字段。
func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return fmt.Errorf("YAML 解析失败: %w", err)
	}

	if addr := trim(fc.Addr); addr != "" {
		cfg.Addr = normalizeAddr(addr)
	}

	if mode := trim(fc.Mode); mode != "" {
		cfg.Mode = mode
	}

	if fc.ReadHeaderTimeout != nil {
		d, err := parseDuration("readHeaderTimeout", *fc.ReadHeaderTimeout)
		if err != nil {
			return err
		}
		cfg.ReadHeaderTimeout = d
	}

	if fc.IdleTimeout != nil {
		d, err := parseDuration("idleTimeout", *fc.IdleTimeout)
		if err != nil {
			return err
		}
		cfg.IdleTimeout = d
	}

	if fc.ShutdownTimeout != nil {
		d, err := parseDuration("shutdownTimeout", *fc.ShutdownTimeout)
		if err != nil {
			return err
		}
		cfg.ShutdownTimeout = d
	}

	return nil
}

// parseDuration 解析配置中的时长（如 "10s"、"1m"），允许 0 表示不限制，负数视为非法。
func parseDuration(field, value string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || d < 0 {
		return 0, fmt.Errorf("非法的 %s=%q，写法示例: 10s", field, strings.TrimSpace(value))
	}
	return d, nil
}

// trim 读取指针字符串并去掉首尾空白，指针为 nil 时返回 ""。
func trim(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// firstNonEmpty 返回第一个非空字符串，都为空时返回 ""。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// normalizeAddr 把 "3010" 这类纯端口补全为监听地址 ":3010"，其余形式（":3010"、"host:port"）原样返回。
func normalizeAddr(addr string) string {
	if strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

// validMode 判断是否为 gin 支持的运行模式，避免 SetMode 因非法值 panic。
func validMode(mode string) bool {
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		return true
	default:
		return false
	}
}
