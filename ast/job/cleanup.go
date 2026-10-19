package job

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RemoveJobData 删除任务的结果目录（Data/<时间>_<jobID>）。
//
// 只允许删除 DataRoot 之下的路径：先用 filepath.Rel 校验相对关系，
// 避免因为传入异常路径把其它目录一起删掉。
func RemoveJobData(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("结果目录为空")
	}

	absRoot, err := filepath.Abs(DataRoot)
	if err != nil {
		return fmt.Errorf("解析结果根目录失败: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("解析任务目录失败: %w", err)
	}

	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("拒绝删除结果根目录之外的路径: %s", dir)
	}

	if err := os.RemoveAll(absDir); err != nil {
		return fmt.Errorf("删除结果目录 %s 失败: %w", dir, err)
	}
	return nil
}

// RemoveTestConfig 删除任务的参数存档（config_test/<jobID>.json）。
// 文件本来就不存在时视为成功。
func RemoveTestConfig(jobID string) error {
	if !ValidJobID(jobID) {
		return fmt.Errorf("非法的任务 ID: %q", jobID)
	}

	path := filepath.Join(ConfigRoot, jobID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除参数存档 %s 失败: %w", path, err)
	}
	return nil
}
