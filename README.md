# opencode-models-sync

把本地 **LM Studio** 中已加载的模型，自动同步到 **OpenCode** 的配置文件（`opencode.jsonc`）里。

## 编译

```bash
go build -o build/opencode-models-sync.exe .
```

## 使用

```bash
# 预览变更（不写文件），推荐先跑一次
./build/opencode-models-sync.exe -dry-run

# 实际同步
./build/opencode-models-sync.exe

# 同步时给新增模型的显示名加上 " (local)" 后缀
./build/opencode-models-sync.exe -suffix " (local)"

# 同时清理 LM Studio 中已卸载、但配置里还留着的模型
./build/opencode-models-sync.exe -prune
```

## 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-config` | 自动查找 | 配置文件路径；也可设环境变量 `OPENCODE_CONFIG` |
| `-url` | `http://127.0.0.1:1234/v1` | LM Studio OpenAI 兼容接口地址 |
| `-provider` | `lmstudio` | 要同步的 provider 名称 |
| `-suffix` | 空 | 新增模型显示名追加的后缀，例如 `" (local)"` |
| `-prune` | `false` | 删除 LM Studio 中已不存在的模型 |
| `-create-provider` | `true` | provider 节点不存在时自动创建 |
| `-dry-run` | `false` | 仅预览，不写入 |
| `-backup` | `true` | 写入前生成 `.bak` 备份 |
| `-timeout` | `5s` | HTTP 超时 |
| `-print` | `false` | 把同步后的完整配置打印到标准输出 |
| `-v` | `false` | 列出检测到的模型清单 |
| `-version` | - | 显示版本号 |

配置文件查找顺序：显式 `-config` → 环境变量 `OPENCODE_CONFIG` → 当前目录 `opencode.jsonc` / `opencode.json` → `~/.config/opencode/` → 系统用户配置目录。

## 设计要点

- **保留注释与排版**：用 `github.com/tailscale/hujson` 解析 JSONC，直接操作有序 AST（`Members []ObjectMember`），未改动的区域**逐字节保持原样**。新增条目会克隆现有条目的排版风格，不会破坏缩进。
- **不覆盖用户自定义**：已存在的模型条目原样保留，仅在缺少 `name` 字段时补齐，绝不擅自改写你手改过的内容。
- **兼容两套 schema**：自动识别新版 `providers.<name>.models` 与旧版 `provider.<name>.models`，缺失时按对应 schema 自动创建（`settings.baseURL` 或 `options.baseURL` + `npm`）。
- **幂等**：无变更时不写文件，重复运行安全。
- **健壮**：LM Studio 未启动、端口不通、超时、返回异常均给出可读提示，不会 panic。

## 退出码

| 码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 1 | 配置定位/解析/写入失败 |
| 2 | 无法连接 LM Studio |
| 3 | LM Studio 未返回任何模型 |
