# ForkSync Engine

Go 核心引擎 for ForkSync — 自动同步 fork 仓库的桌面应用。

## 构建

```bash
cd engine
go build -o bins/forksync .
```

## 使用

```bash
# 扫描目录中的 git 仓库
forksync scan ~/projects

# 添加仓库
forksync add ~/projects/my-fork --upstream https://github.com/original/repo.git

# 查看状态
forksync status
forksync status --json

# 同步
forksync sync my-fork
forksync sync --all

# 解决冲突
forksync resolve my-fork
forksync resolve my-fork --ai
```

## 项目结构

```
engine/
├── main.go              # 入口
├── cmd/                 # Cobra CLI 命令
├── pkg/types/           # 共享类型（JSON 契约）
└── internal/
    ├── config/          # 配置管理 (viper)
    ├── repo/            # 仓库管理 (JSON 存储)
    ├── git/             # Git 操作 (go-git + CLI fallback)
    ├── github/          # GitHub API (fork 检测)
    ├── sync/            # 同步编排
    ├── conflict/        # 冲突检测
    ├── ai/              # AI 冲突解决 (OpenAI)
    ├── notify/          # macOS 通知
    └── scheduler/       # 定时调度
```
