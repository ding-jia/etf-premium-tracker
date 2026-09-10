# 代码审查报告 — ETF 溢价率监控

> 审查日期：2026-08-05
>
> 修复完成：2026-08-05（全部修复项已实施并验证，详见文末「修复记录」）
>
> 范围：Go 版后端（`go/`，当前运行版本）+ 前端（`frontend/`）+ Python 旧版（`backend/main.py`）+ 配置/脚本/文档
>
> 方法：通读全部源码与单测；`go vet ./...`、`go test ./...` 全部通过
>
> 结论：整体质量良好（测试覆盖扎实、并发控制清晰、契约对齐严谨），存在 4 个真实缺陷、6 项健壮性建议、若干死代码与文档漂移。

---

## 🔴 真实缺陷（建议修复）

### 1. 前端图表遇 `null` 溢价率会直接崩溃

- 位置：`frontend/script.js:214`
- 代码：`const values = records.map(r => +r[1].toFixed(2));`
- 问题：`/api/daily/{code}` 允许返回 `["2024-01-08", null]`。后端在 IOPV==0 时把 `premium` 存为 `NULL`（`go/internal/quote/quote.go:130` + `go/internal/store/store.go:99` + `go/internal/server/server.go:113`）。只要有任意一天是 null，`r[1].toFixed` 抛 TypeError，整张图渲染失败并误报"历史数据加载失败"。且 `calcMA` 遇 null 会算出 NaN 污染均线。当前 DB 数据恰好未触发，但这是必然路径的雷。
- 修复建议：`values` 映射时 `r[1] == null ? null : +r[1].toFixed(2)`；`calcMA` 跳过 null 只对有效点求均值。

### 2. 配置默认路径与 start 脚本不一致 → 数据目录分叉

- 位置：`go/internal/config/config.go:39-42` vs `start.sh` / `start.bat`
- 问题：
  - `config.go` 默认 `backend/data`、`backend/watchlist.txt`（相对 CWD）
  - `start.sh` 传入 `-data-dir ../backend/data`
  - 因此 `cd go && go run ./cmd/server` 会把数据写到 `go/backend/data/`，与 start.sh 完全不是一套数据。仓库已残留未跟踪目录 `go/backend/`（2026-08-05 09:42 的陈旧 history.json + premium.db），而活跃数据在 `backend/data/`（15:24）。
  - **AGENTS.md 声称 `./start.sh` 与 `cd go && go run ./cmd/server` 等价，这是错误的**。
- 修复建议（二选一）：
  - 改 config：默认路径按 `os.Executable()` 或向上探测仓库根解析（推荐，彻底消除 CWD 依赖）；或
  - 改文档：说明必须用 start.sh 或用 flag 显式指定路径；删除残留 `go/backend/`；`.gitignore` 增加 `go/backend/`。

### 3. `history.json` 写盘非原子

- 位置：`go/internal/history/history.go:57`
- 代码：`os.WriteFile(path, data, 0o644)` 直接覆盖
- 问题：进程在写一半时崩溃/断电 → 整个历史文件损坏 → 下次启动 `Load` 失败退回空历史。watchlist 已用"临时文件 + rename"原子写（`go/internal/watchlist/watchlist.go`），history 却没有，两者不一致。
- 修复建议：改为 temp + rename（可复用 watchlist 的写法）。

### 4. 时区依赖 `time.Local`

- 位置：`go/internal/market/market.go`、`go/internal/server/server.go:87`（`now.Format("2006-01-02")`）
- 问题：交易状态判断和每日快照日期都用本机时区。部署在非中国时区（如 UTC 的 VPS）时全部错位。
- 修复建议：固定 `time.LoadLocation("Asia/Shanghai")` 注入（Server 已有 `Now func() time.Time` 可注入，成本很低）。

---

## 🟡 健壮性问题

| # | 位置 | 问题 | 建议 |
|---|---|---|---|
| 5 | `frontend/script.js:195` `fetchData` | 不检查 `resp.ok`，后端 5xx 时 `data.nasdaq` undefined → `.filter` 抛错（有 catch 兜底但提示误导）。`refreshData` 已检查，两者不一致 | `fetchData` 同样检查 `resp.ok` |
| 6 | `go/internal/history/history.go:31` `Load` | 加载后不截断到 maxLen(480)，旧文件超长不收敛 | Load 后按 maxLen 截断 |
| 7 | `go/internal/store/store.go:24` | 未设 `SetMaxOpenConns(1)`，SQLite 并发写有锁风险（当前访问量低） | Open 后 `db.SetMaxOpenConns(1)` |
| 8 | `go/internal/quote/quote.go:55` | GBK 严格解码（`NewDecoder().Bytes`），上游混入坏字节整批失败；Python 版用 `errors="replace"` 更宽容 | 使用容错解码 |
| 9 | `go/internal/server/server.go:229` toggle watchlist | 不校验 code 格式，可写入任意字符串 | 校验 6 位数字 |
| 10 | `go/internal/market/market.go` | 无节假日日历，国庆/春节等假期收盘后会把"假日数据"写入当日快照 | 可选：维护交易日历 |

