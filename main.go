package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

const version = "1.0.0"

// exitCode 定义各类失败的退出码，便于脚本化调用。
const (
	exitConfigErr = 1
	exitFetchErr  = 2
	exitNoModels  = 3
)

func main() {
	var (
		configPath  = flag.String("config", "", "OpenCode 配置文件路径（默认自动查找当前目录或 ~/.config/opencode/）")
		baseURL     = flag.String("url", "http://127.0.0.1:1234/v1", "LM Studio OpenAI 兼容接口地址")
		providerKey = flag.String("provider", "lmstudio", "要同步的 provider 名称")
		suffix      = flag.String("suffix", "", "新增模型显示名追加的后缀，例如 \" (local)\"")
		prune       = flag.Bool("prune", false, "删除 LM Studio 中已不存在、但配置里仍保留的模型")
		createProv  = flag.Bool("create-provider", true, "provider 节点不存在时自动创建")
		dryRun      = flag.Bool("dry-run", false, "仅预览变更，不写入文件")
		backup      = flag.Bool("backup", true, "写入前生成 .bak 备份")
		timeout     = flag.Duration("timeout", 5*time.Second, "HTTP 请求超时时间")
		printOut    = flag.Bool("print", false, "把同步后的完整配置打印到标准输出")
		showVer     = flag.Bool("version", false, "显示版本号")
		verbose     = flag.Bool("v", false, "列出详细的模型清单")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("opencode-models-sync %s\n", version)
		return
	}

	// 1. 定位配置文件
	path, err := ResolveConfigPath(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(exitConfigErr)
	}
	fmt.Println("配置文件:", path)

	// 2. 从 LM Studio 拉取模型列表
	ids, err := FetchModels(context.Background(), *baseURL, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		fmt.Fprintln(os.Stderr, "提示: 请启动 LM Studio 并开启 Local Server（默认 http://127.0.0.1:1234）")
		os.Exit(exitFetchErr)
	}
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "错误: LM Studio 未返回任何模型，请确认已加载或下载模型")
		os.Exit(exitNoModels)
	}
	fmt.Printf("LM Studio(%s) 检测到 %d 个模型\n", *baseURL, len(ids))
	if *verbose {
		for _, id := range ids {
			fmt.Println("  -", id)
		}
	}

	// 3. 读取 JSONC 配置
	root, err := LoadConfig(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(exitConfigErr)
	}

	// 4. 计算合并结果
	rep, err := SyncModels(root, ids, SyncOptions{
		ProviderKey:    *providerKey,
		NameSuffix:     *suffix,
		Prune:          *prune,
		CreateProvider: *createProv,
		BaseURL:        *baseURL,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(exitConfigErr)
	}
	rep.ConfigPath = path

	printReport(rep)

	// 5. 落地写入
	if !rep.Changed() {
		fmt.Println("配置已是最新，无需写入。")
		if *printOut {
			fmt.Println(string(root.Pack()))
		}
		return
	}

	if *dryRun {
		fmt.Println("[dry-run] 以上为预览结果，未写入文件。")
		if *printOut {
			fmt.Println(string(root.Pack()))
		}
		return
	}

	bakPath, err := SaveConfig(path, root, *backup)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(exitConfigErr)
	}
	if bakPath != "" {
		fmt.Println("已备份原配置:", bakPath)
	}
	fmt.Println("已写入:", path)
	if *printOut {
		fmt.Println(string(root.Pack()))
	}
}

// printReport 输出人可读的变更摘要。
func printReport(rep *SyncReport) {
	fmt.Println("配置结构:", rep.Schema)
	if rep.CreatedContainer {
		fmt.Println("  新建节点: providers")
	}
	if rep.CreatedProvider {
		fmt.Println("  新建节点: provider（含默认 settings.baseURL）")
	}
	if rep.CreatedModels {
		fmt.Println("  新建节点: models")
	}

	fmt.Printf("新增 %d，补全 %d，移除 %d，未变 %d\n",
		len(rep.Added), len(rep.Updated), len(rep.Removed), len(rep.Unchanged))

	printList("新增", rep.Added, "+")
	printList("补全 name", rep.Updated, "~")
	printList("移除", rep.Removed, "-")
}

func printList(label string, items []string, prefix string) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("  %s:\n", label)
	for _, it := range items {
		fmt.Printf("    %s %s\n", prefix, it)
	}
}
