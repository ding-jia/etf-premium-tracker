package quote

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"etf-premium-tracker/internal/etfs"
	"etf-premium-tracker/internal/fees"
)

var testFees = map[string]fees.Info{
	"513100": {MgmtFee: 0.60, CustodianFee: 0.20, TotalFee: 0.80},
	"159941": {MgmtFee: 0.80, CustodianFee: 0.20, TotalFee: 1.00},
}

// sampleLine 构造一条完整的腾讯响应行（恰好 82 个字段，索引 0-81）：
//
//	[3]现价 1.2340  [4]昨收 1.2000  [6]成交量(手) 123456
//	[32]涨跌幅 2.83  [37]成交额(万) 7890.12  [72]总份额 123456789.00
//	[77]溢价率 2.83  [78]IOPV 1.2001  [81]NAV 1.2340
func sampleLine() string {
	segments := []string{
		"1", "纳指ETF国泰", "513100", "1.2340", "1.2000", "1.2300", "123456",
		"7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31",
		"2.83",
		"33", "34", "35", "36",
		"7890.12",
		"38", "39", "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "50",
		"51", "52", "53", "54", "55", "56", "57", "58", "59", "60", "61", "62", "63",
		"64", "65", "66", "67", "68", "69", "70", "71",
		"123456789.00",
		"73", "74", "75", "76",
		"2.83",
		"1.2001",
		"79", "80",
		"1.2340",
	}
	if len(segments) != 82 {
		panic("fixture must have exactly 82 segments")
	}
	return `v_sh513100="` + strings.Join(segments, "~") + `~";`
}

// replaceField 替换 fixture 中指定索引的字段值。
func replaceField(line string, idx int, val string) string {
	line = strings.TrimSuffix(line, `";`)
	line = strings.TrimPrefix(line, `v_sh513100="`)
	parts := strings.Split(line, "~")
	if idx >= len(parts) {
		panic("idx out of range")
	}
	parts[idx] = val
	return `v_sh513100="` + strings.Join(parts, "~") + `~";`
}

func TestBuildURL(t *testing.T) {
	got := BuildURL(etfs.All[:2])
	want := "http://qt.gtimg.cn/q=sh513100,sz159941"
	if got != want {
		t.Fatalf("BuildURL = %q, want %q", got, want)
	}
}

func TestFetchGBKTolerant(t *testing.T) {
	// 上游响应混入非法 GBK 字节（0xFF）时，应以替换字符容错而非整批失败
	// （对齐 Python 版 errors="replace" 的语义）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte{'1', '~', 0xFF, 'x', '~'})
	}))
	defer srv.Close()

	got, err := Fetch(context.Background(), srv.URL, time.Second)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(got, "\uFFFD") {
		t.Fatalf("bad GBK byte should be replaced, got %q", got)
	}
	if !strings.Contains(got, "1~") {
		t.Fatalf("valid prefix should survive, got %q", got)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := Fetch(context.Background(), srv.URL, time.Second); err == nil {
		t.Fatal("want error on non-200 status")
	}
}

func TestParseBasic(t *testing.T) {
	got := Parse(sampleLine(), etfs.All, testFees)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	e := got[0]

	if e.Code != "513100" || e.Name != "纳指ETF国泰" || e.Category != "nasdaq" || e.Manager != "国泰基金" || e.Exchange != "SH" {
		t.Fatalf("meta fields = %+v", e)
	}
	if *e.Price != 1.234 {
		t.Errorf("price = %v, want 1.234", *e.Price)
	}
	if *e.PrevClose != 1.2 {
		t.Errorf("prev_close = %v, want 1.2", *e.PrevClose)
	}
	if *e.ChangePct != 2.83 {
		t.Errorf("change_pct = %v, want 2.83", *e.ChangePct)
	}
	if *e.Volume != 12345600 {
		t.Errorf("volume = %v, want 12345600", *e.Volume)
	}
	if math.Abs(*e.Amount-78901200.0) > 1e-3 {
		t.Errorf("amount = %v, want ~78901200", *e.Amount)
	}
	if *e.Premium != 2.83 {
		t.Errorf("premium = %v, want 2.83", *e.Premium)
	}
	if *e.IOPV != 1.2001 {
		t.Errorf("iopv = %v, want 1.2001", *e.IOPV)
	}
	if *e.NAV != 1.234 {
		t.Errorf("nav = %v, want 1.234", *e.NAV)
	}
	if *e.FundScale != 1.52 {
		t.Errorf("fund_scale = %v, want 1.52", *e.FundScale)
	}
	if e.Fee.Mgmt == nil || *e.Fee.Mgmt != 0.6 {
		t.Errorf("fee.mgmt = %v, want 0.6", e.Fee.Mgmt)
	}
	if e.Fee.Custodian == nil || *e.Fee.Custodian != 0.2 {
		t.Errorf("fee.custodian = %v, want 0.2", e.Fee.Custodian)
	}
	if e.Fee.Total == nil || *e.Fee.Total != 0.8 {
		t.Errorf("fee.total = %v, want 0.8", e.Fee.Total)
	}
}

func TestParseIOPVZero(t *testing.T) {
	line := replaceField(sampleLine(), 78, "0.0000")
	got := Parse(line, etfs.All, testFees)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Premium != nil {
		t.Fatalf("premium = %v, want nil when IOPV == 0", *got[0].Premium)
	}
	if got[0].IOPV != nil {
		t.Fatalf("iopv = %v, want nil when 0", *got[0].IOPV)
	}
	if got[0].FundScale == nil || *got[0].FundScale != 1.52 {
		t.Fatalf("fund_scale should still compute: %v", got[0].FundScale)
	}
}

func TestParseSkipsUnknownCode(t *testing.T) {
	line := replaceField(sampleLine(), 2, "999999")
	if got := Parse(line, etfs.All, testFees); len(got) != 0 {
		t.Fatalf("len = %d, want 0 (unknown code filtered)", len(got))
	}
}

func TestParseSkipsShortLine(t *testing.T) {
	raw := `v_sh513100="1~纳指ETF国泰~513100~1.23~";`
	if got := Parse(raw, etfs.All, testFees); len(got) != 0 {
		t.Fatalf("len = %d, want 0 (too few fields)", len(got))
	}
}

func TestParseSkipsBadNumeric(t *testing.T) {
	line := replaceField(sampleLine(), 3, "abc")
	if got := Parse(line, etfs.All, testFees); len(got) != 0 {
		t.Fatalf("len = %d, want 0 (bad numeric field)", len(got))
	}
}

func TestParseMultipleLines(t *testing.T) {
	line2 := replaceField(replaceField(sampleLine(), 2, "159941"), 1, "纳指ETF广发")
	raw := sampleLine() + "\n" + line2
	got := Parse(raw, etfs.All, testFees)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Code != "513100" || got[0].Exchange != "SH" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Code != "159941" || got[1].Exchange != "SZ" || got[1].Category != "nasdaq" {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestParseFeeAbsent(t *testing.T) {
	line := replaceField(sampleLine(), 2, "159941") // 159941 在 testFees 中有
	got := Parse(line, etfs.All, map[string]fees.Info{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Fee.Mgmt != nil || got[0].Fee.Custodian != nil || got[0].Fee.Total != nil {
		t.Fatalf("fee should be all nil when fees map empty: %+v", got[0].Fee)
	}
}
