package skywalking

import (
	"path"
	"strings"

	"github.com/SkyAPM/go2sky"
	"gorm.io/gorm"

	"github.com/emotion-echo/shared/pkg/skywalking/trace"
)

// InstrumentGORM 注册 GORM 回调，让 ORM 的 SELECT/INSERT/UPDATE/DELETE 与裸 SQL
// 各自产生 ExitSpan。
//
// 关键设计：
//   - 按 op 分派到**正确的 processor**：Create/Query/Update/Delete/Row 各挂自己的
//     begin（最早）与 end（最晚）回调；裸 SQL（db.Raw/db.Exec）单独挂 Raw。
//   - 历史 bug（CL-01 修复）：原实现把 5 个 op **全部**注册到 db.Callback().Raw()，
//     而 ORM 的 Create/Find/Updates/Delete 各走自己的 processor、**永不进 Raw**
//     ⇒ 5 个 svc 的 ORM 查询零 span（同时 Raw 上堆了 10 个无关回调）。
//   - 通过 db.Statement.Context 传递 ctx（GORM 自动把 ctx 挂到 statement）：
//     业务代码用 db.WithContext(ctx) 时 trace 才接上（无 ctx ⇒ 无父 span ⇒ 不建 span）。
func InstrumentGORM(db *gorm.DB) {
	// 防御：nil DB 不挂回调（以前会 panic；Stage 26-A 暴露 bug #2）
	if db == nil {
		return
	}
	tgr := trace.Tracer()
	cbs := db.Callback()

	cbs.Create().Before("gorm:create").Register("skywalking:begin_create",
		makeBeginCallback("create", tgr))
	cbs.Create().After("gorm:after_create").Register("skywalking:end_create",
		makeEndCallback(tgr))

	cbs.Query().Before("gorm:query").Register("skywalking:begin_query",
		makeBeginCallback("query", tgr))
	cbs.Query().After("gorm:after_query").Register("skywalking:end_query",
		makeEndCallback(tgr))

	cbs.Update().Before("gorm:update").Register("skywalking:begin_update",
		makeBeginCallback("update", tgr))
	cbs.Update().After("gorm:after_update").Register("skywalking:end_update",
		makeEndCallback(tgr))

	cbs.Delete().Before("gorm:delete").Register("skywalking:begin_delete",
		makeBeginCallback("delete", tgr))
	cbs.Delete().After("gorm:after_delete").Register("skywalking:end_delete",
		makeEndCallback(tgr))

	// Row processor 只有 "gorm:row"（无 gorm:after_row），故 begin/end 都相对它排序。
	cbs.Row().Before("gorm:row").Register("skywalking:begin_row",
		makeBeginCallback("row", tgr))
	cbs.Row().After("gorm:row").Register("skywalking:end_row",
		makeEndCallback(tgr))

	// 裸 SQL（db.Raw / db.Exec）走 Raw processor —— 单列一条，避免 5 个 op 回调堆在 Raw。
	cbs.Raw().Before("gorm:raw").Register("skywalking:begin_raw",
		makeBeginCallback("raw", tgr))
	cbs.Raw().After("gorm:raw").Register("skywalking:end_raw",
		makeEndCallback(tgr))
}

// makeBeginCallback 返回创建 span 的回调
func makeBeginCallback(op string, tgr *go2sky.Tracer) func(*gorm.DB) {
	return func(d *gorm.DB) {
		if tgr == nil {
			return
		}
		// peer 形如 postgres@db:5432（更易在 UI 按 DB 类型过滤）
		peer := buildDBPeer(d)

		// 操作名：gorm.query users / gorm.update users
		name := "gorm." + op
		if d.Statement.Table != "" {
			name = "gorm." + op + " " + d.Statement.Table
		}

		// ctx 直接取 Statement.Context（nil 由 trace.CreateExitSpan 内部兜底）
		end := trace.CreateExitSpan(d.Statement.Context, tgr, name, peer)
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
			if end, ok := v.(func(...trace.EndOption)); ok {
				// 错误时打 error tag
				if d.Error != nil && d.Error != gorm.ErrRecordNotFound {
					end(trace.WithError(d.Error))
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
