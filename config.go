package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tailscale/hujson"
)

// configEnvVar 是可用于显式指定配置路径的环境变量。
const configEnvVar = "OPENCODE_CONFIG"

// configFileNames 是候选配置文件名，按优先级排列（优先 JSONC）。
var configFileNames = []string{"opencode.jsonc", "opencode.json"}

// CandidatePaths 返回配置文件的候选搜索路径，按优先级排列。
// 覆盖当前目录、XDG 风格 ~/.config/opencode、以及系统用户配置目录。
func CandidatePaths() []string {
	var out []string
	for _, name := range configFileNames {
		out = append(out, filepath.Join(".", name))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, ".config", "opencode", "opencode.jsonc"),
			filepath.Join(home, ".config", "opencode", "opencode.json"),
			filepath.Join(home, ".opencode", "opencode.jsonc"),
		)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		out = append(out,
			filepath.Join(dir, "opencode", "opencode.jsonc"),
			filepath.Join(dir, "opencode", "opencode.json"),
		)
	}
	return out
}

// ResolveConfigPath 按「显式参数 > 环境变量 > 候选路径」的顺序定位配置文件。
func ResolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		if fi, err := os.Stat(explicit); err == nil && !fi.IsDir() {
			return explicit, nil
		}
		return "", fmt.Errorf("指定的配置文件不存在: %s", explicit)
	}

	if env := os.Getenv(configEnvVar); env != "" {
		if fi, err := os.Stat(env); err == nil && !fi.IsDir() {
			return env, nil
		}
	}

	for _, p := range CandidatePaths() {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	return "", fmt.Errorf(
		"未找到 OpenCode 配置文件。已尝试：当前目录的 %v、~%s、~/.config/opencode/。\n"+
			"可用 -config <路径> 显式指定，或设置环境变量 %s",
		configFileNames, string(os.PathSeparator)+filepath.Join(".config", "opencode"), configEnvVar)
}

// LoadConfig 读取并解析 JSONC 配置。返回的 Value 完整保留注释与原始排版。
func LoadConfig(path string) (*hujson.Value, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	v, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("解析 JSONC 失败，文件可能存在语法错误: %w", err)
	}
	return &v, nil
}

// SaveConfig 将配置写回磁盘；backup 为 true 时先生成 .bak。
// 返回备份文件路径（未备份时返回空字符串）。
func SaveConfig(path string, v *hujson.Value, backup bool) (string, error) {
	if backup {
		original, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("读取原文件用于备份失败: %w", err)
		}
		bakPath := path + ".bak"
		if err := os.WriteFile(bakPath, original, 0o644); err != nil {
			return "", fmt.Errorf("写入备份文件失败: %w", err)
		}
		if err := os.WriteFile(path, v.Pack(), 0o644); err != nil {
			return "", fmt.Errorf("写入配置文件失败: %w", err)
		}
		return bakPath, nil
	}

	if err := os.WriteFile(path, v.Pack(), 0o644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %w", err)
	}
	return "", nil
}
