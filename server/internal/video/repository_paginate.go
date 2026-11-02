package video

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

// PaginateRepo 分页查询仓储（只操作 feed_video 库的视频表）
type PaginateRepo struct {
	db *gorm.DB
}

func NewPaginateRepo(db *gorm.DB) *PaginateRepo {
	return &PaginateRepo{db: db}
}

// CursorPaginate 执行游标分页查询。
func (r *PaginateRepo) CursorPaginate(ctx context.Context, query *gorm.DB, opt CursorOption, dest interface{}) (int64, string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 创建新 Session，避免修改传入的 *gorm.DB 对象状态影响调用方
	query = query.Session(&gorm.Session{})

	limit := opt.Limit
	if limit <= 0 || limit > 30 {
		limit = 10
	}

	var total int64
	// 使用 WithContext 传播超时控制，确保游标分页查询也受 context 超时约束
	if err := query.WithContext(ctx).Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return 0, "", err
	}

	if opt.Cursor != "" {
		values, err := decodeCursorValues(opt.Cursor)
		if err == nil && len(values) == len(opt.Fields) {
			query = applyCursorConditions(query, opt.Fields, values)
		}
	}

	for _, f := range opt.Fields {
		query = query.Order(f.Column + " " + strings.ToUpper(f.Direction))
	}

	query = query.Limit(limit + 1)

	if err := query.WithContext(ctx).Find(dest).Error; err != nil {
		return 0, "", err
	}

	destVal := reflect.ValueOf(dest).Elem()
	var nextCursor string
	if destVal.Len() > limit {
		lastItem := destVal.Index(limit)
		values := extractCursorValues(lastItem, opt.Fields)
		nextCursor = encodeCursorValues(values)
		destVal.SetLen(limit)
	}

	return total, nextCursor, nil
}

// applyCursorConditions 根据游标字段和值构建 WHERE 条件。
func applyCursorConditions(query *gorm.DB, fields []CursorField, values []interface{}) *gorm.DB {
	var conditions []string
	var args []interface{}

	for i := 0; i < len(fields); i++ {
		op := "<"
		if strings.ToUpper(fields[i].Direction) == "ASC" {
			op = ">"
		}

		var parts []string
		var partArgs []interface{}
		for j := 0; j < i; j++ {
			parts = append(parts, fields[j].Column+" = ?")
			partArgs = append(partArgs, values[j])
		}
		parts = append(parts, fields[i].Column+" "+op+" ?")
		partArgs = append(partArgs, values[i])

		conditions = append(conditions, "("+strings.Join(parts, " AND ")+")")
		args = append(args, partArgs...)
	}

	return query.Where(strings.Join(conditions, " OR "), args...)
}

// extractCursorValues 从结构体中提取游标字段值。
func extractCursorValues(v reflect.Value, fields []CursorField) []interface{} {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	var values []interface{}
	for _, f := range fields {
		goName := toPascalCase(f.Column)
		fieldVal := v.FieldByName(goName)
		if fieldVal.IsValid() {
			values = append(values, fieldVal.Interface())
		}
	}
	return values
}

// encodeCursorValues 将游标字段值编码为 base64 字符串。
func encodeCursorValues(values []interface{}) string {
	data, _ := json.Marshal(values)
	return base64.StdEncoding.EncodeToString(data)
}

// decodeCursorValues 从 base64 字符串解码游标字段值。
func decodeCursorValues(cursor string) ([]interface{}, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	var values []interface{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("invalid cursor values: %w", err)
	}
	return values, nil
}

// toPascalCase 将 snake_case 转换为 PascalCase。
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}
