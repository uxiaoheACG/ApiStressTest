package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigRoot 压测配置存档目录（相对进程工作目录），文件名是 <jobID>.json
const ConfigRoot = "./config_test"

// SaveTestConfig 把一次压测的完整配置写入存档，供历史页查看与「重新测试」复用。
//
// 存档包含 auth token 等凭据，因此文件权限设为 0600（仅当前用户可读写）。
// jobID 必须是 8 位十六进制，避免用任务 ID 拼出越界路径。
func SaveTestConfig(jobID string, cfg any) error {
	if !ValidJobID(jobID) {
		return fmt.Errorf("非法的任务 ID: %q", jobID)
	}

	if err := os.MkdirAll(ConfigRoot, 0o755); err != nil {
		return fmt.Errorf("创建配置目录 %s 失败: %w", ConfigRoot, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化压测配置失败: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(ConfigRoot, jobID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置存档 %s 失败: %w", path, err)
	}
	return nil
}

// LoadTestConfig 读取任务配置存档，返回原始 JSON（由调用方直接透传给前端）。
// 文件不存在时返回的错误满足 os.IsNotExist。
func LoadTestConfig(jobID string) (json.RawMessage, error) {
	if !ValidJobID(jobID) {
		return nil, fmt.Errorf("非法的任务 ID: %q", jobID)
	}

	data, err := os.ReadFile(filepath.Join(ConfigRoot, jobID+".json"))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