---

## 🟢 死代码与文档漂移

- **JS 死代码**（`frontend/script.js`）：
  - `isMarketOpen`（只写不读，16/207/455 行）
  - `currentChartCode`、`closeChart()`（从未被调用——AGENTS.md 说"弹窗式图表，点击外部或 Escape 关闭"，实际是内嵌面板且无关闭机制，423-428 行）
  - `data-premium` 属性只写不读（104 行）
- **AGENTS.md 过期**：
  - 启动等价性表述错误（见缺陷 2）
  - "弹窗式图表"描述与实际内嵌面板不符
  - "`go run ./cmd/server` 直接运行"会踩数据目录分叉
- **`plan.md` 过期**：D1 写"默认 30s 轮询"，实际 30m；"缺失（本计划主体）"的部分已全部实现。
- **`backend/main.py:1,158`**：`fcntl` import 被注释但 `toggle_watchlist` 仍调用 `fcntl.flock`（Linux 上会 NameError）。Python 版已弃用，建议文件头标注"仅存历史"或删除。
- **未跟踪的实验文件**：`pyproject.toml`、`src/`、`uv.lock`、`.python-version`、`tmp/build-errors.log` 与 Go 重写无关（`.venv` 靠自身 `.gitignore` 里的 `*` 侥幸被忽略）。建议清理或有意提交。
- **`.gitignore`**：缺 `go/backend/`（残留数据目录）。

---

## ✅ 做得好的地方

- **测试覆盖扎实**：每个包都有单测，`go vet`/`go test` 全绿；快照"仅收盘后一次"有专门测试（`server_test.go` `TestDailySnapshotOnlyAfterClose`）
- **并发控制清晰**：`refreshMu` 串行化轮询与手动刷新、`mu` 保护缓存、`lastDailySave` 在临界区内
- **静态文件路径穿越防护**（`server.go:262` 前缀检查）、**watchlist 原子写**、**CORS** 齐全
- **API 契约与前端严格对齐**：`[ts,premium]`/`[date,premium]` 自定义 JSON 有 round-trip 测试，前端无需改动即复用
- 与 Python 版的语义差异（快照时机收紧、IOPV==0 → premium null）都有注释说明

---

## 📋 建议修复优先级

1. 缺陷 1：图表 null 崩溃（前端一行改动 + calcMA）
2. 缺陷 3：history 原子写
3. 缺陷 2：配置路径 / 文档 + 清理残留 `go/backend/`
4. 缺陷 4：时区固定 Asia/Shanghai
5. 健壮性 7：SQLite 连接数
6. 死代码与文档清理

---

## 🔧 修复记录（2026-08-05）

