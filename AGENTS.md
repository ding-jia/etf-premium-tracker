# ETF 溢价率监控 · 在线版 — 项目指南

## 概述

纯静态在线版（GitHub Pages）：**没有后端、没有构建步骤、没有 Go 代码**。

- **前端**：Vanilla JS + Chart.js 4.4.4（jsdelivr CDN）+ 单文件内联 CSS（`pages/index.html`）
- **行情**：浏览器 `<script>` 注入 `https://qt.gtimg.cn`（临时脚本标签，非 fetch）
- **图表数据**：仓库里的 `pages/data/daily.json` + localStorage 合并
- **部署**：`.github/workflows/deploy-pages.yml`（push 到 `master` 且改动 `pages/**`；默认分支是 `master` 而不是 `main`）
- **数据更新**：`.github/workflows/update-data.yml` + `scripts/update_daily.py`

带后端的本地版（Go + SQLite，端口 8000，`/api/*`）已拆到 `../etf-premium-tracker-go/`。

## 目录结构

```
pages/
  index.html           单文件页面：内联样式（含 <900px / <768px 媒体查询）
  script.js            全部逻辑：META/FEES、取行情、渲染、图表、主题、置顶
  data/daily.json      每日溢价率历史 {"513100": [["2026-05-19", 2.5], ...]}
scripts/
  update_daily.py      收盘后并入当日溢价率（仅 Python 3 标准库）
.github/workflows/
  deploy-pages.yml     部署 pages/ 到 GitHub Pages
  update-data.yml      每交易日收盘后更新数据 → 提交 → 显式触发部署
```

## 关键约定（改动前必读）

1. **ETF 元数据只有一份**：`pages/script.js` 的 `META`（代码/名称/板块/管理人/交易所）与 `FEES`（综合费率）。新增或下架 ETF 只改这一处；`scripts/update_daily.py` 会从 `META` 解析代码表，**不要再手抄一份代码清单**。
2. **列宽模型**：表头 `.lh-*` 与数据行 `.li-*` 共用同一套 `flex` 基准（`flex: 0 1 <basis>` + `min-width: 0`），这是逐列对齐的唯一依据；改列宽要两边同时改，并用浏览器实测。
3. **`daily.json` 格式**：`{"code": [["YYYY-MM-DD", premium|null], ...]}`，紧凑 JSON、key 排序、日期升序、`5.0` 写成 `5`（与原先 Go 导出对齐，避免整行 diff）。只由 `scripts/update_daily.py` 写入（临时文件 + rename 原子替换）。
4. **溢价率 null 语义**：`IOPV == 0` → premium 为 null，前端必须显示 N/A、图表跳过该点、均线只对有值的点求均值，不能当 0 渲染成「+0.00% / 正常」。
5. **部署触发**：用 `GITHUB_TOKEN` 推的提交不会触发其它 workflow，所以 `update-data.yml` 里那句显式 `gh workflow run "Deploy Pages"` 是必需的，别删。
6. **`pages/` 会被原样发布**：不要往 `pages/` 里放脚本、密钥或说明文件（`deploy-pages.yml` 上传的是整个 `pages/` 目录）。

## 常见操作

### 新增 / 下架 ETF

只改 `pages/script.js` 的 `META` 与 `FEES`（同一文件内，两处都要改）。下架时顺手删掉 `pages/data/daily.json` 里该代码的序列——脚本只追加/替换当天，不会清理已下架代码。

### 手动补一天数据

- Actions → **Update Daily Data** → Run workflow（勾 `force` 可写盘中值，勾 `dry_run` 只抓取不写）
- 或本地 `python3 scripts/update_daily.py`（收盘后才写），提交 `pages/data/daily.json` 后线上自动部署

### 本地预览

```bash
python3 -m http.server 8124 -d pages     # http://localhost:8124/
```

### 改完自查

```bash
node --check pages/script.js
python3 -m py_compile scripts/update_daily.py
python3 scripts/update_daily.py --dry-run            # 需要联网
```

再起本地 HTTP 服务实际点一遍：刷新按钮、点行切图表、切主题、切排序、点星标、缩到手机宽度。

## 已知局限

- **没有交易日历**：工作日节假日收盘后仍会写入一条点（沿用上一交易日收盘值）。当前判断只有「非周末 + 15:00 之后」；可靠修法是用上游自己的交易日信息（日 K 最后一根 bar 的日期）做校验。
- GitHub Actions 的 `schedule` 在免费 runner 上常有数小时漂移（实测多在北京时间 20:00-23:00 执行），数据日期取运行时日期，因此不影响正确性，只是更新晚。
- Chart.js 走 CDN 且无 SRI / 本地兜底。
- localStorage 的点只有本机刷新过才有；跨设备只能等 `daily.json`。

## 历史

`plan.md`（Go 重写计划）与 `review.md`（代码审查记录）随 Go 版一起搬到了 `../etf-premium-tracker-go/`。
