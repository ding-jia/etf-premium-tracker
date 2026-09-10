# ETF 溢价率监控 - 项目指南

## 概述
A股美股ETF溢价率实时监控面板。跟踪沪深两市上市的纳斯达克100和标普500ETF，实时显示溢价率。

- **前端**：Vanilla JS + Chart.js (CDN) + 纯 CSS
- **后端**：**Go**（`go/` 目录，当前为运行版本）；旧 Python/FastAPI 版源码已于 2026-09 删除，`backend/` 现在只放运行数据与配置
- **数据源**：腾讯财经 API (`qt.gtimg.cn`)，通过 HTTP GET 获取（GBK 编码）
- **持久化**：SQLite（每日快照）+ JSON（日内历史）
- **端口**：8000

## 目录结构
```
etf-premium-tracker/
├── start.sh / start.bat / start.ps1  # 启动脚本（优先运行 go/server.exe，缺失时才 go build）
├── AGENTS.md                    # 本文件
├── .gitignore                   # __pycache__/、backend/data/、go 构建产物
├── backend/                     # 运行数据与配置（无源码）
│   ├── etf_fees.json            # ETF 费率（Go 版也读取此文件）
│   ├── watchlist.txt            # 置顶的ETF代码（每行一个，Go 版读写此文件）
│   └── data/                    # 运行数据（已 gitignore）
│       ├── premium.db           # SQLite 表：daily_premium(code, date, premium, price, iopv)
│       └── history.json         # 日内历史：{code: [[unix_ts, premium], ...]}
├── frontend/                    # 前端（SPA，Go 版静态服务此目录）
│   ├── index.html / style.css / bloomberg.css / script.js
├── pages/                       # GitHub Pages 在线版（纯静态、无后端，浏览器直连腾讯接口）
│   ├── index.html               # 内联样式；列与排序项必须与 frontend/ 保持一致
│   ├── script.js                # 含第三份 ETF 元数据 META/FEES（改元数据时必须同步）
│   └── data/daily.json          # 后端导出的每日溢价率历史（提交进仓库，在线版图表的数据源）
└── go/                          # Go 重写版（当前运行版本）
    ├── go.mod / go.sum          # module etf-premium-tracker, go 1.26.5
    ├── cmd/server/main.go       # 入口：config → 初始化 → 轮询 + HTTP
    └── internal/
        ├── config/              # flag 解析（-addr/-poll/-data-dir/-watchlist/-fees/-frontend）
        ├── etfs/                # 17 只 ETF 静态元数据表（与 pages/script.js 的 META 一致）
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
./start.sh        # 或 Windows 下 start.bat / start.ps1（等价于 cd go && go build -o server.exe ./cmd/server 后运行）
# 访问 http://0.0.0.0:8000
```
PowerShell 可用 `\start.ps1 [-Addr :9000] [-Poll 5m]`（参数可选，默认 :8000 / 30m）。
路径默认值（`-data-dir`/`-watchlist`/`-fees`/`-frontend`）按**仓库根**解析（从 CWD 向上探测含 `go/go.mod` 的目录），因此从 `go/` 目录直接 `go run ./cmd/server` 与 start.sh 行为一致，数据都落在 `backend/data/`。显式传入的相对路径按 CWD 解析。

常用 flag：`-poll 30m`（轮询间隔）、`-addr :8001`（换端口）、`-frontend ../frontend`（静态目录）。

## 开发（热重载）
```bash
cd go && air   # 监听 go/ 下 .go 文件变更，自动构建并重启服务（配置：go/.air.toml）
```
- Air 版本 v1.67+（`go install github.com/air-verse/air@latest`）
- 构建产物在 `go/tmp/`（已 gitignore），不污染 `go/server.exe`；退出时自动清理
- 前端静态文件无需重启后端：只监听 `.go` 扩展名，改 HTML/CSS/JS 直接刷新浏览器
- 参数与 start.sh 对齐（端口 :8000，路径指回仓库根）；改端口编辑 `go/.air.toml` 的 `-addr`

## 交易时间（A股）
- **上午**：09:30-11:30，**下午**：13:00-15:00，**周末**：休市
- 判断逻辑在 `go/internal/market/market.go`（总分钟 [570,690) ∪ [780,900)）
- 全部时间判断基于 `market.ShanghaiTZ`（UTC+8 固定时区），不依赖系统时区
- **已知局限**：未维护法定节假日/调休日历，节假日收盘后可能把节前数据写入当日快照

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
- ETF 静态元数据共 **3 份**，改时必须同步：`go/internal/etfs/etfs.go`、`pages/script.js` 的 `META`、`backend/etf_fees.json`（费率）
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
| `lastChartRecords` | 最近一次图表数据（均线切换时重渲染） |

