# ETF 溢价率监控

A股美股 ETF 溢价率实时监控面板。跟踪沪深两市上市的纳斯达克 100 与标普 500 ETF，实时展示溢价率、费率、成交额、基金规模，并提供每日溢价率历史图表。

纯 Go 实现：单二进制、零前端构建、无 cgo 依赖（SQLite 为纯 Go 驱动）。

## 功能特性

- 📊 实时行情：后台 **30 分钟**轮询腾讯财经接口，内存缓存直接响应前端
- 🏷️ 溢价率分级：正常 / 溢价 / 高溢价 / 极高（深度折价）四级配色
- ⭐ 置顶关注：点击星标切换关注，置顶项始终排在板块最前
- 📈 历史图表：Chart.js 每日溢价率折线 + MA5/MA10/MA20 均线，支持 Bloomberg 深色主题
- 💰 费率与规模：展示综合费率、成交额（亿/万）、基金规模（亿）
- 📁 双持久化：日内历史存 JSON，每日快照存 SQLite（收盘后自动落盘）

## 快速开始

### 前置要求

- Go 1.26+（`go.mod` 声明 `go 1.26.5`）

### 启动

```bash
# 方式一：启动脚本（推荐，路径自动指向仓库根）
./start.sh            # Windows 下用 start.bat 或 start.ps1

# 方式二：直接运行（需在 go/ 目录内，路径相对 CWD）
cd go && go run ./cmd/server

# 方式三：编译后从任意目录运行（配合 flag 指定路径）
cd go && go build -o server.exe ./cmd/server
```

启动后访问 **http://localhost:8000**

### 命令行参数

| Flag | 默认值 | 说明 |
|---|---|---|
| `-addr` | `:8000` | HTTP 监听地址 |
| `-poll` | `30m` | 后台轮询上游数据的间隔 |
| `-timeout` | `10s` | 单次上游抓取超时 |
| `-data-dir` | `backend/data` | 运行数据目录（history.json / premium.db） |
| `-watchlist` | `backend/watchlist.txt` | 置顶 ETF 代码文件 |
| `-fees` | `backend/etf_fees.json` | ETF 费率 JSON 文件 |
| `-frontend` | `frontend` | 前端静态文件目录 |
| `-pages-daily` | `pages/data/daily.json` | 在线版静态日线数据输出路径（空字符串 = 不导出） |

示例：`go run ./cmd/server -addr :9000 -poll 1m`

### 开发：热重载（Air）

```bash
cd go && air   # 监听 .go 文件变更，自动构建并重启服务
```

- 需先安装 Air：`go install github.com/air-verse/air@latest`
- 配置在 `go/.air.toml`：构建产物输出到 `go/tmp/`（已 gitignore），参数与 start.sh 对齐
- 前端是纯静态文件，改 HTML/CSS/JS 无需重启后端，直接刷新浏览器

## API 端点

| 路径 | 方法 | 说明 |
|---|---|---|
| `/api/etfs` | GET | 全部 ETF 行情（按板块分组 + 市场状态） |
| `/api/refresh` | POST | 手动触发一次抓取，失败返回 502 |
| `/api/watchlist` | GET | 置顶代码列表 |
| `/api/watchlist/toggle/{code}` | POST | 切换置顶状态 |
| `/api/fees` | GET | 费率数据（etf_fees.json 原样） |
| `/api/history/{code}` | GET | 日内溢价率历史 `[[ts, premium], ...]` |
| `/api/daily/{code}` | GET | 每日溢价率历史 `[[date, premium], ...]` |
| `/{path}` | GET | 静态文件，缺失回退 `index.html` |

### `/api/etfs` 响应示例

```json
{
  "nasdaq": [
    {
      "code": "513100",
      "name": "纳指ETF国泰",
      "category": "nasdaq",
      "manager": "国泰基金",
      "exchange": "SH",
      "price": 2.112,
      "iopv": 1.914,
      "nav": 1.9024,
      "premium": 10.34,
      "change_pct": 3.48,
      "volume": 168780400,
      "amount": 356440000.0,
      "prev_close": 2.041,
      "fee": { "mgmt": 0.6, "custodian": 0.2, "total": 0.8 },
      "fund_scale": 12.3
    }
  ],
  "sp500": [],
  "market_status": "open",
  "update_time": "2026-08-03 09:35:34",
  "total_count": 17
}
```

> 缺失字段以 `null` 表示（如 IOPV 为 0 时 `premium` 为 `null`）。

## 数据来源与字段

- **上游**：腾讯财经接口 `http://qt.gtimg.cn/q=sh513100,sz159941,...`（GBK 编码，`~` 分隔）
- **溢价率**：直接取腾讯字段 [77]；IOPV（字段 78）为 0 时置 `null`
- **换算**：成交量 = 手 × 100；成交额 = 万元 × 10000；基金规模 = NAV × 总份额 / 1e8
- **交易时段**：工作日 09:30-11:30 / 13:00-15:00（`internal/market` 判断）
- **每日快照**：收盘后（15:00 之后）每个交易日保存一次至 SQLite；日内历史每只 ETF 保留最近 480 条（约 10 天，30 分钟间隔）

## 项目结构

