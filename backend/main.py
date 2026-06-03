import json
import time
import asyncio
import sqlite3
import logging
import httpx
from datetime import datetime
from pathlib import Path
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.responses import JSONResponse, FileResponse, Response
from fastapi.middleware.cors import CORSMiddleware

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s", datefmt="%H:%M:%S")
logger = logging.getLogger("etf")


ETFS = [
    {"code": "513100", "name": "纳指ETF国泰", "exchange": "SH", "category": "nasdaq", "manager": "国泰基金"},
    {"code": "159941", "name": "纳指ETF广发", "exchange": "SZ", "category": "nasdaq", "manager": "广发基金"},
    {"code": "513300", "name": "纳斯达克ETF华夏", "exchange": "SH", "category": "nasdaq", "manager": "华夏基金"},
    {"code": "159632", "name": "纳斯达克ETF华安", "exchange": "SZ", "category": "nasdaq", "manager": "华安基金"},
    {"code": "513110", "name": "纳指ETF华泰柏瑞", "exchange": "SH", "category": "nasdaq", "manager": "华泰柏瑞基金"},
    {"code": "159696", "name": "纳指ETF易方达", "exchange": "SZ", "category": "nasdaq", "manager": "易方达基金"},
    {"code": "159501", "name": "纳指ETF嘉实", "exchange": "SZ", "category": "nasdaq", "manager": "嘉实基金"},
    {"code": "159513", "name": "纳斯达克100ETF大成", "exchange": "SZ", "category": "nasdaq", "manager": "大成基金"},
    {"code": "159659", "name": "纳斯达克100ETF招商", "exchange": "SZ", "category": "nasdaq", "manager": "招商基金"},
    {"code": "159660", "name": "纳指ETF汇添富", "exchange": "SZ", "category": "nasdaq", "manager": "汇添富基金"},
    {"code": "513390", "name": "纳指100ETF博时", "exchange": "SH", "category": "nasdaq", "manager": "博时基金"},
    {"code": "513870", "name": "纳指ETF富国", "exchange": "SH", "category": "nasdaq", "manager": "富国基金"},
    {"code": "159509", "name": "纳指科技ETF景顺", "exchange": "SZ", "category": "nasdaq", "manager": "景顺长城基金"},
    {"code": "513500", "name": "标普500ETF博时", "exchange": "SH", "category": "sp500", "manager": "博时基金"},
    {"code": "159655", "name": "标普500ETF华夏", "exchange": "SZ", "category": "sp500", "manager": "华夏基金"},
    {"code": "159612", "name": "标普500ETF国泰", "exchange": "SZ", "category": "sp500", "manager": "国泰基金"},
    {"code": "513650", "name": "标普500ETF南方", "exchange": "SH", "category": "sp500", "manager": "南方基金"},
]

HISTORY_DIR = Path(__file__).parent / "data"
HISTORY_FILE = HISTORY_DIR / "history.json"
DB_PATH = HISTORY_DIR / "premium.db"
cached_response = {"nasdaq": [], "sp500": [], "total_count": 0, "market_status": "closed", "update_time": ""}
history_store: dict = {}
history_lock = asyncio.Lock()
last_daily_save: str = ""


def load_history():
    global history_store
    if HISTORY_FILE.exists():
        try:
            history_store = json.loads(HISTORY_FILE.read_text())
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("failed to load history: %s", e)
            history_store = {}


def save_history():
    HISTORY_DIR.mkdir(exist_ok=True)
    HISTORY_FILE.write_text(json.dumps(history_store, ensure_ascii=False))


def _save_daily_snapshot(rows):
    with sqlite3.connect(str(DB_PATH)) as conn:
        conn.executemany(
            "INSERT OR REPLACE INTO daily_premium (code, date, premium, price, iopv) VALUES (?, ?, ?, ?, ?)",
            rows,
        )
        conn.commit()


def init_db():
    HISTORY_DIR.mkdir(exist_ok=True)
    with sqlite3.connect(str(DB_PATH)) as conn:
        conn.execute("""
            CREATE TABLE IF NOT EXISTS daily_premium (
                code TEXT NOT NULL,
                date TEXT NOT NULL,
                premium REAL,
                price REAL,
                iopv REAL,
                PRIMARY KEY (code, date)
            )
        """)
        conn.commit()


async def fetch_all() -> list:
    codes = ",".join(
        f"{'sh' if e['exchange'] == 'SH' else 'sz'}{e['code']}" for e in ETFS
    )
    url = f"http://qt.gtimg.cn/q={codes}"
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            resp = await client.get(url)
            text = resp.content.decode("gbk", errors="replace")
    except Exception as e:
        logger.error("fetch_all http error: %s", e)
        return []

    result = []
    for line in text.strip().split(";"):
        if not line.strip():
            continue
        parts = line.split("~")
        if len(parts) < 82:
            continue
        try:
            code = parts[2]
            price = float(parts[3]) if parts[3] else 0
            prev_close = float(parts[4]) if parts[4] else 0
            change_pct = float(parts[32]) if parts[32] else 0
            volume_hands = int(parts[6]) if parts[6] else 0
            turnover_wan = float(parts[37]) if parts[37] else 0
            premium = float(parts[77]) if parts[77] else 0
            iopv = float(parts[78]) if parts[78] else 0
            nav = float(parts[81]) if parts[81] else None
            name = parts[1]
        except (ValueError, IndexError):
            continue

        etf = next((e for e in ETFS if e["code"] == code), None)
        if not etf:
            continue

        if iopv == 0:
            premium = None

        fee = fees_cache.get(code)

        result.append({
            "code": code,
            "name": name,
            "category": etf["category"],
            "manager": etf["manager"],
            "exchange": etf["exchange"],
            "price": price,
            "iopv": round(iopv, 4) if iopv else None,
            "nav": round(nav, 4) if nav else None,
            "premium": premium,
            "change_pct": change_pct,
            "volume": volume_hands * 100,
            "amount": turnover_wan * 10000,
            "prev_close": prev_close,
            "fee": {
                "mgmt": fee["mgmt_fee"] if fee else None,
                "custodian": fee["custodian_fee"] if fee else None,
                "total": fee["total_fee"] if fee else None,
            },
        })
    return result


