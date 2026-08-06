# ETF 溢价率监控 - Go 版启动脚本 (PowerShell)
#
# 用法：
#   .\start.ps1                     # 默认监听 :8000，30 分钟轮询
#   .\start.ps1 -Addr :9000         # 换端口
#   .\start.ps1 -Poll 5m            # 调整轮询间隔
#   .\start.ps1 -Frontend ../frontend
#
# 提示：
#   若执行报"禁止运行脚本"，先运行：Set-ExecutionPolicy -Scope Process Bypass
#   脚本可直接运行已编译好的 go\server.exe，仅在 exe 缺失时自动 go build。
#   Ctrl+C 停止服务。
#Requires -Version 5.1

param(
    [string]$Addr = ":8000",
    [string]$Poll = "30m",
    [string]$Frontend = ""
)

$ErrorActionPreference = "Stop"

# 脚本位于仓库根目录（无论从何处调用都成立）
$RepoRoot = $PSScriptRoot
$GoDir = Join-Path $RepoRoot "go"
$ServerExe = Join-Path $GoDir "server.exe"

Set-Location $GoDir

# 仅在 exe 缺失时才需要构建
if (-not (Test-Path $ServerExe)) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "[错误] 未找到 go 命令，且 server.exe 不存在，无法自动构建。" -ForegroundColor Red
        Write-Host "请先安装 Go：https://go.dev/dl/" -ForegroundColor Red
        exit 1
    }
    Write-Host "server.exe 不存在，先构建一次 ..."
    go build -o server.exe ./cmd/server
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[错误] go build 失败（退出码 $LASTEXITCODE）。" -ForegroundColor Red
        exit 1
    }
}

# 前端目录：未指定时默认仓库根下的 frontend
if ($Frontend -eq "") {
    $Frontend = Join-Path $RepoRoot "frontend"
}

$url = if ($Addr.StartsWith(":")) { "http://localhost$Addr" } else { "http://$Addr" }
Write-Host "启动 ETF 溢价率监控 ... 访问 $url（Ctrl+C 停止）" -ForegroundColor Cyan

try {
    & $ServerExe `
        -frontend $Frontend `
        -data-dir (Join-Path $RepoRoot "backend/data") `
        -watchlist (Join-Path $RepoRoot "backend/watchlist.txt") `
        -fees (Join-Path $RepoRoot "backend/etf_fees.json") `
        -addr $Addr `
        -poll $Poll
    $exitCode = $LASTEXITCODE
} finally {
    Set-Location $RepoRoot
}

if ($exitCode -eq 0) {
    Write-Host "服务已退出（正常关闭）。" -ForegroundColor Cyan
} else {
    Write-Host "服务异常退出，退出码: $exitCode（常见原因：端口被占用、上游网络异常）。" -ForegroundColor Red
    Read-Host "按回车键关闭窗口"
}
