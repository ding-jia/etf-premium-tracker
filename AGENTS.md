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
2. **列宽模型**：列头 `.lh-*` 与数据行 `.li-*` 共用同一套 `flex` 基准（`flex: 0 1 <basis>` + `min-width: 0`），这是逐列对齐的唯一依据；改列宽要用浏览器实测。注意 `.list-header` 是 `.list-panel > div` 里唯一**不带**纵向 flex 的那个子元素（CSS 用 `:not(.list-header)` 排除），否则 11 个列宽基准会变成高度基准、把表头撑成几百像素高。
3. **`daily.json` 格式**：`{"code": [["YYYY-MM-DD", premium|null], ...]}`，紧凑 JSON、key 排序、日期升序、`5.0` 写成 `5`（与原先 Go 导出对齐，避免整行 diff）。只由 `scripts/update_daily.py` 写入（临时文件 + rename 原子替换）。
4. **溢价率 null 语义**：`IOPV == 0` → premium 为 null，前端必须显示 N/A、图表跳过该点、均线只对有值的点求均值，不能当 0 渲染成「+0.00% / 正常」。
5. **部署触发**：用 `GITHUB_TOKEN` 推的提交不会触发其它 workflow，所以 `update-data.yml` 里那句显式 `gh workflow run "Deploy Pages"` 是必需的，别删。
6. **`pages/` 会被原样发布**：不要往 `pages/` 里放脚本、密钥或说明文件（`deploy-pages.yml` 上传的是整个 `pages/` 目录）。

## UI 布局与交互

### 桌面（> 900px）

- 一屏固定布局（`body{position:fixed}`，无页面滚动），`.list-panel` 是唯一的滚动容器，两个板块按内容占高。
- **两个板块共用一条吸顶列头**：`.list-header` 放在 `.list-panel` 顶层（`position:sticky`），不要再给每个板块各放一条 —— 双表头会多吃 32px 并挤掉数据行。
- 目标：**17 行全部一屏可见、零滚动**。实测基线：1440×900 / 1600×900 行高 42px；高度 ≤820px 的笔记本自动换成 36px 行高并隐藏页脚（`@media(min-width:901px) and (max-height:820px)`）。
- 图表列默认展开占 32% 宽；矮屏（≤820px）默认收起成 44px 细栏（`.columns.chart-collapsed`）。用户手动点过收起/展开后写进 localStorage（`chartCollapsed`），之后不再自动改。
- **新增 ETF 会打破"一屏"**：17 行已接近当前高度预算，加标的后要么接受列表内滚动，要么同步再压一档行高。

### 手机 / 窄屏（≤ 900px）

- `100dvh` 固定一屏 + 列表内部滚动，板块标题（`.section-header`）吸顶。
- 行是**两行卡片**：第一行 星标/代码/名称/溢价率/状态，第二行 涨跌/管理人/费率/成交额/规模。换行靠 `.li-break`（`flex:0 0 100%`）+ 各 `.li-*` 的 `order`，**改行内 span 时不要删 `.li-break`**。
- 图表是**底部抽屉**（`.chart-panel.open`，高 46dvh）：点行弹出、✕ 或 Esc 关闭。刻意**不加遮罩** —— 抽屉开着时还能直接点别的行切换标的，这是本应用的核心操作。
- 排序 `<select>` 字号必须 ≥16px（更小的话 iOS 聚焦时会放大整页），所以用 `height:22px` 压高度补偿。
- 星标字形保持小，用 `.li-star::after` 撑出 ≥44px 点击热区（伪元素不占布局高度）。
- `viewport-fit=cover` + `env(safe-area-inset-bottom)`：全面屏底部留白。
- 实测 390×844 能做到 17 行零滚动；360×640 与横屏 844×390 高度不够，列表内滚动属预期。

### 改版后必须自查

```bash
node --check pages/script.js
python3 -m http.server 8124 -d pages     # 浏览器逐项点：刷新、点行、切标的、切主题、缩到手机宽
```

重点是"一屏行数"：改完行高/表头/页脚后，用无头浏览器量一次"视口内可见行数"，别凭感觉。

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
- 手机端抽屉没有"下滑关闭"手势，只能点 ✕ 或 Esc（桌面端）；小屏（≤640px 高）与横屏手机仍需要少量滚动。

## 历史

`plan.md`（Go 重写计划）与 `review.md`（代码审查记录）随 Go 版一起搬到了 `../etf-premium-tracker-go/`。