```
etf-premium-tracker/
├── start.sh / start.bat / start.ps1   # 启动脚本
├── go/                       # Go 服务（本仓库主体）
│   ├── cmd/server/main.go    # 入口：配置 → 初始化 → 轮询 + HTTP
│   └── internal/
│       ├── config/           # 命令行配置解析
│       ├── etfs/             # 17 只 ETF 静态元数据表
│       ├── model/            # DTO（与前端契约严格对齐）
│       ├── quote/            # 腾讯行情客户端：URL 构建 / GBK 解码 / 字段解析
│       ├── market/           # 交易时间判断
│       ├── history/          # 日内历史（内存 + JSON，480 条截断）
│       ├── store/            # SQLite 仓储（modernc.org/sqlite，纯 Go）
│       ├── watchlist/        # 置顶文件读写（原子替换）
│       ├── fees/             # 费率加载
│       └── server/           # 缓存、后台轮询、HTTP 路由、CORS、静态文件
├── frontend/                 # 前端 SPA（原生 JS + Chart.js CDN，无构建）
├── backend/                  # 数据文件：watchlist.txt / etf_fees.json / data/
│                             #   （旧 Python 版源码已删除，此目录只放运行数据与配置）
├── pages/                    # GitHub Pages 在线版（纯静态、无后端，浏览器直连腾讯接口）
│   └── data/daily.json       # 由后端数据库导出的每日溢价率历史（提交进仓库，随站点发布）
└── plan.md                   # Go 重写计划与决策记录
```

### 在线版与本地版的差异

`pages/` 是部署到 GitHub Pages 的纯静态版本，没有后端，因此**展示层一致、数据来源不同**：

- 图表：两端画的是同一份每日**溢价率**。本地版直接读 SQLite；在线版读随仓库发布的 `pages/data/daily.json`（由 SQLite 导出），再叠加本浏览器 localStorage 里更新的点。只有两者都没有数据时才回落腾讯 K 线的**收盘价**，此时纵轴单位会变成"元"并在图注中说明。
- 置顶：本地版写服务端 `backend/watchlist.txt`（多设备共享）；在线版存 localStorage（每浏览器独立）。

改动列表列、排序项、配色等展示逻辑时，`frontend/` 与 `pages/` 两边都要改。

### 更新在线版的历史数据

```bash
cd go && go run ./cmd/export      # backend/data/premium.db → pages/data/daily.json
```

导出文件只包含当前跟踪的 ETF，格式与 `/api/daily/{code}` 一致。提交并推送后，GitHub Actions 会自动部署到 Pages（工作流监听 `master` 分支的 `pages/**` 改动）。

服务器也内置了同样的导出：启动时、以及每天收盘落盘快照后都会重写该文件（用 `-pages-daily ""` 可关闭），因此本地跑着服务的话，只要定期提交推送即可。

### 无人值守更新（GitHub Actions）

仓库里的 `.github/workflows/update-data.yml` 每交易日收盘后（北京时间 15:10 / 15:40 / 16:10）在 GitHub 的 runner 上抓一次行情，把当日溢价率写进 `pages/data/daily.json` 并提交，然后显式触发 Pages 部署——**不需要本地开机**。

```bash
# 手动验证上游连通性（只抓取不写入）
gh workflow run "Update Daily Data" -f dry_run=true
```

命令本身也能本地运行，用于手动补数据：

```bash
cd go && go run ./cmd/snapshot -dry-run   # 只抓取并打印
cd go && go run ./cmd/snapshot -force     # 未到收盘也写入（写入的是盘中值）
```

## 开发

```bash
cd go

# 运行全部测试（数据层 + HTTP handler，含真实响应 fixture 与快照时机用例）
go test ./...

# 静态检查与格式化
go vet ./...
gofmt -l .
```

### 新增 ETF

1. `go/internal/etfs/etfs.go` 的 `All` 列表加一条（code/name/category/manager/exchange）
2. `backend/etf_fees.json` 补充对应费率
3. 重启服务（列表按代码顺序展示，前端自动渲染）

### 清除历史数据

删除 `backend/data/history.json` 与 `backend/data/premium.db`，重启后自动重建。

## 数据流

```
腾讯财经 API ──(30 分钟轮询 / 手动刷新)──▶ quote 抓取+解析
        │
        ├─▶ history.Store（内存 + history.json）
        ├─▶ 收盘后 ▶ SQLite daily_premium（每日快照）
        └─▶ 内存缓存 resp（RWMutex 保护）
                    │
                    └─▶ /api/etfs 等 8 个端点 ──▶ 前端渲染
```

- 前端请求**不查数据库**，全部走内存缓存
- 后台轮询与手动刷新共用互斥锁，避免并发抓取写坏缓存
- 优雅退出：SIGINT/SIGTERM 触发 HTTP 关停，保存数据后退出

## 技术栈

- **后端**：Go 1.26、标准库 `net/http`（Go 1.22+ 路由模式）、`modernc.org/sqlite`（纯 Go SQLite）、`golang.org/x/text`（GBK 解码）
- **前端**：原生 JavaScript + Chart.js 4.4.4（CDN）+ 纯 CSS（含 Bloomberg 深色主题）
- **零第三方服务依赖**：无 Redis、无消息队列，单进程即可运行
