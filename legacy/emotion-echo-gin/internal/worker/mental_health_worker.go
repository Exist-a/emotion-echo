package worker

import (
	"context"
	"log"
	"time"

	"emotion-echo-gin/internal/service"
)

// MentalHealthWorker 心理健康评估工作器
type MentalHealthWorker struct {
	mentalHealthService *service.MentalHealthService
}

// NewMentalHealthWorker 创建工作器
func NewMentalHealthWorker(mentalHealthService *service.MentalHealthService) *MentalHealthWorker {
	return &MentalHealthWorker{
		mentalHealthService: mentalHealthService,
	}
}

// BatchDailyAssessment 批量每日评估
func (w *MentalHealthWorker) BatchDailyAssessment(ctx context.Context) error {
	log.Println("Starting batch mental health assessment...")
	
	start := time.Now()
	if err := w.mentalHealthService.BatchDailyAssessment(ctx); err != nil {
		log.Printf("Batch assessment failed: %v", err)
		return err
	}
	
	log.Printf("Batch assessment completed in %v", time.Since(start))
	return nil
}
