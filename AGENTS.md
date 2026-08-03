# ETF 溢价率监控 - 项目指南

## 概述
A股美股ETF溢价率实时监控面板。跟踪沪深两市上市的纳斯达克100和标普500ETF，实时显示溢价率。

- **前端**：Vanilla JS + Chart.js (CDN) + 纯 CSS
- **后端**：**Go**（重写中，`go/` 目录，当前为运行版本）；旧 Python/FastAPI 版保留在 `backend/`
- **数据源**：腾讯财经 API (`qt.gtimg.cn`)，通过 HTTP GET 获取（GBK 编码）
- **持久化**：SQLite（每日快照）+ JSON（日内历史）
- **端口**：8000

## 目录结构
```
etf-premium-tracker/
├── start.sh / start.bat         # Go 版启动脚本（go run ./cmd/server）
├── AGENTS.md                    # 本文件
├── .gitignore                   # __pycache__/、backend/data/、go 构建产物
├── backend/                     # 旧 Python 版（保留，不再维护）
│   ├── main.py                  # FastAPI 服务（历史参考实现）
│   ├── etf_fees.json            # ETF 费率（Go 版也读取此文件）
│   ├── watchlist.txt            # 置顶的ETF代码（每行一个，Go 版读写此文件）
│   └── data/                    # 运行数据（已 gitignore）
│       ├── premium.db           # SQLite 表：daily_premium(code, date, premium, price, iopv)
│       └── history.json         # 日内历史：{code: [[unix_ts, premium], ...]}
├── frontend/                    # 前端（SPA，Go 版静态服务此目录）
│   ├── index.html / style.css / bloomberg.css / script.js
└── go/                          # Go 重写版（当前运行版本）
    ├── go.mod / go.sum          # module etf-premium-tracker, go 1.26.5
    ├── cmd/server/main.go       # 入口：config → 初始化 → 轮询 + HTTP
    └── internal/
        ├── config/              # flag 解析（-addr/-poll/-data-dir/-watchlist/-fees/-frontend）
        ├── etfs/                # 17 只 ETF 静态元数据表（与 backend/main.py 一致）
        ├── model/               # DTO（与前端契约严格对齐，含 [ts,premium]/[date,premium] 自定义 JSON）
        ├── quote/               # 腾讯行情客户端：BuildURL / Fetch(GBK) / Parse(字段解析)
        ├── market/              # 交易时间判断 IsTrading / IsAfterClose
        ├── history/             # 日内历史：内存 + history.json + 480 条截断
        ├── store/               # SQLite 仓储（modernc.org/sqlite，纯 Go 无 cgo）
        ├── watchlist/           # watchlist.txt 读/写/toggle（原子替换）
        ├── fees/                # etf_fees.json 加载
        └── server/              # 缓存、30m 后台轮询、8 个 HTTP handler、CORS、静态文件
```

## 启动
```bash
./start.sh        # 或 Windows 下 start.bat；等价于 cd go && go run ./cmd/server
# 访问 http://0.0.0.0:8000
```
常用 flag：`-poll 30m`（轮询间隔）、`-addr :8001`（换端口）、`-frontend ../frontend`（静态目录）。

## 交易时间（A股）
- **上午**：09:30-11:30，**下午**：13:00-15:00，**周末**：休市
- 判断逻辑在 `go/internal/market/market.go`（总分钟 [570,690) ∪ [780,900)）

## Go 后端架构（`go/internal/server`）

### 数据流
1. `server.Start()` 在启动时运行 goroutine，每 **30 分钟**（`-poll` 可调）轮询一次
2. `RefreshOnce()` → `quote.Fetch`（HTTP + GBK 解码）→ `quote.Parse`（字段解析 + 换算）
3. 每次刷新：追加日内历史到 `history.Store` → 保存 `history.json` → 判断 `market.IsTrading` → 收盘后落盘每日快照 → 更新内存缓存 `resp`
4. 前端请求直接读内存缓存（`mu.RWMutex` 保护），不查 DB

