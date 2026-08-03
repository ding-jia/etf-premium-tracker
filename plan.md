# ETF 溢价率监控 — Go 重写计划

> 状态：审阅完成，计划定稿。目标：用 Go 完整复刻 Python 版（backend/main.py + frontend）的全部行为，并修复 Python 版已发现的问题。

---

## 一、现状审阅结论

### 1.1 Python 版实际行为 vs AGENTS.md 文档（重要差异）

| 项目 | AGENTS.md 声称 | 实际代码 |
|---|---|---|
| 后台轮询 | `background_updater()` 每 **30s** 轮询 | **不存在**！`lifespan` 只在启动时 `update_cache()` 一次 |
| 前端自动刷新 | 5 分钟 `setInterval` + 60s 状态检测 | **不存在**！script.js 没有 setInterval，只有手动刷新按钮 |
| 实际数据流 | 后台持续更新缓存 | **启动拉取一次 + 用户点刷新按钮（`POST /api/refresh`）** |

Go 重写按文档意图实现后台轮询（D1），并保留手动刷新端点。

### 1.2 Python 版发现的 bug / 风险点

1. **`toggle_watchlist` 会崩**：`import fcntl` 被注释掉，但函数体仍调用 `fcntl.flock`。Windows 上直接 `NameError`，只有 Linux 可跑。
2. **每日快照保存时机有误**：`if not is_trading and not is_weekend` 在**开盘前**（如 08:00 启动）也会把"盘前数据"写入当日快照；且每次手动刷新都会重复写（靠 INSERT OR REPLACE 幂等兜底，属浪费）。Go 改为**收盘后（15:00 之后）且每交易日只保存一次**。
3. **无后台轮询**：文档承诺的数据新鲜度实际不存在，数据只能靠用户手点。
4. `.gitignore` 未包含 `backend/data/`（AGENTS.md 声称已 ignore，实际只有 `__pycache__/`、`*.pyc`、`.reasonix/`）。

### 1.3 Go 版现状（已完成部分）

```
go/
├── go.mod / go.sum          module etf-premium-tracker, go 1.26.5（本机 go1.26.5）
├── internal/config/         flag 解析，路径相对 CWD，可覆盖 ✓
├── internal/etfs/           17 只静态元数据（13 nasdaq + 4 sp500），与 Python 一致 ✓
└── internal/model/          ETF/Fee DTO + HistoryPoint[ts,premium] 自定义 JSON ✓ 有单测 ✓
```

- `go vet` / `go test ./...` 通过。
- ETF 静态表 17 只与 `main.py` 逐条一致（代码、名称、类别、管理人、交易所）。
- `model.ETF` 的 json tag（`price/iopv/nav/premium/change_pct/volume/amount/prev_close/fund_scale` 用 `*float64` 表达 null）与前端契约完全对齐。
- `HistoryPoint` 的 `[ts, premium]` 二元组格式与 `history.json` 存量格式一致，且有 round-trip 测试。

**缺失（本计划主体）**：cmd 入口、腾讯行情客户端（GBK 解码）、行情解析、交易时间判断、后台轮询、history store、SQLite 仓储、watchlist 服务、fees 加载、HTTP 路由、CORS、静态文件服务、`/api/refresh`、`/api/daily`、全部响应包装类型。

---

## 二、关键决策点（D1-D9）

