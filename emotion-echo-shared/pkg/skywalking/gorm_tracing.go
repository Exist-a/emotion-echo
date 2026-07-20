package skywalking

import (
	"context"
	"path"
	"strings"

	"github.com/SkyAPM/go2sky"
	"gorm.io/gorm"
)

// InstrumentGORM 注册 GORM 回调，让 SELECT/INSERT/UPDATE/DELETE 自动产生 ExitSpan。
//
// 关键设计：
//   - 在每个 op type（Create/Query/Update/Delete/Row）的最早期注册 Before 钩子，
//     创建 ExitSpan；在最晚期注册 After 钩子，结束 span。
//   - 跳过 raw SQL（name = ""）和 transaction-only operations。
//   - 通过 db.Statement.Context 传递 ctx（GORM 自动用 ctx 注册到 statement）。
//     这意味着业务代码应使用 db.WithContext(ctx)，trace 才会接上
func InstrumentGORM(db *gorm.DB) {
	// 防御：nil DB 不挂回调（以前会 panic；Stage 26-A 暴露 bug #2）
	if db == nil {
		return
	}
	tgr := Tracer()

	for _, opType := range []string{"create", "query", "update", "delete", "row"} {
		db.Callback().Raw().Before("gorm:"+opType).Register("skywalking:begin_"+opType,
			makeBeginCallback(opType, tgr))
		db.Callback().Raw().After("gorm:after_"+opType).Register("skywalking:end_"+opType,
			makeEndCallback(tgr))
	}
}

// makeBeginCallback 返回创建 span 的回调
func makeBeginCallback(op string, tgr *go2sky.Tracer) func(*gorm.DB) {
	return func(d *gorm.DB) {
		if tgr == nil {
			return
		}
		ctx := contextOrBg(d.Statement.Context)

		// peer 形如 postgres@db:5432（更易在 UI 按 DB 类型过滤）
		peer := buildDBPeer(d)

		// 操作名：gorm.query users / gorm.update users
		name := "gorm." + op
		if d.Statement.Table != "" {
			name = "gorm." + op + " " + d.Statement.Table
		}

		end := createExitSpan(ctx, tgr, name, peer)
		// GORM Statement 是单次操作级共享的临时对象，存 end 引用在 Statement 上读不到，
		// 所以改用 map 缓存：d.InstanceGet/Set("skywalking:span")
		d.InstanceSet("skywalking:end", end)
		d.InstanceSet("skywalking:err", nil)
	}
}

// makeEndCallback 返回结束 span 的回调
func makeEndCallback(tgr *go2sky.Tracer) func(*gorm.DB) {
	return func(d *gorm.DB) {
		if tgr == nil {
			return
		}
		// 取出 begin 时存的 end 闭包并执行
		if v, ok := d.InstanceGet("skywalking:end"); ok {
			if end, ok := v.(func(...EndOption)); ok {
				// 错误时打 error tag
				if d.Error != nil && d.Error != gorm.ErrRecordNotFound {
					end(WithError(d.Error))
				} else {
					end()
				}
			}
		}
	}
}

// buildDBPeer 把 GORM 的 Conn 拼成 peer 串
//
// 边界：d == nil / d.Statement == nil 时返回兜底 "db"，
// 避免 nil pointer 解引用 panic（见 Stage 26-A 暴露 bug #1）。
func buildDBPeer(d *gorm.DB) string {
	if d == nil {
		return "db"
	}
	// d.Dialector 含 DB 名与连接方式，但拿不到 host:port（那是 instance 配置）
	// 简化用 dbname 兜底
	if d.Dialector != nil {
		// postgres 用 "postgres@<table>" 形式（DB 名通过 getter 不易取得，用 Schema.Table）
		dialect := strings.ToLower(path.Base(strings.ReplaceAll(d.Dialector.Name(), "*", "")))
		schemaName := "<unknown>"
		if d.Statement != nil && d.Statement.Schema != nil {
			schemaName = d.Statement.Schema.Name
		}
		return dialect + "@" + schemaName
	}
	return "db"
}

func contextOrBg(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
