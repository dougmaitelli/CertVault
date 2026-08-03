package repository

import (
	"context"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type JobRepository struct {
	database *database.Database
}

func (r *JobRepository) Start(ctx context.Context, name, kind string) (int64, error) {
	model := database.Job{
		CertificateName: name,
		Kind:            kind,
		Status:          "running",
		StartedAt:       time.Now().UTC(),
	}
	if err := r.database.ORM().WithContext(ctx).Create(&model).Error; err != nil {
		return 0, err
	}
	return model.ID, nil
}

func (r *JobRepository) Finish(ctx context.Context, id int64, jobErr error) error {
	status := "succeeded"
	message := ""
	if jobErr != nil {
		status = "failed"
		message = jobErr.Error()
	}
	finishedAt := time.Now().UTC()
	return r.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.Job{}).
			Where(&database.Job{ID: id}).
			Updates(map[string]any{
				"status":      status,
				"error":       message,
				"finished_at": finishedAt,
			}).Error; err != nil {
			return err
		}
		if jobErr == nil {
			return nil
		}
		var job database.Job
		if err := tx.First(&job, id).Error; err != nil {
			return err
		}
		return tx.Model(&database.Certificate{}).
			Where(&database.Certificate{Name: job.CertificateName}).
			Updates(map[string]any{
				"status":     "error",
				"last_error": message,
				"updated_at": finishedAt,
			}).Error
	})
}

func (r *JobRepository) List(ctx context.Context, limit int) ([]Job, error) {
	var models []database.Job
	err := r.database.ORM().WithContext(ctx).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(models))
	for _, model := range models {
		jobs = append(jobs, Job{
			ID:              model.ID,
			CertificateName: model.CertificateName,
			Kind:            model.Kind,
			Status:          model.Status,
			Error:           model.Error,
			StartedAt:       model.StartedAt,
			FinishedAt:      model.FinishedAt,
		})
	}
	return jobs, nil
}