| 项 | 状态 | 修复内容 |
|---|---|---|
| 缺陷 1 | ✅ 已修 | `frontend/script.js`：`values` 映射对 null 输出 null（不渲染）；`calcMA` 仅对窗口内有效点求均值，避免 NaN；`node --check` 语法校验通过 |
| 缺陷 2 | ✅ 已修 | `go/internal/config/config.go`：新增 `RepoRoot` 向上探测（含 `go/go.mod` 的目录），未显式指定的路径默认值按仓库根解析，显式 flag 保持原样；新增 `config_test.go`（3 个用例）；删除残留 `go/backend/`；`.gitignore` 增 `go/backend/`。端到端验证：从 `go/` 不带 flag 运行，数据正确落盘 `backend/data/` |
| 缺陷 3 | ✅ 已修 | `go/internal/history/history.go`：`Save` 改为临时文件 + rename 原子写；`Load` 按 maxLen 截断超长旧文件；新增截断单测 |
| 缺陷 4 | ✅ 已修 | `go/internal/market/market.go`：新增 `ShanghaiTZ`（UTC+8 FixedZone，不依赖 tzdata）；`server.New` 默认 `now` 基于上海时区 |
| 健壮性 5 | ✅ 已修 | `frontend/script.js`：`fetchData` 检查 `resp.ok`，非 2xx 抛出带 detail 的错误 |
| 健壮性 6 | ✅ 已修 | 见缺陷 3（Load 截断） |
| 健壮性 7 | ✅ 已修 | `go/internal/store/store.go`：`SetMaxOpenConns(1)` |
| 健壮性 8 | ⚪ 已核实无需改 | 实测 x/text GBK 解码器对非法字节返回替换字符 U+FFFD（非错误），现有 `NewDecoder().Bytes` 已是容错语义；新增 `TestFetchGBKTolerant` 固化该行为 |
| 健壮性 9 | ✅ 已修 | `go/internal/server/server.go`：`handleToggleWatchlist` 校验 6 位数字 code，非法返回 400；新增单测 |
| 健壮性 10 | ⏸️ 暂缓 | 未实施：维护法定节假日/调休日历成本高且数据可靠性存疑（补班周末交易日问题），已记录为 AGENTS.md 已知局限，建议后续接入交易所官方日历 |
| 死代码 JS | ✅ 已修 | 删除 `isMarketOpen`、`currentChartCode`、`closeChart()`、`data-premium` 属性；`index.html` script 版本号 `?v=3` → `?v=4` |
| AGENTS.md | ✅ 已修 | 修正启动等价性表述（路径按仓库根解析）、图表面板描述（非弹窗）、交易时间时区与节假日局限说明 |
| plan.md | ✅ 已修 | 顶部标注「已执行完毕（2026-08-05），内容仅存档」 |
| main.py fcntl | ✅ 已修 | 移除 `fcntl` 死代码，改为 `Path.write_text` 直接写文件；文件头标注「仅供历史参考」 |
| 未跟踪实验文件 | ⏸️ 暂缓 | `pyproject.toml`、`src/`、`uv.lock`、`.python-version`、`tmp/` 未删除（用户意图不明），如需清理请单独确认 |

验证：`go vet ./...`、`go test ./...` 全绿；从 `go/` 目录不带 flag 构建运行 server.exe，`/api/etfs`（17 只）、`/api/watchlist`、`/api/daily`、`/api/fees`、静态回退全部 200，非法 code 返回 400，数据正确落盘 `backend/data/`，未再创建 `go/backend/`。

---

# 第二轮审查 — 2026-08-14

> 范围：全部 Go 后端 + 前端 + Python 旧版 + 配置/脚本/文档（在上轮修复基础上复审）。
>
> 方法：通读全部源码与单测；`go vet ./...`、`go test ./...`、`node --check script.js` 通过；`gofmt -l .` 干净；`go test -race` 因环境无 cgo 未能运行，竞争分析为静态推断。

## ✅ 上轮修复项复核

缺陷 1–4、健壮性 5–9 全部核实已落地（图表 null 处理、仓库根路径解析、history 原子写、ShanghaiTZ、SQLite 单连接、GBK 容错、toggle code 校验、死代码清理），代码与 review.md/AGENTS.md 一致。

## 🔴 本轮新发现

### N1. `history.Store.Get` 返回内部切片 → 与轮询 `Append` 数据竞争（已修）

- 位置：`go/internal/history/history.go` `Get` → `go/internal/server/server.go` `handleHistory`
- 问题：`Get` 持锁仅返回切片引用，`handleHistory` 解锁后 `json.Marshal` 读取；后台轮询 `Append` 持锁原地写同一 backing array（容量足够时）。并发读写下未定义行为。单测为顺序执行，race detector 也测不出。
- 修复：`Get` 返回副本 `append([]model.HistoryPoint(nil), ...)`，注释说明缘由。

### N2. 主题切换后图表配色不刷新（已修）

- 位置：`frontend/script.js` `applyTheme`；`frontend/bloomberg.css`
- 问题：`toggleTheme` 只改 `data-theme`，不重绘已渲染 Chart（`renderChart` 按 `theme` 取色）；且 bloomberg.css 缺 `.ma-toggle` 规则，深色主题下开关仍是浅色样式。
- 修复：`applyTheme` 在有 `lastChartRecords` 时重绘；bloomberg.css 补 `.ma-toggle`（含 `.active`/`:hover`）深色覆盖。

### N3. 前端 `toggleWatchlist` / `selectETF` 不检查 `resp.ok`（已修）

- 位置：`frontend/script.js`
- 问题：与 `fetchData`/`refreshData` 不一致；后端 5xx 时 `data.codes` undefined → `new Set(undefined)` 抛错，提示误导且星标状态不同步。
- 修复：两处均按 `fetchData` 模式检查 `resp.ok`，非 2xx 抛带 detail 的错误。

### N4. `http.Server` 无读写超时（已修）