| # | 决策 | 决定 | 理由 |
|---|---|---|---|
| D1 | 后台轮询间隔 | **默认 30s**（对齐文档意图），flag 可调 | 腾讯接口轻量；文档契约 30s；goroutine + ticker 成本极低，比 Python 现状体验好。`config.go` 现默认 5min，改为 30s |
| D2 | 每日快照时机 | **改进**：交易日 15:00 后首次轮询时保存一次（记录 `lastDailySaveDate`） | 修复 Python 开盘前写脏快照的问题 |
| D3 | 文件锁 | **不需要** | Go 单进程，`watchlist.txt` 用"临时文件 + rename"原子替换，避免 fcntl 跨平台问题 |
| D4 | GBK 解码 | `golang.org/x/text/encoding/simplifiedchinese`（已在 go.sum，从 indirect 提为 direct） | 纯 Go、无 cgo |
| D5 | HTTP 框架 | **标准库 `net/http`**（Go 1.22+ ServeMux 支持 `"GET /api/etfs"`、`"POST /api/watchlist/toggle/{code}"` 模式） | 只有 8 个端点，零依赖，保持轻量 |
| D6 | SQLite | `modernc.org/sqlite`（已在 go.mod，纯 Go 无 cgo，Windows 友好），用标准 `database/sql` | 已是现成依赖 |
| D7 | `/api/fees`、`/api/history` | **保留**（兼容），标注"前端未消费" | 成本低；前端实际只调 `/api/etfs`、`/api/refresh`、`/api/watchlist`、`/api/watchlist/toggle/{code}`、`/api/daily/{code}` |
| D8 | 前端目录 | 增加 `-frontend` flag（默认 `frontend` 相对 CWD） | 现 config 只覆盖数据/文件路径；静态目录同样可配，否则 exe 双击运行时 CWD=exe 目录会找不到 |
| D9 | 并发控制 | 后台轮询与 `/api/refresh` 共用同一把锁 | 防止两个 fetch 并发写缓存 |

---

## 三、目标目录结构

```
go/
├── go.mod / go.sum
├── cmd/
│   └── server/
│       └── main.go                  # 入口：config.Parse → 组装 → 启动轮询 + HTTP
└── internal/
    ├── config/config.go             # 已有；改动：PollInterval 默认 30s、加 FrontendDir
    ├── etfs/etfs.go                 # 已有，不动
    ├── model/model.go               # 已有；新增响应包装类型（见 §五）
    ├── model/model_test.go          # 已有
    ├── quote/                       # 新增：腾讯行情客户端
    │   ├── quote.go                 #   BuildURL / Fetch / Parse / GBK 解码
    │   └── quote_test.go            #   用真实响应样本测解析
    ├── market/                      # 新增：交易时间
    │   ├── market.go                #   IsTrading(now) / IsAfterClose(now)
    │   └── market_test.go           #   边界用例
    ├── history/                     # 新增：日内历史
    │   ├── history.go               #   Store：内存 map + Load/Save JSON + 480 截断
    │   └── history_test.go
    ├── store/                       # 新增：SQLite 仓储
    │   ├── store.go                 #   Open/Init/UpsertDaily/QueryDaily
    │   └── store_test.go            #   t.TempDir 隔离
    ├── watchlist/
    │   └── watchlist.go             # 新增：读/写/toggle（原子替换）
    ├── fees/
    │   └── fees.go                  # 新增：加载 etf_fees.json
    └── server/
        └── server.go                # 新增：缓存、轮询 goroutine、8 个 handler、CORS、静态文件
```

---

## 四、分阶段实现计划

### 阶段 1：数据层（纯逻辑，先写测试）

**1.1 `internal/quote` — 腾讯行情客户端**
- `BuildURL(etfs []model.ETF) string`：`http://qt.gtimg.cn/q=sh513100,sz159941,...`（`SH`→`sh`，否则 `sz`）
- `Parse(raw string, meta []model.ETF, fees map[string]fees.Info) []model.ETF`：按 `;` 分行 → `~` 切分 → 需 ≥82 字段 → 取字段：
  - `[1]`名称 `[2]`代码 `[3]`现价 `[4]`昨收 `[6]`成交量(手) `[32]`涨跌幅% `[37]`成交额(万) `[72]`总份额 `[77]`溢价率 `[78]`IOPV `[81]`NAV
- 换算规则（与 Python 逐条一致）：
  - `premium = 字段77`；**IOPV==0 时 premium=None**（Python 注释里的"兜底算法"实际没实现，Go 不实现，保持一致）
  - `volume = 手 × 100`，`amount = 万 × 10000`
  - `fund_scale = NAV × total_shares / 1e8`（round 2）；`iopv/nav` round 4
  - `fee = {mgmt, custodian, total}` 从 fees 映射（缺表为 null）
- `Fetch(ctx, cfg) ([]byte, error)`：GET + 10s 超时（对齐 Python）+ GBK→UTF-8 解码（`simplifiedchinese.GBK.NewDecoder()`，无效字节 replace）
- 测试：把一条真实腾讯响应样本（含中文名、含 IOPV=0 的变体）固化成 fixture，断言解析结果与 Python 语义一致