### API 端点（与 Python 版契约完全一致）
| 路径 | 返回内容 | 是否缓存 |
|---|---|---|
| `GET /api/etfs` | `{ nasdaq, sp500, market_status, update_time, total_count }` | 是（30m后台更新） |
| `GET /api/watchlist` | `{ codes: [...] }` | 读文件 |
| `POST /api/watchlist/toggle/{code}` | `{ codes, in_watchlist }` | 原子写文件 |
| `POST /api/refresh` | 同 `/api/etfs`；失败 502 `{"detail":"数据获取失败，上游不可达"}` | 立即抓取 |
| `GET /api/fees` | etf_fees.json 原样 | 启动时加载 |
| `GET /api/history/{code}` | `{ code, history: [[时间戳, 溢价], ...] }` | 内存（兼容端点，前端未消费） |
| `GET /api/daily/{code}` | `{ code, daily: [[日期, 溢价], ...] }` | 读 SQLite |
| `GET /{path:path}` | 静态文件，回退到 `index.html` | - |

### 溢价率计算
```go
// go/internal/quote/quote.go：直接取腾讯字段 [77] 作为溢价率
// IOPV（字段78）== 0 时 premium 为 null
// 换算：volume = 手×100，amount = 万×10000，fund_scale = NAV×总份额/1e8
```

### 关键约定
- ETF 静态元数据在 `go/internal/etfs/etfs.go`（Python 版另有一份在 `backend/main.py`，改时必须同步）
- `watchlist.txt` — 每行一个代码，置顶显示并带星标
- 日内历史每只 ETF 最多保留 **480 条**（约 10 天，30 分钟间隔）
- 每日快照**仅在收盘后（15:00 之后）保存一次**（`lastDailySave` 记录日期防重复；需至少一只 ETF 有 IOPV），比 Python 版更严格（Python 在开盘前也会保存）
- `/api/etfs` 不查数据库，纯内存缓存
- 后台轮询与手动 `/api/refresh` 共用 `refreshMu` 互斥

## 前端 (`frontend/script.js`)

### 刷新架构
- 数据仅通过手动刷新按钮触发 `POST /api/refresh`；后端 30m 后台轮询保证数据新鲜，前端无需定时器
- `fetchData()` 拉取 `/api/etfs` 渲染网格；`refreshData()` 手动刷新后同样渲染

### 全局状态
| 变量 | 用途 |
|---|---|
| `allData` | 最新缓存响应 |
| `watchlist` | 置顶代码集合 |
| `chartInstance` | Chart.js 实例 |
| `theme` / `maVisibility` | 主题、均线开关 |

### 图表
- Chart.js 4.4.4（CDN），折线图 + 渐变填充
- 数据来自 `/api/daily/{code}`（`HISTORY_API = '/api/daily'`）
- 点击列表行打开弹窗式图表，点击外部或 Escape 关闭

### 排序
- 每板块下拉框（溢价率/费率/成交额/规模 升降序、代码、名称）
- 置顶项始终排在前面

## Git 工作流
- **无构建步骤** — Go 版 `go run ./cmd/server` 直接运行；前端纯 HTML/CSS/JS
- `backend/data/`、`go/server.exe` 已 gitignore（运行时数据/构建产物）
- 使用中式简洁提交信息
- 修改 Go 代码后：`cd go && go test ./...`，重启服务 `./start.sh`

## 常见操作

### 新增 ETF
在 `go/internal/etfs/etfs.go` 和 `backend/main.py` 的静态列表各加一条，`category` 使用 `"nasdaq"` 或 `"sp500"`；费率加到 `backend/etf_fees.json`。

### 置顶 ETF（关注）
将代码加入 `backend/watchlist.txt`，每行一个；或点击页面星标自动写入。

### 清除历史数据
删除 `backend/data/history.json` 和/或 `backend/data/premium.db`。

### Go 版测试
```bash
cd go && go test ./...
# 端到端：./start.sh 后 curl http://localhost:8000/api/etfs
```
