// Package store 提供 SQLite 持久化（每日溢价率快照）。
//
// 表结构与 Python 版 backend/data/premium.db 兼容：
//
//	daily_premium(code, date, premium, price, iopv, PRIMARY KEY(code, date))
package store

import (
	"database/sql"
	"os"
	"path/filepath"

	"etf-premium-tracker/internal/model"
	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 连接。
type Store struct {
	db *sql.DB
}

// DailyRow 是一条待写入的每日快照。
type DailyRow struct {
	Code    string
	Date    string
	Premium *float64
	Price   float64
	IOPV    float64
}

// Open 打开（必要时创建）SQLite 数据库并建立连接。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// Init 确保 daily_premium 表存在。
func (s *Store) Init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS daily_premium (
		code TEXT NOT NULL,
		date TEXT NOT NULL,
		premium REAL,
		price REAL,
		iopv REAL,
		PRIMARY KEY (code, date)
	)`)
	return err
}

// UpsertDaily 批量写入当日快照（INSERT OR REPLACE，幂等）。
func (s *Store) UpsertDaily(rows []DailyRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO daily_premium (code, date, premium, price, iopv) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		var premium any
		if r.Premium != nil {
			premium = *r.Premium
		}
		if _, err := stmt.Exec(r.Code, r.Date, premium, r.Price, r.IOPV); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// QueryDaily 按日期升序返回某只 ETF 的每日溢价率。
func (s *Store) QueryDaily(code string) ([]model.DailyPoint, error) {
	rows, err := s.db.Query(`SELECT date, premium FROM daily_premium WHERE code = ? ORDER BY date ASC`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DailyPoint
	for rows.Next() {
		var d model.DailyPoint
		var premium sql.NullFloat64
		if err := rows.Scan(&d.Date, &premium); err != nil {
			return nil, err
		}
		if premium.Valid {
			p := premium.Float64
			d.Premium = &p
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