async def update_cache():
    global cached_response, history_store, last_daily_save
    data = await fetch_all()
    if not data:
        print("  fetch failed", flush=True)
        return

    now = datetime.now()
    now_ts = int(time.time())
    async with history_lock:
        for item in data:
            c = item["code"]
            if c not in history_store:
                history_store[c] = []
            history_store[c].append([now_ts, item["premium"]])
            history_store[c] = history_store[c][-480:]
        await asyncio.to_thread(save_history)

    is_weekend = now.weekday() >= 5
    total_min = now.hour * 60 + now.minute
    is_trading = not is_weekend and ((570 <= total_min < 690) or (780 <= total_min < 900))

    if not is_trading and not is_weekend:
        today = now.strftime("%Y-%m-%d")
        has_iopv = any(item.get("iopv") for item in data)
        if today != last_daily_save and has_iopv:
            rows = [(item["code"], today, item["premium"], item["price"], item["iopv"] or 0) for item in data]
            await asyncio.to_thread(_save_daily_snapshot, rows)
            last_daily_save = today
            print(f"  saved daily premiums for {today}", flush=True)

    cached_response = {
        "nasdaq": [d for d in data if d["category"] == "nasdaq"],
        "sp500": [d for d in data if d["category"] == "sp500"],
        "market_status": "open" if is_trading else "closed",
        "update_time": now.strftime("%Y-%m-%d %H:%M:%S"),
        "total_count": len(data),
    }
    print(f"  updated: {len(data)} ETFs", flush=True)


async def background_updater():
    await asyncio.sleep(2)
    await update_cache()
    while True:
        await asyncio.sleep(30)
        await update_cache()


@asynccontextmanager
async def lifespan(app: FastAPI):
    load_history()
    load_fees()
    init_db()
    task = asyncio.create_task(background_updater())
    yield
    task.cancel()


app = FastAPI(title="ETF Premium Tracker", lifespan=lifespan)
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["GET", "POST"], allow_headers=["*"])

FRONTEND_DIR = Path(__file__).parent.parent / "frontend"
WATCHLIST_FILE = Path(__file__).parent / "watchlist.txt"
FEES_FILE = Path(__file__).parent / "etf_fees.json"

fees_cache: dict = {}

def load_fees():
    global fees_cache
    if FEES_FILE.exists():
        try:
            fees_cache = json.loads(FEES_FILE.read_text())
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("failed to load fees: %s", e)
            fees_cache = {}


@app.get("/api/watchlist")
async def get_watchlist():
    if not WATCHLIST_FILE.exists():
        return {"codes": []}
    codes = [line.strip() for line in WATCHLIST_FILE.read_text().splitlines() if line.strip()]
    return {"codes": codes}


@app.post("/api/watchlist/toggle/{code}")
async def toggle_watchlist(code: str):
    import fcntl
    codes = []
    if WATCHLIST_FILE.exists():
        codes = [line.strip() for line in WATCHLIST_FILE.read_text().splitlines() if line.strip()]
    if code in codes:
        codes.remove(code)
    else:
        codes.append(code)
    with open(str(WATCHLIST_FILE), "w") as f:
        fcntl.flock(f, fcntl.LOCK_EX)
        try:
            f.write("\n".join(codes) + "\n")
        finally:
            fcntl.flock(f, fcntl.LOCK_UN)
    return {"codes": codes, "in_watchlist": code in codes}


@app.get("/api/etfs")
async def get_etfs():
    return cached_response


@app.get("/api/fees")
async def get_fees():
    return fees_cache


@app.get("/api/history/{code}")
async def get_history(code: str):
    async with history_lock:
        return {"code": code, "history": history_store.get(code, [])}


@app.get("/api/daily/{code}")
async def get_daily(code: str):
    rows = await asyncio.to_thread(_query_daily, code)
    return {"code": code, "daily": [[row[0], row[1]] for row in rows]}


def _query_daily(code: str):
    with sqlite3.connect(str(DB_PATH)) as conn:
        return conn.execute(
            "SELECT date, premium FROM daily_premium WHERE code = ? ORDER BY date ASC",
            (code,)
        ).fetchall()


@app.get("/{path:path}")
async def serve_frontend(path: str):
    if not FRONTEND_DIR.exists():
        return JSONResponse({"error": "frontend not found"}, status_code=404)
    target = FRONTEND_DIR / path
    if target.is_file():
        return FileResponse(str(target))
    index = FRONTEND_DIR / "index.html"
    if index.exists():
        return FileResponse(str(index))
    return JSONResponse({"error": "not found"}, status_code=404)


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000, log_level="info")