**1.2 `internal/market` — 交易时间**
- `IsTrading(t time.Time) bool`：周末 → false；`总分钟∈[570,690)` 或 `[780,900)` → true（09:30-11:30 / 13:00-15:00）
- `IsAfterClose(t)`：非周末且总分钟 ≥ 900（供快照判断）
- 测试：09:29/09:30/11:29/11:30/12:59/13:00/14:59/15:00/周末各边界

**1.3 `internal/history` — 日内历史**
- 内存 `map[string][]model.HistoryPoint` + `Load(path)` / `Save(path)`（读写 `history.json`，格式 `{"code": [[ts, premium], ...]}`，复用已有 `HistoryPoint` 自定义 JSON）
- `Append(code, ts, premium)`：追加 + 截断尾部 480 条
- 测试：round-trip 读写、480 截断

**1.4 `internal/store` — SQLite 仓储**
- `Open(path)` / `Init()`：`CREATE TABLE IF NOT EXISTS daily_premium (code TEXT, date TEXT, premium REAL, price REAL, iopv REAL, PRIMARY KEY (code, date))`
- `UpsertDaily(rows)`：INSERT OR REPLACE（对齐 Python）
- `QueryDaily(code) ([]model.DailyPoint, error)`：`SELECT date, premium FROM ... ORDER BY date ASC`，premium 用 `sql.NullFloat64` 保留 null
- 测试：`t.TempDir()` 建库 → 写入 → 查询 → 重复 upsert 幂等

**1.5 `internal/watchlist` + `internal/fees`**
- `Read(path) []string` / `Toggle(path, code) ([]string, bool)`：读 → 在/不在 → 原子写（`tempfile + os.Rename`）
- `fees.Load(path) (map[string]Info, error)`：解析 `{"513100": {"mgmt_fee":0.6,...}}`，`Info{mgmt_fee, custodian_fee, total_fee}`（字段名带 `_fee`，与 `/api/etfs` 里的 `{mgmt,custodian,total}` 是**两个不同形状**，注意区分）

### 阶段 2：模型补充（`internal/model`）

新增响应包装类型（对齐前端契约）：

```go
type EtfsResponse struct {
    Nasdaq       []ETF  `json:"nasdaq"`
    Sp500        []ETF  `json:"sp500"`
    MarketStatus string `json:"market_status"` // "open" | "closed"
    UpdateTime   string `json:"update_time"`   // "YYYY-MM-DD HH:MM:SS" 本地时区
    TotalCount   int    `json:"total_count"`
}
type DailyPoint struct { Date string; Premium *float64 }  // 自定义 JSON → [date, premium]
type WatchlistResponse struct { Codes []string `json:"codes"` }
type ToggleResponse struct { Codes []string `json:"codes"`; InWatchlist bool `json:"in_watchlist"` }
type HistoryResponse struct { Code string `json:"code"`; History []HistoryPoint `json:"history"` }
type ErrorDetail struct { Detail string `json:"detail"` }  // 502 响应形状
```

`DailyPoint` 与 `HistoryPoint` 一样需要自定义 `MarshalJSON`（`[date, premium]`，date 为字符串）。

### 阶段 3：服务层（`internal/server`）

**3.1 缓存与轮询**

```go
type Cache struct {
    mu            sync.RWMutex
    resp          model.EtfsResponse
    hist          *history.Store        // 内嵌 history store
    lastDailySave string                // 每交易日只存一次
}
```

- `Start(ctx)`：goroutine 内 `ticker(PollInterval)` 循环调用 `refreshOnce()`
- `refreshOnce()`：`quote.Fetch` → `Parse` → 空数据则跳过（对齐 Python：warning + return false）→ 追加 history → save history.json → 判断 `IsTrading` → 若**收盘后且非周末且 `lastDailySave != today` 且存在 IOPV** 则 `store.UpsertDaily` → 更新 `Cache.resp`
- 手动 `/api/refresh` 与后台轮询共用 `refreshMu` 互斥，避免并发写缓存

**3.2 HTTP 路由（标准库 ServeMux，Go 1.22+ 语法）**

