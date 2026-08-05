#!/usr/bin/env bash
# ETF 溢价率监控 Go 版启动脚本。
# 用法：./start.sh   （从仓库根目录运行）
#
# 直接运行已编译好的 go/server.exe，不再每次 go run 重新编译；
# 仅当 exe 不存在时（首次克隆/构建产物被清理）才构建一次。
set -e
cd "$(dirname "$0")/go"
if [ ! -f server.exe ]; then
  echo "server.exe 不存在，先构建一次 ..."
  go build -o server.exe ./cmd/server
fi
echo "启动 ETF 溢价率监控 ... 访问 http://localhost:8000"
exec ./server.exe \
  -frontend ../frontend \
  -data-dir ../backend/data \
  -watchlist ../backend/watchlist.txt \
  -fees ../backend/etf_fees.json
