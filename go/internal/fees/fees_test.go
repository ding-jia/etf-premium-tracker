package fees

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etf_fees.json")
	content := `{
		"513100": {"mgmt_fee": 0.60, "custodian_fee": 0.20, "total_fee": 0.80},
		"159941": {"mgmt_fee": 0.80, "custodian_fee": 0.20, "total_fee": 1.00}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	info, ok := m["513100"]
	if !ok {
		t.Fatal("513100 missing")
	}
	if info.MgmtFee != 0.6 || info.CustodianFee != 0.2 || info.TotalFee != 0.8 {
		t.Fatalf("info = %+v", info)
	}
	if _, ok := m["999999"]; ok {
		t.Fatal("unknown code should be absent")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("want error for missing file")
	}
}
