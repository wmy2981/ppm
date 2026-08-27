# ppm — Windows Portproxy Manager

一个管理 Windows `netsh interface portproxy` 端口转发规则的终端 TUI 工具。单二进制、零依赖、k9s 风格快捷键操作。

## 功能

- 查看全部 v4tov4 转发规则（表格，含备注列）
- 新增 / 编辑 / 删除规则（编辑 = 删旧 + 建新，netsh 无原地更新）
- 删除前二次确认，防误操作
- 连通性测试：`t` 测当前规则，`T` 并发测全部（TCP dial 目标地址，显示延迟或错误原因）
- 监听状态列：联查 netstat，标注每条规则的监听端口是否真的在监听
- 规则备注：本地 JSON 存储（netsh 本身不支持备注），按「监听地址：端口」关联
- 导出 / 导入备份：一键导出全部规则+备注为 JSON；导入时合并去重（已存在的监听键跳过）

## 安装

从 [Releases](https://github.com/wmy2981/ppm/releases) 下载对应架构的 exe，放到任意目录运行即可。

**通过Powershell安装：**

```powershell
Invoke-WebRequest -Uri "https://github.com/wmy2981/ppm/releases/download/v0.1.0/ppm-v0.1.0-windows-amd64.exe" -OutFile ppm.exe; $path = [Environment]::GetEnvironmentVariable("Path", "User"); if ($path -notlike "*$PWD*") { [Environment]::SetEnvironmentVariable("Path", "$path;$PWD", "User") }; $env:Path = "$env:Path;$PWD"
```

**自行构建：**

```powershell
powershell -File .\scripts\build.ps1
```

## 使用

直接双击或终端运行 `ppm.exe`。程序需要管理员权限才能增删规则——启动时会弹 UAC 提权重启自身，拒绝则退出。

| 按键 | 操作 |
|---|---|
| `a` | 新增规则 |
| `e` | 编辑选中规则 |
| `d` | 删除选中规则（y 确认 / n 取消） |
| `t` | 测当前规则连通性 |
| `T` | 并发测试全部规则 |
| `E` | 导出到 `%APPDATA%\ppm\backup-YYYYMMDD-HHMMSS.json` |
| `I` | 输入备份 JSON 文件路径进行导入（合并去重，已存在的监听键跳过） |
| `r` | 手动刷新列表与监听状态 || `j/k` 或方向键 | 移动光标（支持 pgup/pgdn/home/end） |
| `enter` | 编辑选中规则（同 `e`） |
| `q` | 退出（y 确认 / n 取消） |

### CLI 用法

ppm 同时支持 TUI 和 CLI 模式。无参数运行 `ppm` 进入 TUI；传入子命令则以 CLI 模式运行。

**子命令一览**

| 命令 | 别名 | 说明 |
|---|---|---|
| `ppm list` | `ls` | 列出所有规则 |
| `ppm add` | - | 新增规则 |
| `ppm edit` | - | 编辑规则（删除旧规则 + 创建新规则） |
| `ppm delete` | `del` | 删除一条或多条规则 |
| `ppm test` | - | 测试规则连通性 |
| `ppm export` | - | 导出全部规则和备注为 JSON 备份 |
| `ppm import` | - | 从备份文件导入规则 |
| `ppm tui` | - | 显式启动 TUI |
| `ppm version` | `ver` | 打印版本号 |

**Flag**

| 长写 | 短写 | 适用命令 |
|---|---|---|
| `--listen` | `-l` | `add`、`edit` |
| `--connect` | `-c` | `add`、`edit` |
| `--note` | `-n` | `add`、`edit` |
| `--json` | `-j` | `list`、`test` |

**常用示例**

```bash
ppm ls -j                              # JSON 格式列出所有规则

ppm add :8080 10.0.0.1:80              # 等同于 0.0.0.0:8080 → 10.0.0.1:80
ppm add :3000 10.0.0.1:3000 web        # 带备注
ppm add -l :8080 -c 10.0.0.1:80 -n web # 同上，使用 flag 形式

ppm edit :8080 -c 10.0.0.2:80          # 修改转发目标（监听地址不变）
ppm edit :8080 :9090 10.0.0.2:80       # 同时修改监听地址和转发目标

ppm del :8080                          # 删除单条规则
ppm del :8080 :9090 :3000              # 批量删除多条规则

ppm test :8080                         # 测试单条规则连通性
ppm test -a                            # 测试所有规则

ppm export -o backup.json              # 导出到指定文件
ppm import backup.json                 # 从备份文件导入（去重）
```

## 数据位置

- 备注与备份文件：`%APPDATA%\ppm\`
- 备注（`notes.json`）按「监听地址：端口」键值关联规则，随导出一起写入备份

## 注意事项

- 仅支持 v4tov4 模式（IPv4 → IPv4），覆盖绝大多数场景
- portproxy 底层依赖系统服务 `iphlpsvc`，若转发不生效检查该服务是否运行
- 监听 `0.0.0.0` 会占用所有网卡的该端口；被占用的端口会显示 `!off` 无法正常工作

## License

[MIT](./LICENSE)