- 位置：`go/cmd/server/main.go`
- 问题：慢客户端可长期占用连接（gosec G112）。
- 修复：`ReadHeaderTimeout: 5s`、`ReadTimeout/WriteTimeout: 15s`。

### N5. `internal/market/market.go` CRLF 行尾（已修）

- `gofmt -l` 报红，全仓库唯一非 LF 的 Go 文件；`gofmt -w` 统一为 LF。

## 🟡 遗留（未修，记录在案）

| # | 位置 | 问题 | 建议 |
|---|---|---|---|
| N6 | `README.md:28-29`、`AGENTS.md` start.sh 描述、`model.go:108` UpdateTime 注释 | 文档漂移：README"方式二路径相对 CWD"已过期（config 现按仓库根解析）；AGENTS.md 称 start.sh 为 `go run`，实际是编译运行 exe；UpdateTime 注释"本地时区"实为上海时区 | 下次文档改动时一并修正 |
| N7 | `server.go` history/daily handler、`quote.go` Fetch、`script.js` 排序/重复渲染 | history/daily 不校验 code（与 toggle 不一致）；Fetch 无 UA 与响应体大小限制；null 溢价按 0 参与排序；fetchData/refreshData 渲染逻辑重复 | 低优先，可并入日常小清理 |
| N8 | `market.go` | 无节假日日历，法定假期工作日会显示 "open" 且 15:00 后把节前陈旧数据写入当日快照（上轮已记录，仍存在） | 维持 AGENTS.md 已知局限，建议接交易所日历 |

## 📋 验证

- `go vet ./...`、`go test ./...`（9 包）全绿；`gofmt -l .` 空；`node --check script.js` 通过。
- 本轮改动文件：`go/internal/history/history.go`、`go/cmd/server/main.go`、`frontend/script.js`、`frontend/bloomberg.css`、`go/internal/market/market.go`（仅行尾）。

---

# 第三轮审查 — 2026-09-10

> 范围：全库（Go 后端 + 本地前端 `frontend/` + 在线版 `pages/` + 文档/脚本）
>
> 方法：通读全部源码；`go vet ./...`、`go test ./...`、`go test -race ./...`、`gofmt -l .`、`node --check`；用 Go 服务 + 静态服务 + 无头 Chromium（通过 DevTools 协议量取每列像素坐标）对本地版与在线版做渲染比对；真实调用腾讯行情/K 线接口核对字段语义。
>
> 更正：第二轮记录「`go test -race` 因环境无 cgo 未能运行」不成立，本机可正常运行且全绿。

## 本轮修复

### 1. 前端把 null 溢价率显示成「+0.00% / 正常」（已修）

- 位置：`frontend/script.js` `renderCard`
- `const premium = etf.premium ?? 0;` 使 `premium != null ? ... : 'N/A'` 与 `premiumLabel()` 里的 null 分支全部失效；IOPV 缺失（后端确实会返回 `premium: null`）时渲染为 `+0.00%` + `正常`，与真实数据无法区分。
- 修复：保留 null 原值，交给既有的 N/A 分支。

### 2. `-poll 0` 会让进程 panic（已修）

- 位置：`go/internal/config/config.go`、`go/internal/server/server.go` `Start`
- `time.NewTicker` 对非正间隔 panic，且位于后台 goroutine、无 recover → 整个进程退出（实测 `interval 0s -> panic: non-positive interval`）。`-timeout 0` 在 `http.Client` 中语义是「永不超时」，上游卡死会长期占住 `refreshMu`。
- 修复：`Parse()` 校验两个时长必须 > 0；新增 `TestParseRejectsNonPositiveDurations`。

### 3. 在线版图表把收盘价当溢价率画（已修）

- 位置：`pages/script.js` `fetchDaily` / `select` / `drawChart`
- `qfqday` 行第 3 个字段是**收盘价**（实测 sh513500 2026-09-09 = 2.682），却被画在「溢价率%」轴上；同一天本地版显示 8.96%，在线版显示 2.7，相差 3 倍以上，且数据源会随 localStorage 记录条数在「溢价率」与「收盘价」之间自行切换。
- 修复：优先用本地累积的溢价率历史；不足 2 条时回落收盘价，但数据集名改「收盘价」、纵轴单位改「元」、tooltip 同步，并在图表标题旁加图注说明。顺带用递增 token 丢弃「连续点击」产生的过期异步结果。

### 4. 在线版与本版展示不一致（已修）

