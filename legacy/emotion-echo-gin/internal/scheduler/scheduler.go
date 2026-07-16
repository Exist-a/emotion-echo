package scheduler

import (
	"context"
	"log"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/worker"
)

// Scheduler 定时任务调度器
type Scheduler struct {
	cfg                 *config.Config
	emotionWorker       *worker.EmotionWorker
	mentalHealthWorker  *worker.MentalHealthWorker
	ticker              *time.Ticker
	stop                chan bool
}

// NewScheduler 创建调度器
func NewScheduler(cfg *config.Config, emotionWorker *worker.EmotionWorker, mentalHealthWorker *worker.MentalHealthWorker) *Scheduler {
	return &Scheduler{
		cfg:                cfg,
		emotionWorker:      emotionWorker,
		mentalHealthWorker: mentalHealthWorker,
		stop:               make(chan bool),
	}
}

// Start 启动调度器
func (s *Scheduler) Start() {
	if !s.cfg.Analysis.Enabled {
		log.Println("Scheduler disabled")
		return
	}

	// 解析定时表达式 (简化版，只支持每小时或每天)
	interval := s.parseSchedule()
	s.ticker = time.NewTicker(interval)

	log.Printf("Scheduler started, interval: %v", interval)

	go s.run()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.stop)
	}
}

// run 运行循环
func (s *Scheduler) run() {
	// 立即执行一次
	s.trigger()

	for {
		select {
		case <-s.ticker.C:
			s.trigger()
		case <-s.stop:
			return
		}
	}
}

// trigger 触发分析任务
func (s *Scheduler) trigger() {
	// 触发情绪分析
	log.Println("Scheduler triggering emotion analysis...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	if err := s.emotionWorker.BatchAnalyze(ctx); err != nil {
		log.Printf("Batch emotion analysis failed: %v", err)
	}
	cancel()

	// 触发心理健康评估
	log.Println("Scheduler triggering mental health assessment...")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Minute)
	if err := s.mentalHealthWorker.BatchDailyAssessment(ctx2); err != nil {
		log.Printf("Batch mental health assessment failed: %v", err)
	}
	cancel2()
}

// parseSchedule 解析定时表达式
func (s *Scheduler) parseSchedule() time.Duration {
	schedule := s.cfg.Analysis.CronSchedule
	
	// 简化解析，支持常见格式
	switch schedule {
	case "0 * * * *": // 每小时
		return time.Hour
	case "0 */6 * * *": // 每6小时
		return 6 * time.Hour
	case "0 3 * * *": // 每天凌晨3点
		return 24 * time.Hour
	default:
		// 默认每天
		return 24 * time.Hour
	}
}

// TriggerNow 立即触发一次分析（用于手动触发）
func (s *Scheduler) TriggerNow() {
	go s.trigger()
}
