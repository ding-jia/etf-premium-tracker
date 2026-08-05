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
