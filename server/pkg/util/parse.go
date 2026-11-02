package util

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

func ParseDuration(d string) (time.Duration, error) {
	d = strings.TrimSpace(d)
	if len(d) == 0 {
		return 0, errors.New("Empty duration")
	}
	uintPattern := map[string]time.Duration{
		"d": time.Hour * 24,
		"h": time.Hour,
		"m": time.Minute,
		"s": time.Second,
	}
	var totalDuration time.Duration
	remaining := d
	for _, unit := range []string{"d", "h", "m", "s"} {
		for strings.Contains(remaining, unit) {
			uintIndex := strings.Index(remaining, unit)
			part := remaining[:uintIndex]
			if part == "" {
				part = "0"
			}
			val, err := strconv.Atoi(part)
			if err != nil {
				return 0, errors.New("Invalid duration format")
			}
			totalDuration += time.Duration(val) * uintPattern[unit]
			remaining = remaining[uintIndex+len(unit):]
		}
	}
	if len(remaining) > 0 {
		return 0, errors.New("Invalid duration format")
	}
	return totalDuration, nil
}

// EncodeCursor 将 (score, id) 编码为 base64(json([score, id])) 格式的游标字符串。
// score == 0 且 id == 0 表示无下一页，返回空串。
// 双字段游标保证同分元素的翻页不重复不遗漏。
func EncodeCursor(score float64, id uint) string {
	if score == 0 && id == 0 {
		return ""
	}
	data, _ := json.Marshal([]interface{}{score, id})
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeCursor 从 base64(json([score, id])) 格式的游标字符串中解码 (score, id)。
// cursor 为空或格式非法时返回 (0, 0)（表示第一页）。
func DecodeCursor(cursor string) (float64, uint) {
	if cursor == "" {
		return 0, 0
	}
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0, 0
	}
	var values []interface{}
	if err := json.Unmarshal(data, &values); err != nil || len(values) < 2 {
		return 0, 0
	}
	score, _ := values[0].(float64)
	id, _ := values[1].(float64)
	return score, uint(id)
}