- 位置：`pages/index.html` / `pages/script.js`
- 分叉来源：`d0424a1`（删名称列）、`077a824`（删费率列）这两个提交**只改了 `pages/`**，本地版一直是 11 列。
- 差异清单：少「名称」「费率」两列、少「代码/名称」排序项、无最低费率高亮、报错只在 console、时间格式不同（`2026/9/10 09:20:43` vs `2026-09-10 09:20:43`）、残留 3 处 `console.log`、`S&P500` 未转义。
- 修复：补齐 11 列 / 10 个排序项 / best-fee 高亮 / toast 报错 / 统一时间格式 / 删除调试日志。脚本比对确认两端表头列、排序项、数据行、骨架行完全一致。

### 5. 表头与数据行逐列错位（已修，两端）

- 两个根因，均已实测确认：
  1. 列宽用 `min-width` 约束 → 内容超过基准（如 "159501" 实际 51px > 48px）就把该列撑宽，表头与数据行宽度不同；
  2. **列表内竖向滚动条**使数据行可用宽度小于表头 —— 在线版实测 15px，被 `flex:1` 的名称列吃掉，导致其后的列整体偏移 15px；本地版则因 `.list-item` 比表头多 1px 右边框而偏移 1px。
- 修复：表头与数据行共用同一套 `flex` 基准（`flex: 0 1 <basis>` + `min-width: 0`，收缩量按 basis 等比例分配，理论上恒等）；补平左右边框差；新增 `syncHeaderGutter()` 把滚动条宽度补给表头右内边距并在 resize 时重算。
- 验证（CDP 量取 11 列 × 2 端的 left/width）：修复前本地 11 列全错、在线 9 列错；修复后**两端 0 错位**。

### 6. 刷新后选中行高亮丢失（已修，两端）

- `renderGrid` 重建 `innerHTML` 会清掉 `.selected`：本地版刷新后图表不再跟随；在线版的「重新选中」逻辑因为读取时机在重建之后，实际是死代码。
- 修复：两端都在重建前记住选中代码、重建后恢复。

## 仍遗留（本轮未改，需决策）

| # | 位置 | 问题 | 建议 |
|---|---|---|---|
| N9 | `backend/20060902.tar.gz` | 被 git 跟踪的 312KB 二进制：扩展名 `.tar.gz` 但内容是**未压缩 tar**（`tar xzf` 直接失败），且装的是 `.gitignore` 排除的 `data/premium.db` + `data/history.json` | 移出仓库，或改名 `.tar` / 真正 gzip；若属月度归档流程，建议改用 Release 附件 |
| N10 | `go/internal/server/server.go` | 收盘时段进程没运行时，当日快照永久缺失（只在 `IsAfterClose` 时落盘） | 启动时回补最近缺失的交易日，或写入已知局限 |
| N11 | 同上 | 部分抓取成功（如 3/17）会静默覆盖缓存与当日快照，本轮只加了告警日志 | 加阈值，低于 N% 视为失败 |
| N12 | `go/internal/history` | `history.json` 从不清理已下架代码（现存 5 个僵尸 code，冻结在 2026-06-03，`/api/history/{code}` 仍可读到） | Load 时按 `etfs.All` 剪枝 |
| N13 | `go/internal/quote/quote.go` | 上游走明文 `http://qt.gtimg.cn`，而 `pages/` 用的是 `https://`（说明上游支持 TLS） | 改 https |
| N14 | `go/internal/server/server.go` | 默认监听全网卡 + CORS `*` + 两个无鉴权 POST 端点（同网段可改置顶、触发抓取） | 默认 `127.0.0.1:8000`，或在文档中说明 |
| N15 | `go/cmd/server/main.go` | 关停时未等轮询 goroutine 收尾就 `db.Close()` | 用 done channel / WaitGroup 收尾 |

## 验证

- `go vet ./...`、`go test ./...`（11 包）、`go test -race ./...` 全绿；`gofmt -l .` 为空。
- `node --check` 两个前端脚本通过。
- 无头 Chromium 实测：两端 11 列逐列 left/width 完全相同（0 错位）；本地图表 `dataset=溢价率 %`（72 点），在线图表 `dataset=收盘价`、`y 轴=2.68元`（251 点）+ 图注。
- 真实接口核对：`qfqday` 第 3 字段为收盘价（sh513500 2026-09-09 = 2.682），证实此前在线版标注错误。
- 三份 ETF 元数据当前一致（17/17/17，费率数值一致）。
- 本轮改动文件：`go/internal/config/config.go`、`go/internal/config/config_test.go`、`go/internal/server/server.go`、`go/internal/etfs/etfs.go`（仅 gofmt）、`frontend/script.js`、`frontend/style.css`、`frontend/index.html`、`pages/index.html`、`pages/script.js`、`AGENTS.md`、`README.md`。
