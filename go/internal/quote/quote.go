// Package quote 负责从腾讯财经接口抓取并解析 ETF 行情。
//
// 字段索引与 backend/main.py 逐条对应：
//
//	[1] 名称  [2] 代码  [3] 现价  [4] 昨收  [6] 成交量(手)
//	[32] 涨跌幅%  [37] 成交额(万)  [72] 总份额  [77] 溢价率  [78] IOPV  [81] NAV
package quote

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"etf-premium-tracker/internal/fees"
	"etf-premium-tracker/internal/model"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// minFields 是腾讯响应单行最少字段数（索引 0-81，共 82 个）。
const minFields = 82

// BuildURL 拼接腾讯批量行情接口地址，如 http://qt.gtimg.cn/q=sh513100,sz159941,...
func BuildURL(meta []model.ETF) string {
	codes := make([]string, 0, len(meta))
	for _, e := range meta {
		prefix := "sz"
		if e.Exchange == "SH" {
			prefix = "sh"
		}
		codes = append(codes, prefix+e.Code)
	}
	return "http://qt.gtimg.cn/q=" + strings.Join(codes, ",")
}

// Fetch 抓取腾讯接口响应，按 GBK 解码为 UTF-8 文本。
func Fetch(ctx context.Context, url string, timeout time.Duration) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(body)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Parse 解析腾讯接口返回的 UTF-8 文本，产出行情列表。
//
// 语义对齐 Python 版：
//   - 行内字段数 < 82 或代码不在 meta 中 → 整行跳过
//   - 任何数值字段解析失败 → 整行跳过（对齐 Python 的 try/except 包裹）
//   - IOPV == 0 → premium 置为 null；IOPV/NAV 为 0 时对应字段置 null
//   - volume = 手 × 100；amount = 万 × 10000
//   - fund_scale = NAV × 总份额 / 1e8，四舍五入到 2 位
func Parse(raw string, meta []model.ETF, feesMap map[string]fees.Info) []model.ETF {
	byCode := make(map[string]model.ETF, len(meta))
	for _, e := range meta {
		byCode[e.Code] = e
	}

	var out []model.ETF
	for _, line := range strings.Split(raw, ";") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 形如 v_sh513100="1~name~code~...~";
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			line = line[idx+1:]
		}
		line = strings.Trim(line, `"`)
		parts := strings.Split(line, "~")
		if len(parts) < minFields {
			continue
		}
		code := parts[2]
		metaETF, ok := byCode[code]
		if !ok {
			continue
		}

		price, err := parseField(parts[3])
		if err != nil {
			continue
		}
		prevClose, err := parseField(parts[4])
		if err != nil {
			continue
		}
		changePct, err := parseField(parts[32])
		if err != nil {
			continue
		}
		volumeHands, err := parseField(parts[6])
		if err != nil {
			continue
		}
		turnoverWan, err := parseField(parts[37])
		if err != nil {
			continue
		}
		premium, err := parseField(parts[77])
		if err != nil {
			continue
		}
		iopv, err := parseField(parts[78])
		if err != nil {
			continue
		}
		nav, err := parseField(parts[81])
		if err != nil {
			continue
		}
		totalShares, err := parseField(parts[72])
		if err != nil {
			continue
		}

		var premiumPtr *float64
		var iopvPtr *float64
		if iopv != 0 {
			premiumPtr = f64(premium)
			iopvPtr = f64(round(iopv, 4))
		}
		var navPtr *float64
		if nav != 0 {
			navPtr = f64(round(nav, 4))
		}
		var fundScale *float64
		if nav != 0 && totalShares != 0 {
			fundScale = f64(round(nav*totalShares/1e8, 2))
		}

		fee := model.Fee{}
		if info, ok := feesMap[code]; ok {
			fee.Mgmt = f64(info.MgmtFee)
			fee.Custodian = f64(info.CustodianFee)
			fee.Total = f64(info.TotalFee)
		}

		out = append(out, model.ETF{
			Code:      code,
			Name:      parts[1],
			Category:  metaETF.Category,
			Manager:   metaETF.Manager,
			Exchange:  metaETF.Exchange,
			Price:     f64(price),
			IOPV:      iopvPtr,
			NAV:       navPtr,
			Premium:   premiumPtr,
			ChangePct: f64(changePct),
			Volume:    f64(volumeHands * 100),
			Amount:    f64(turnoverWan * 10000),
			PrevClose: f64(prevClose),
			Fee:       fee,
			FundScale: fundScale,
		})
	}
	return out
}

// parseField 解析数值字段：空字符串 → 0；非数字 → 错误（调用方跳过整行）。
func parseField(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func f64(v float64) *float64 { return &v }

func round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}
