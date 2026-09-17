#!/usr/bin/env python3
"""抓一次腾讯行情，把当日溢价率并入在线版使用的静态日线数据。

在线版是纯静态站、写不回仓库，所以由 GitHub Actions 在收盘后跑这个脚本：
抓行情 → 并入 pages/data/daily.json → 提交 → 触发 Pages 部署。

用法（在仓库根目录执行）：

    python3 scripts/update_daily.py                # 收盘后写入当日溢价率
    python3 scripts/update_daily.py --dry-run      # 只抓取并打印，不写文件
    python3 scripts/update_daily.py --force        # 未到收盘也写入（写入的是盘中值）
    python3 scripts/update_daily.py --data pages/data/daily.json

只依赖 Python 3 标准库。本脚本原先由 Go 版的 go/cmd/snapshot 承担，
带后端的本地版（Go + SQLite）已拆到 ../etf-premium-tracker-go/。

行为约定（与原 Go 实现一致）：

  * 同一交易日只替换当天的值，反复运行幂等；
  * 上游任一行数值字段解析失败 → 整行跳过；
  * IOPV 缺失（无溢价率）的 ETF 跳过，宁缺勿假；
  * 写文件用「临时文件 + rename」原子替换，避免半截 JSON 被提交部署；
  * 序列化与 Go 的 encoding/json 对齐（紧凑、key 排序、5.0 写成 5），git diff 干净；
  * ETF 代码表从 pages/script.js 的 META 解析，不再维护第四份副本。
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import tempfile
import urllib.request
from datetime import datetime, timedelta, timezone

# 仓库根目录（本脚本位于 <repo>/scripts/ 下）
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRIPT_JS = os.path.join(ROOT, "pages", "script.js")
DEFAULT_DATA = "pages/data/daily.json"

# 上游接口：优先 https（与 pages/script.js 一致），失败时回落明文 http（与 Go 版一致）
QUOTE_URL_PREFIXES = ("https://qt.gtimg.cn/q=", "http://qt.gtimg.cn/q=")

# 中国标准时间 UTC+8：固定偏移，不依赖系统 tzdata
SHANGHAI_TZ = timezone(timedelta(hours=8))

# 腾讯单行字段数（索引 0-81）
MIN_FIELDS = 82
IDX_CODE = 2
# 需要解析成功的数值字段：任一失败即整行跳过（对齐 Go 版 Parse 的 try/except 语义）
NUMERIC_FIELDS = (3, 4, 6, 32, 37, 72, 77, 78, 81)
IDX_PREMIUM = 77
IDX_IOPV = 78

# 收盘时间：总分钟 >= 900（15:00）
CLOSE_MINUTES = 900


def load_codes(path: str = SCRIPT_JS) -> list[tuple[str, str]]:
    """从 pages/script.js 的 META 解析 (code, exchange)，保持原顺序并去重。

    单一数据源：ETF 元数据的唯一副本就是 pages/script.js，这里不再抄一份。
    """
    with open(path, encoding="utf-8") as fh:
        text = fh.read()
    if "const META = [" not in text:
        raise SystemExit(f"错误：{path} 里找不到 META 定义")
    block = text.split("const META = [", 1)[1].split("];", 1)[0]
    rows = re.findall(r'\["(\d{6})","[^"]*","[^"]*","[^"]*","(SH|SZ)"\]', block)
    seen: set[str] = set()
    out: list[tuple[str, str]] = []
    for code, exchange in rows:
        if code not in seen:
            seen.add(code)
            out.append((code, exchange))
    if not out:
        raise SystemExit(f"错误：没能从 {path} 的 META 解析出 ETF 代码")
    return out


def build_query(codes: list[tuple[str, str]]) -> str:
    """拼接腾讯批量行情接口的 code 段，如 sh513100,sz159941,..."""
    return ",".join(("sh" if exchange == "SH" else "sz") + code for code, exchange in codes)


def fetch(url: str, timeout: float) -> str:
    """抓取腾讯接口响应，按 GBK 解码（非法字节用替换字符容错）。"""
    req = urllib.request.Request(url, headers={"User-Agent": "etf-premium-tracker/1.0"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        raw = resp.read()
    return raw.decode("gbk", errors="replace")


def parse(text: str, known: set[str]) -> dict[str, float | None]:
    """解析腾讯响应，返回 code → 溢价率（IOPV 为 0 或缺失时为 None）。"""
    out: dict[str, float | None] = {}
    for line in text.split(";"):
        line = line.strip()
        if not line:
            continue
        # 形如 v_sh513100="1~name~code~...~";
        if "=" in line:
            line = line.split("=", 1)[1]
        parts = line.strip('"').split("~")
        if len(parts) < MIN_FIELDS:
            continue
        code = parts[IDX_CODE]
        if code not in known:
            continue
        try:
            for idx in NUMERIC_FIELDS:
                float(parts[idx] or 0)
            iopv = float(parts[IDX_IOPV] or 0)
        except ValueError:
            continue
        out[code] = float(parts[IDX_PREMIUM] or 0) if iopv != 0 else None
    return out


def is_after_close(now: datetime) -> bool:
    """收盘后：非周末且总分钟 >= 900。"""
    if now.weekday() >= 5:  # 5=周六 6=周日
        return False
    return now.hour * 60 + now.minute >= CLOSE_MINUTES


def status_text(now: datetime) -> str:
    total = now.hour * 60 + now.minute
    if now.weekday() < 5 and (570 <= total < 690 or 780 <= total < 900):
        return "交易中"
    if is_after_close(now):
        return "已收盘"
    return "非交易时段"


def load_daily(path: str) -> dict[str, list]:
    """读取已导出的日线数据；文件不存在时返回空 map（首次运行）。"""
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except FileNotFoundError:
        return {}
    except json.JSONDecodeError as err:
        raise SystemExit(f"错误：{path} 不是合法 JSON（{err}），未做任何修改")
    if not isinstance(data, dict):
        raise SystemExit(f"错误：{path} 顶层不是对象，未做任何修改")
    return data


def upsert(points: list, date: str, premium: float) -> list:
    """写入某个交易日：当日已有则替换，否则追加到末尾。

    每日序列天然按日期升序，而任何一次抓取只可能写「今天」这一个日期，
    因此「已存在就替换、否则追加」即可保持有序，反复运行也幂等。
    """
    for point in points:
        if not isinstance(point, list) or len(point) != 2:
            raise SystemExit(f"错误：数据点格式不是 [date, premium]：{point!r}")
        if point[0] == date:
            point[1] = premium
            return points
    points.append([date, premium])
    return points


def format_number(value: object) -> str:
    """按 Go encoding/json 的风格输出数字：整数型浮点写 5 而不是 5.0。"""
    if value is None:
        return "null"
    if isinstance(value, bool):
        raise SystemExit(f"错误：溢价率不应为布尔值：{value!r}")
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        if value != value or value in (float("inf"), float("-inf")):
            raise SystemExit(f"错误：溢价率不是有限数字：{value!r}")
        text = repr(value)
        return text[:-2] if text.endswith(".0") else text
    raise SystemExit(f"错误：溢价率类型无法序列化：{value!r}")


def marshal(daily: dict[str, list]) -> str:
    """序列化为紧凑 JSON；dict 按键排序，与 Go 的 map 序列化一致。

    手工拼装而不是用 json.dumps：只有这样才能既保持 key 排序、又和 Go 一样
    把 5.0 写成 5，避免每次更新都产生无意义的整行 diff。
    """
    entries = []
    for code in sorted(daily):
        points = daily[code]
        if not isinstance(points, list):
            raise SystemExit(f"错误：{code} 的序列不是数组")
        body = ",".join(
            "[%s,%s]" % (json.dumps(point[0]), format_number(point[1]))
            for point in points
            if _check_point(code, point)
        )
        entries.append('"%s":[%s]' % (code, body))
    return "{" + ",".join(entries) + "}"


def _check_point(code: str, point: object) -> bool:
    if not isinstance(point, list) or len(point) != 2 or not isinstance(point[0], str):
        raise SystemExit(f"错误：{code} 存在格式异常的数据点：{point!r}")
    return True


def write_atomic(path: str, payload: str) -> None:
    """临时文件 + rename 原子写入（父目录自动创建，权限 0644）。"""
    directory = os.path.dirname(os.path.abspath(path))
    os.makedirs(directory, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=directory, prefix=".daily-", suffix=".json")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(payload)
        os.chmod(tmp, 0o644)
        os.replace(tmp, path)
    except BaseException:
        if os.path.exists(tmp):
            os.remove(tmp)
        raise


def resolve(path: str) -> str:
    """相对路径按仓库根解析（与 Go 版 config.RepoRoot 的行为对齐）。"""
    return path if os.path.isabs(path) else os.path.join(ROOT, path)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="抓取当日溢价率并并入在线版静态日线数据",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--data", default=DEFAULT_DATA,
                        help=f"日线数据文件路径，相对路径按仓库根解析（默认 {DEFAULT_DATA}）")
    parser.add_argument("--timeout", type=float, default=10.0, help="单次上游抓取超时秒数（默认 10）")
    parser.add_argument("--dry-run", action="store_true", help="只抓取并打印，不写入文件")
    parser.add_argument("--force", action="store_true", help="未到收盘时间也写入（写入的是盘中值）")
    args = parser.parse_args(argv)

    data_path = resolve(args.data)
    codes = load_codes()
    known = {code for code, _ in codes}

    text, last_err = None, None
    for prefix in QUOTE_URL_PREFIXES:
        try:
            text = fetch(prefix + build_query(codes), args.timeout)
            break
        except Exception as err:  # 依次尝试 https / http，最后统一报错
            last_err = err
    if text is None:
        raise SystemExit(f"抓取行情失败：{last_err}")

    quotes = parse(text, known)
    if not quotes:
        raise SystemExit("上游没有返回任何可用行情")

    now = datetime.now(SHANGHAI_TZ)
    today = now.strftime("%Y-%m-%d")
    fresh = sum(1 for premium in quotes.values() if premium is not None)
    print(f"已获取 {len(quotes)}/{len(codes)} 只 ETF 行情，其中 {fresh} 只有溢价率")
    print(f"快照日期 {today}（{now:%H:%M:%S}，{status_text(now)}）")

    if args.dry_run:
        print(f"dry-run：不写入 {data_path}")
        return 0
    if not args.force and not is_after_close(now):
        print("未到收盘时间（15:00 之后才写收盘值），跳过写入；如需强制写入盘中值请加 --force")
        return 0

    daily = load_daily(data_path)
    written = 0
    for code, premium in quotes.items():
        if premium is None:
            continue  # IOPV 缺失，宁可不写也不写 null
        daily[code] = upsert(daily.get(code) or [], today, premium)
        written += 1
    if written == 0:
        raise SystemExit("没有任何可写入的溢价率，保持文件不变")

    payload = marshal(daily)
    write_atomic(data_path, payload)
    total = sum(len(points) for points in daily.values())
    print(f"已更新 {data_path}：本次写入 {written} 只，累计 {len(daily)} 只 / {total} 点 / {len(payload)} 字节")
    return 0


if __name__ == "__main__":
    sys.exit(main())
