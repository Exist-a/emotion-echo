package database

import (
	"fmt"
	"log"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPostgres 创建 PostgreSQL 连接
func NewPostgres(cfg *config.Config) (*gorm.DB, error) {
	dsn := cfg.GetPostgresDSN()

	// 始终使用 Silent 模式，避免 SQL 查询中的中文乱码问题
	logLevel := logger.Silent

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql db: %w", err)
	}

	// 设置连接池
	sqlDB.SetMaxOpenConns(cfg.Database.Postgres.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.Postgres.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(1 * time.Hour)

	// 自动迁移数据库表（带版本检查，避免与 SQL 迁移脚本冲突）
	if err := autoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Println("PostgreSQL connected")
	return db, nil
}

// autoMigrate 自动迁移数据库表
func autoMigrate(db *gorm.DB) error {
	log.Println("Running database migration...")
	
	// 处理手动迁移与 AutoMigrate 的兼容性问题
	// 如果 conversations.last_message_time 仍是 TIMESTAMP 类型，先手动转换为 BIGINT
	if err := migrateLastMessageTime(db); err != nil {
		return fmt.Errorf("failed to migrate last_message_time: %w", err)
	}
	
	err := db.AutoMigrate(
		&models.User{},
		&models.Conversation{},
		&models.Message{},
		&models.RefreshToken{},
		&models.Survey{},
		&models.SurveyResult{},
		&models.EmotionAnalysis{},
		&models.MentalHealthAssessment{},
	)
	
	if err != nil {
		return err
	}
	
	log.Println("Database migration completed")
	return nil
}

// migrateLastMessageTime 处理 last_message_time 列的类型转换
// 解决手动迁移与 GORM AutoMigrate 的冲突
func migrateLastMessageTime(db *gorm.DB) error {
	// 检查 conversations 表是否存在
	if !db.Migrator().HasTable("conversations") {
		return nil // 表不存在，无需处理
	}
	
	// 检查 last_message_time 列是否存在
	if !db.Migrator().HasColumn(&models.Conversation{}, "last_message_time") {
		return nil // 列不存在，无需处理
	}
	
	// 检查当前列类型
	var columnType string
	err := db.Raw(`
		SELECT data_type 
		FROM information_schema.columns 
		WHERE table_name = 'conversations' AND column_name = 'last_message_time'
	`).Scan(&columnType).Error
	
	if err != nil {
		return err // 可能是权限问题或其他错误
	}
	
	// 如果已经是 bigint，无需处理
	if columnType == "bigint" {
		return nil
	}
	
	// 如果是 timestamp/timestamptz，需要手动转换
	if columnType == "timestamp with time zone" || columnType == "timestamp without time zone" || columnType == "timestamp" {
		log.Println("Converting conversations.last_message_time from TIMESTAMP to BIGINT...")
		
		// 使用事务执行转换
		return db.Transaction(func(tx *gorm.DB) error {
			// 1. 添加新列
			if err := tx.Exec(`ALTER TABLE conversations ADD COLUMN last_message_time_new BIGINT`).Error; err != nil {
				return err
			}
			
			// 2. 转换数据
			if err := tx.Exec(`
				UPDATE conversations 
				SET last_message_time_new = EXTRACT(EPOCH FROM last_message_time) * 1000 
				WHERE last_message_time IS NOT NULL
			`).Error; err != nil {
				return err
			}
			
			// 3. 删除旧列
			if err := tx.Exec(`ALTER TABLE conversations DROP COLUMN last_message_time`).Error; err != nil {
				return err
			}
			
			// 4. 重命名新列
			if err := tx.Exec(`ALTER TABLE conversations RENAME COLUMN last_message_time_new TO last_message_time`).Error; err != nil {
				return err
			}
			
			log.Println("Conversion completed successfully")
			return nil
		})
	}
	
	return nil
}
