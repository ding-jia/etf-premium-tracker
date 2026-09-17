# ETF 溢价率监控 · 在线版

A 股美股 ETF 溢价率监控的**纯静态**页面（GitHub Pages）。无后端、无构建步骤：浏览器直连腾讯财经取行情，图表历史来自仓库里的 `pages/data/daily.json`。

线上地址：https://ding-jia.github.io/etf-premium-tracker/

带后端的本地版（Go + SQLite 实时面板 + `/api/*`）已拆到独立目录 `../etf-premium-tracker-go/`；本仓库只负责在线展示。

## 功能

- 跟踪 17 只标的（13 只纳指 + 4 只标普），溢价率四级配色、费率/成交额/规模列、10 项排序
- 点击列表行看历史溢价率折线（MA5/MA10/MA20 可切换），支持 Bloomberg 深色主题
- 置顶关注（localStorage，每浏览器独立）、手机端自适应（<768px 裁掉次要列）

## 本地预览

```bash
python3 -m http.server 8124 -d pages     # 打开 http://localhost:8124/
```

必须用 HTTP 服务打开：`file://` 下 `fetch('data/daily.json')` 会被 CORS 拦掉，图表会静默回落到收盘价。

## 数据来源

| 内容 | 来源 |
|---|---|
| 实时行情 | `<script>` 注入 `https://qt.gtimg.cn`（点刷新按钮时抓一次） |
| 图表历史 | `pages/data/daily.json`（随仓库部署）+ localStorage 里本机刷新记录，按日期合并 |
| 图表兜底 | 某只 ETF 一条溢价率历史都没有时，回落腾讯日 K 的**收盘价**，纵轴单位改「元」并加图注 |
| 置顶 | localStorage |

溢价率直接取腾讯字段 `[77]`；`IOPV == 0` 时为 null（前端显示 N/A，图表跳过该点，均线只对有值的点求均值）。

## 每日数据更新（无人值守）

`.github/workflows/update-data.yml` 在每交易日收盘后（北京时间 17:00 / 17:20 / 17:40，跑三次防抖动）执行 `scripts/update_daily.py`：

1. 抓腾讯行情 → 只取有溢价率的 ETF → 并入 `pages/data/daily.json`（同一天替换，反复运行幂等）
2. 文件有变化才提交（`chore: 更新在线版日线数据（YYYY-MM-DD）`）
3. 显式 `gh workflow run "Deploy Pages"` —— 用 `GITHUB_TOKEN` 推的提交不会触发其它 workflow

脚本只用 Python 3 标准库，不需要 Go 工具链。手动补数据 / 验证链路：Actions → **Update Daily Data** → Run workflow，可勾 `force`（未收盘也写，写入盘中值）或 `dry_run`（只抓取不写）。

本地也可以直接跑：

```bash
python3 scripts/update_daily.py --dry-run               # 只抓取并打印
python3 scripts/update_daily.py --data /tmp/daily.json  # 写到别处试验
```

> 已知行为：GitHub 免费 runner 的 `schedule` 常有数小时漂移（实测多在北京时间 20:00-23:00 才跑），数据日期取运行时日期，所以只是线上更新比收盘晚几小时，不影响正确性。

## 目录结构

```
pages/
  index.html          单文件页面（内联样式 + 手机端媒体查询）
  script.js           全部前端逻辑：META/FEES 元数据、取行情、渲染、图表、主题
  data/daily.json     每日溢价率历史 {"513100": [["2026-05-19", 2.5], ...]}
scripts/
  update_daily.py     收盘后把当日溢价率并入 daily.json（仅标准库）
.github/workflows/
  deploy-pages.yml    部署（push 到 master 且改动 pages/**）
  update-data.yml     每个交易日收盘后更新数据并触发部署
```

## 改动的注意点

- **ETF 元数据只有一份**：`pages/script.js` 的 `META`（代码/名称/板块/管理人/交易所）与 `FEES`（综合费率）。新增 ETF 只改这一个文件；`scripts/update_daily.py` 会自己从 `META` 解析代码表。
- 展示层（列、排序项、配色、格式化）全在 `pages/`，改动后 `node --check pages/script.js`，再本地起 HTTP 服务实际点一遍。
- `pages/data/daily.json` 由脚本写入（临时文件 + rename 原子替换），手工编辑请保持紧凑 JSON、key 排序、日期升序，避免出现无意义的整行 diff。

## 已知局限

- **无交易日历**：工作日节假日收盘后仍会写入一条点（沿用上一交易日收盘值），判断逻辑目前只有「非周末 + 15:00 之后」。要彻底解决得用上游自己的交易日信息（如日 K 最后一根 bar 的日期）做校验。
- 每日数据依赖 GitHub Actions 的 `schedule`，实测有数小时漂移。
- Chart.js 走 jsdelivr CDN，无本地兜底；CDN 不可用时页面没有图表。
- localStorage 里的点只有本机刷新过才有，跨设备只能等 `daily.json` 更新。

## 历史文档

Go 版的开发计划与代码审查（`plan.md`、`review.md`）已随 Go 版一起搬到 `../etf-premium-tracker-go/`。
