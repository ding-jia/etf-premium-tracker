@echo off
rem Start the Go backend. CWD is go\ here; flags point paths back to repo root.
cd /d "%~dp0go"
echo Starting ETF Premium Tracker ... http://localhost:8000
go run ./cmd/server -frontend ../frontend -data-dir ../backend/data -watchlist ../backend/watchlist.txt -fees ../backend/etf_fees.json
pause