### 图表
- Chart.js 4.4.4（CDN），折线图 + 渐变填充
- 数据来自 `/api/daily/{code}`（`HISTORY_API = '/api/daily'`）
- 点击列表行在右侧图表面板展示历史走势（默认选中 513500）；溢价率为 null 的点不渲染，均线跳过 null 点
- 支持切换 MA5/MA10/MA20 均线

### 排序
- 每板块下拉框（溢价率/费率/成交额/规模 升降序、代码、名称）
- 置顶项始终排在前面

## 在线版（`pages/`，GitHub Pages）

无后端，浏览器直连腾讯接口（`<script>` 注入 `qt.gtimg.cn`），由 `.github/workflows/deploy-pages.yml` 部署。展示层（列、排序项、配色、格式化）**必须与 `frontend/` 保持一致**，只在数据来源上有意分叉：

| 项目 | 本地版 | 在线版 |
|---|---|---|
| 行情 | 后端内存缓存（30 分钟轮询） | 浏览器直连腾讯 |
| 图表数据 | `/api/daily`（SQLite 每日**溢价率**） | `pages/data/daily.json`（由 SQLite 导出、随仓库发布）+ localStorage 里更新的点；两者都为空时才回落腾讯 K 线的**收盘价**（纵轴单位变"元"并在图注说明） |
| 置顶 | `backend/watchlist.txt`（服务端共享） | localStorage（每浏览器独立） |
| 费率 | `/api/fees` | `pages/script.js` 内置 `FEES` |

改展示层时两边都要改；两者的列宽模型（`.lh-*`/`.li-*` 的 `flex` 基准）也必须一致。

### 更新在线版历史数据

```bash
cd go && go run ./cmd/export     # backend/data/premium.db → pages/data/daily.json
```

服务器在启动时与每天收盘落盘快照后会各自动重写一次该文件（`-pages-daily ""` 可关闭；数据库为空时跳过写入，避免清空已提交的数据）。提交推送后由 GitHub Actions 自动部署。

### 无人值守更新（GitHub Actions）

`.github/workflows/update-data.yml` 在每交易日收盘后（北京时间 15:10 / 15:40 / 16:10）于 runner 上运行 `cmd/snapshot`：直接抓腾讯行情 → 用 `internal/dailyfile` 把当日溢价率并入 `pages/data/daily.json` → 提交 → **显式触发 Deploy Pages**。

两个容易踩的点：

- 用 `GITHUB_TOKEN` 推的提交**不会**触发其它 workflow，所以必须显式 `gh workflow run "Deploy Pages"`（因此该 workflow 需要 `actions: write` 权限）
- `internal/dailyfile` 与 `internal/export` 分开是为了让 `cmd/snapshot` 不依赖 SQLite（纯 Go SQLite 编译很重，会拖慢 CI）

## Git 工作流
- **无构建步骤** — Go 版 `go run ./cmd/server` 直接运行；前端纯 HTML/CSS/JS
- `backend/data/`、`go/backend/`、`go/server.exe` 已 gitignore（运行时数据/残留数据目录/构建产物）
- 使用中式简洁提交信息
- 修改 Go 代码后：`cd go && go test ./...`，重启服务 `./start.sh`
- 代码审查记录见 `review.md`（2026-08-05）

## 常见操作

### 新增 ETF
三处都要改，否则本地版与在线版会不一致：

1. `go/internal/etfs/etfs.go` 的 `All` 列表（code/name/category/manager/exchange）
2. `pages/script.js` 的 `META`（同上）与 `FEES`（综合费率数字）
3. `backend/etf_fees.json`（mgmt_fee/custodian_fee/total_fee）

`category` 使用 `"nasdaq"` 或 `"sp500"`。

### 置顶 ETF（关注）
将代码加入 `backend/watchlist.txt`，每行一个；或点击页面星标自动写入。

### 清除历史数据
删除 `backend/data/history.json` 和/或 `backend/data/premium.db`。

### Go 版测试
```bash
cd go && go test ./...
# 端到端：./start.sh 后 curl http://localhost:8000/api/etfs
```
