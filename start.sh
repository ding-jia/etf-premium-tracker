#!/bin/bash
cd "$(dirname "$0")"
echo "=== ETF 溢价率监控系统 ==="
echo "安装依赖（如已安装可跳过）..."
pip install --break-system-packages fastapi uvicorn -q 2>/dev/null
echo "启动服务: http://0.0.0.0:8000"
echo ""
python3 backend/main.py
