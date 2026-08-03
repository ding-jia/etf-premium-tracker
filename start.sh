#!/usr/bin/env bash
# ETF 溢价率监控 Go 版启动脚本。
# 用法：./start.sh   （从仓库根目录运行）
#
# 服务 CWD 在 go/ 下，通过 flag 把数据/前端路径指回仓库根，
# 与 config 默认值（相对 CWD）配合，直接双击 go/server.exe 也能工作。
set -e
cd "$(dirname "$0")/go"
echo "启动 ETF 溢价率监控 ... 访问 http://localhost:8000"
exec go run ./cmd/server \
  -frontend ../frontend \
  -data-dir ../backend/data \
  -watchlist ../backend/watchlist.txt \
  -fees ../backend/etf_fees.json
