// Package watchlist 管理置顶 ETF 代码文件（backend/watchlist.txt，每行一个 code）。
//
// 服务是单进程单实例，无需文件锁（Python 版的 fcntl 在 Windows 上不可用且
// import 被注释掉，属缺陷）；写入用"临时文件 + rename"原子替换，避免写一半损坏文件。
package watchlist

import (
	"os"
	"path/filepath"
	"strings"
)

// Read 读取代码列表，保留文件顺序，忽略空行。
// 文件不存在时返回 (nil, nil)。
func Read(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var codes []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			codes = append(codes, line)
		}
	}
	return codes, nil
}

// Toggle 切换某代码的置顶状态并原子写回文件。
//
// 返回更新后的完整列表（保持原顺序，新增追加到末尾）以及该代码是否在列表中。
func Toggle(path string, code string) ([]string, bool, error) {
	codes, err := Read(path)
	if err != nil {
		return nil, false, err
	}
	in := false
	for i, c := range codes {
		if c == code {
			codes = append(codes[:i], codes[i+1:]...)
			in = true
			break
		}
	}
	if !in {
		codes = append(codes, code)
	}
	if err := writeAtomic(path, codes); err != nil {
		return nil, false, err
	}
	return codes, !in, nil
}

// writeAtomic 以"临时文件 + rename"方式原子写入代码列表。
func writeAtomic(path string, codes []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".watchlist-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后此调用无害
	if _, err := tmp.WriteString(strings.Join(codes, "\n") + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
