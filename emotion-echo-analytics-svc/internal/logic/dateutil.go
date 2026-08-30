// Package logic — dateutil.go
//
// Stage 30-A Round 2 utility: parseDateWindow 是 day-night / depth
// / frequency 三处 logic 共用的日期窗口解析。
//
// 设计：
//   - parseDateWindow 返回本地时区 midnight + UTC midnight pair
//   - 顶层用本地时区（per reports_daily_logic 一致；业务边界
//     "今天" = 用户所在日）
//   - start 必须 <= end（与 trend 校验一致）
package logic

import (
	"fmt"
	"time"
)

// parseDateWindow 解析 [startDate, endDate] YYYY-MM-DD 区间
//
// 返回 (start, end) — 都是本地时区午夜。校验 start <= end。
func parseDateWindow(startDate, endDate string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01-02", startDate, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("validation: invalid start_date %q: %w", startDate, err)
	}
	end, err := time.ParseInLocation("2006-01-02", endDate, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("validation: invalid end_date %q: %w", endDate, err)
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("validation: start_date %s must be <= end_date %s", startDate, endDate)
	}
	return start, end, nil
}