```
GET  /api/etfs                     → 直接返回 Cache.resp（纯内存）
GET  /api/watchlist                → {codes}
POST /api/watchlist/toggle/{code}  → watchlist.Toggle，返回 {codes, in_watchlist}
POST /api/refresh                  → refreshOnce()，失败返回 502 {"detail":"数据获取失败，上游不可达"}
GET  /api/fees                     → fees 缓存（原样）
GET  /api/history/{code}           → 内存 history（前端未消费，保留兼容）
GET  /api/daily/{code}             → store.QueryDaily → {code, daily:[[date,premium],...]}
GET  /{path...}                    → 静态文件 frontend/，不存在回退 index.html
```

- CORS 中间件：`Access-Control-Allow-Origin: *`、`GET,POST`、`*` headers（对齐 Python）
- 静态文件用 `http.FileServer` 包一层回退；`script.js`/`index.html` 带 `?v=N` 查询串，FileServer 天然支持

### 阶段 4：入口（`cmd/server/main.go`）

```go
func main() {
    cfg, err := config.Parse()  // 失败打印 usage + os.Exit(2)
    // 初始化：load history / fees / init db / 首次 refresh
    // 启动轮询 goroutine + http.Server
    // 优雅退出：SIGINT/SIGTERM → cancel ctx → 保存 history → 关 db
}
```

### 阶段 5：测试与端到端验证

- 单测：quote 解析（fixture）、market 边界、history 截断、store upsert/query、watchlist toggle、model 序列化
- 端到端：`go build` → 启动 → curl 验证 8 个端点（含 502 分支、静态回退）→ 浏览器打开确认网格/图表/星标功能

### 阶段 6：收尾

- `start.sh`（或 `start.bat`）：`cd go && go run ./cmd/server`（原 `start.sh` 实际不存在，需新建）
- 更新 `.gitignore`：补 `backend/data/`、`go/bin/`
- 更新 `AGENTS.md` 的 Go 架构部分；标注 `/api/fees`、`/api/history` 为兼容端点
- 可选：`embed.FS` 内嵌 frontend 静态资源（单文件 exe），作为后续优化项

---

## 五、契约对齐清单（实现时逐项核对）

| 端点 | 响应形状 | null 语义 |
|---|---|---|
| `GET /api/etfs` | `{nasdaq[], sp500[], market_status, update_time, total_count}` | ETF 内 `price/iopv/nav/premium/change_pct/volume/amount/prev_close/fund_scale/fee.*` 均可 null |
| `GET /api/watchlist` | `{codes[]}` | - |
| `POST /api/watchlist/toggle/{code}` | `{codes[], in_watchlist}` | - |
| `POST /api/refresh` | 同 `/api/etfs`；失败 `502 {"detail":"数据获取失败，上游不可达"}` | - |
| `GET /api/fees` | etf_fees.json 原样（key=code，字段 `mgmt_fee/custodian_fee/total_fee`） | - |
| `GET /api/history/{code}` | `{code, history:[[ts,premium]...]}` | premium 可 null |
| `GET /api/daily/{code}` | `{code, daily:[[date,premium]...]}`，date 为 `"YYYY-MM-DD"` 字符串 | premium 可 null |
| `GET /{path}` | 静态文件 → index.html 回退 | - |

前端关键假设（不可破坏）：
- `change_pct` 前端的 `etf.change_pct >= 0` 判断（null 时走 false 分支，Python 同款行为，Go `*float64` 保持一致）
- `selectETF('513500')` 在页面加载时立即请求 `/api/daily/513500`（**SQLite 必须建表并容忍空数据**）
- `update_time` 必须为本地时区字符串

---

## 六、风险与注意

1. `go.mod` 里 `golang.org/x/text` 是 indirect——加了 GBK 解码后运行 `go mod tidy` 会转 direct，属预期。
2. `etf_fees.json` 两个形状：`/api/fees` 原样返回 vs ETF 内嵌的 `{mgmt,custodian,total}`，别复用同一个 struct。
3. history.json 并发写：轮询与 `/api/refresh` 共用锁后，写文件只在锁内进行，单线程安全。
4. 快照只存一次：用 `lastDailySave`（记录日期）防止 Python 版每次刷新重复写盘；同时保留 `has_iopv` 检查防脏数据。
5. 不引入 gin/echo/viper 等框架——现有依赖已覆盖全部需求，保持零额外第三方依赖（除 sqlite/x-text）。
