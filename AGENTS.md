# ETF 溢价率监控 - 项目指南

## 概述
A股美股ETF溢价率实时监控面板。跟踪沪深两市上市的纳斯达克100和标普500ETF，实时显示溢价率。

- **前端**：Vanilla JS + Chart.js (CDN) + 纯 CSS
- **后端**：Python FastAPI + uvicorn
- **数据源**：腾讯财经 API (`qt.gtimg.cn`)，通过 `curl` 获取
- **持久化**：SQLite（每日快照）+ JSON（日内历史）
- **端口**：8000

## 目录结构
```
etf-premium-tracker/
├── start.sh                     # pip install + python3 backend/main.py
├── AGENTS.md                    # 本文件
├── .gitignore                   # __pycache__/, *.pyc, backend/data/
├── backend/
│   ├── main.py                  # FastAPI 服务（路由、数据获取、缓存、后台轮询）
│   ├── requirements.txt         # fastapi, uvicorn
│   ├── watchlist.txt            # 置顶的ETF代码（每行一个）
│   └── data/                    # 运行数据（已 gitignore）
│       ├── premium.db           # SQLite 表：daily_premium(code, date, premium, price, iopv)
│       └── history.json         # 日内历史：{code: [[unix_ts, premium], ...]}
└── frontend/
    ├── index.html               # SPA 外壳
    ├── style.css                # 全部样式（~500行）
    └── script.js                # 全部前端逻辑（~380行）
```

## 启动
```bash
./start.sh
# 访问 http://0.0.0.0:8000
```

## 交易时间（A股）
- **上午**：09:30-11:30
- **下午**：13:00-15:00
- **周末**：休市
- 在 `main.py:158-160` 中用 `now.hour * 60 + now.minute` 判断

## 后端 (`backend/main.py`)

### 数据流
1. `background_updater()` 在 FastAPI `lifespan` 启动时运行，每 **30秒** 轮询一次
2. `fetch_all()` → `curl qt.gtimg.cn` → 解析 GBK 响应 → 计算溢价率
3. `update_cache()` → 保存日内数据到 `history.json` → 判断 `is_trading` → 收盘时保存日快照到 SQLite → 更新 `cached_response`
4. `cached_response` 直接响应前端请求（纯内存，无需查 DB）

### API 端点
| 路径 | 返回内容 | 是否缓存 |
|---|---|---|
| `GET /api/etfs` | `{ nasdaq, sp500, market_status, update_time, total_count }` | 是（30s后台更新） |
| `GET /api/watchlist` | `{ codes: ["159501", ...] }` | 读取文件 |
| `GET /api/history/{code}` | `{ code, history: [[时间戳, 溢价], ...] }` | 内存 |
| `GET /api/daily/{code}` | `{ code, daily: [[日期, 溢价], ...] }` | 读 SQLite |
| `GET /{path:path}` | 静态文件，回退到 `index.html` | - |

### 溢价率计算
```python
# 主逻辑：从腾讯 API 字段 [77] 获取溢价率
# 兜底（IOPV=0时）：(现价 - 昨收) / 昨收 * 100
premium = (price - iopv) / iopv * 100
```

### 关键约定
- ETF 数据硬编码在 `ETFS` 列表（前后端各一份）
- `watchlist.txt` — 每行一个代码，置顶显示并带星标
- 日内历史每只 ETF 最多保留 **480 条**（约4小时，30s间隔）
- 每日快照在收盘后保存一次（首次 `is_trading=False` 时触发）
- `/api/etfs` 不查数据库，纯内存缓存

## 前端 (`frontend/script.js`)

### 刷新架构
- **状态检测**：`checkMarketStatus()` 每 **60秒** 通过 `setInterval` 运行。检测开/收盘转换。
  - 开盘 → 启动 5 分钟数据刷新定时器 + 立即拉取数据
  - 收盘 → 停止数据刷新定时器
- **数据刷新**：`fetchData()` 每 **5 分钟** 通过 `setInterval(startAutoRefresh)` 运行。仅当市场开盘时才渲染网格（除非 `forceRender=true`）。
- **手动刷新**：`fetchData(true)` — 无论是否开盘都渲染
- **开关**：用户可暂停/恢复自动刷新。恢复时若处于开盘状态，重启定时器

### 全局状态
| 变量 | 用途 |
|---|---|
| `autoRefresh` | 用户开关（true=自动刷新） |
| `refreshInterval` | 5分钟数据刷新定时器 |
| `statusInterval` | 60秒状态检测定时器 |
| `isMarketOpen` | 从 API 响应中获取 |
| `allData` | 最新缓存响应 |
| `watchlist` | 置顶代码集合 |
| `chartInstance` | Chart.js 实例 |

### ETF 数据字段（来自 API）
```
{ code, name, category, manager, exchange, price, iopv, nav,
  premium, change_pct, volume, amount, prev_close }
```

### 溢价率等级与颜色
| 条件 | 等级 | CSS 类名 |
|---|---|---|
| \|p\| > 5% | 极高/深度折价 | `premium-purple` |
| p > 0% | 溢价 | `premium-up` |
| p < 0% | 折价 | `premium-down` |
| 其他 | 正常 | (默认) |

### 图表
- Chart.js 4.4.4（CDN）
- 折线图 + 渐变填充
- 数据来自 `/api/daily/{code}`
- 点击打开弹窗，按 Escape 或点击外部关闭

### 排序
- 每个板块的下拉框（溢价降序/升序、代码、名称）
- 置顶项始终排在前面

## Git 工作流
- **无构建步骤** — 前端是纯 HTML/CSS/JS
- `backend/data/` 已被 gitignore（运行时数据）
- 使用中式简洁提交信息
- 修改后重启服务：`./start.sh`

## 常见操作

### 新增 ETF
在 `backend/main.py` 和 `frontend/script.js` 的 `ETFS` 列表中各加一条，`category` 使用 `"nasdaq"` 或 `"sp500"`。

### 置顶 ETF（关注）
将代码加入 `backend/watchlist.txt`，每行一个。

### 清除历史数据
删除 `backend/data/history.json` 和/或 `backend/data/premium.db`。
