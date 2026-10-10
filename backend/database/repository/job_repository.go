package repository

import (
	"context"
	"errors"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type JobRepository struct {
	database *database.Database
}

type JobFilter struct {
	Certificates []string
	Kinds        []string
	Statuses     []string
	Page         int
	PerPage      int
}

type JobSearchResult struct {
	Items []Job
	Total int64
}

const MaxPendingIssuanceJobs = 64

const InterruptedIssuanceError = "issuance interrupted by process restart; external ACME outcome may be unknown"

var (
	ErrIssuanceQueueFull = errors.New("issuance queue is full")
	ErrIssuancePending   = errors.New("certificate issuance is already pending")
)

func (r *JobRepository) Admit(ctx context.Context, name, kind string) (Job, bool, error) {
	return r.admit(ctx, name, kind, "queued")
}

func (r *JobRepository) admit(ctx context.Context, name, kind, status string) (Job, bool, error) {
	var model database.Job

	created := false
	err := r.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		certificate, err := findEnabledCertificate(tx, name)
		if err != nil {
			return err
		}

		model.Certificate = certificate

		err = tx.Where("certificate_id = ? AND status IN ?", certificate.ID, []string{"queued", "running"}).First(&model).Error
		if err == nil {
			return nil
		}

		if !NotFound(err) {
			return err
		}

		var pending int64
		if err = tx.Model(&database.Job{}).Where("status IN ?", []string{"queued", "running"}).Count(&pending).Error; err != nil {
			return err
		}

		if pending >= MaxPendingIssuanceJobs {
			return ErrIssuanceQueueFull
		}

		model = database.Job{CertificateID: certificate.ID, Kind: kind, Status: status, StartedAt: time.Now().UTC()}
		if err = tx.Omit("Certificate").Create(&model).Error; err != nil {
			return err
		}

		model.Certificate = certificate
		created = true

		return nil
	})

	return jobFromModel(model), created, err
}

// Start supports synchronous issuance without bypassing admission or deduplication.
func (r *JobRepository) Start(ctx context.Context, name, kind string) (int64, error) {
	job, created, err := r.admit(ctx, name, kind, "running")
	if err != nil {
		return 0, err
	}

	if !created {
		return 0, ErrIssuancePending
	}

	return job.ID, nil
}

func (r *JobRepository) Get(ctx context.Context, id int64) (Job, error) {
	var model database.Job

	err := r.database.ORM().WithContext(ctx).Preload("Certificate").First(&model, id).Error

	return jobFromModel(model), err
}

func (r *JobRepository) Claim(ctx context.Context) (*Job, error) {
	var model database.Job

	err := r.database.ORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("Certificate").Where("status = ?", "queued").Order("id").First(&model).Error; err != nil {
			return err
		}

		model.Status = "running"
		model.StartedAt = time.Now().UTC()

		return tx.Model(&database.Job{}).Where("id = ? AND status = ?", model.ID, "queued").Updates(map[string]any{"status": model.Status, "started_at": model.StartedAt}).Error
	})
	if NotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	job := jobFromModel(model)

	return &job, nil
}

// An interrupted ACME request may already have issued externally. Never retry it
// automatically; retain its failed job for inspection and resume only queued work.
func (r *JobRepository) RecoverInterrupted(ctx context.Context) error {
	return r.database.ORM().WithContext(ctx).Model(&database.Job{}).Where("status = ?", "running").Updates(map[string]any{
		"status": "failed", "error": InterruptedIssuanceError, "finished_at": time.Now().UTC(),
	}).Error
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
		if err := tx.Preload("Certificate").First(&job, id).Error; err != nil {
			return err
		}

		return tx.Model(&database.Certificate{}).
			Where(&database.Certificate{ID: job.CertificateID}).
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
		Preload("Certificate").
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
			CertificateName: model.Certificate.Name,
			Kind:            model.Kind,
			Status:          model.Status,
			Error:           model.Error,
			StartedAt:       model.StartedAt,
			FinishedAt:      model.FinishedAt,
		})
	}

	return jobs, nil
}

func (r *JobRepository) Search(ctx context.Context, filter JobFilter) (JobSearchResult, error) {
	query := r.database.ORM().WithContext(ctx).Model(&database.Job{}).Joins("Certificate")
	if len(filter.Certificates) > 0 {
		query = query.Where("Certificate.name IN ?", filter.Certificates)
	}

	if len(filter.Kinds) > 0 {
		query = query.Where("jobs.kind IN ?", filter.Kinds)
	}

	if len(filter.Statuses) > 0 {
		query = query.Where("jobs.status IN ?", filter.Statuses)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return JobSearchResult{}, err
	}

	var models []database.Job

	err := query.Order(clause.OrderByColumn{Column: clause.Column{Table: "jobs", Name: "id"}, Desc: true}).
		Limit(filter.PerPage).Offset((filter.Page - 1) * filter.PerPage).Find(&models).Error
	if err != nil {
		return JobSearchResult{}, err
	}

	jobs := make([]Job, 0, len(models))
	for _, model := range models {
		jobs = append(jobs, jobFromModel(model))
	}

	return JobSearchResult{Items: jobs, Total: total}, nil
}

func (r *JobRepository) FilterOptions(ctx context.Context) (certificates, kinds, statuses []string, err error) {
	base := func() *gorm.DB { return r.database.ORM().WithContext(ctx).Model(&database.Job{}) }
	if err = base().Joins("JOIN certificates ON certificates.id = jobs.certificate_id").
		Distinct("certificates.name").Order("certificates.name").Pluck("certificates.name", &certificates).Error; err != nil {
		return nil, nil, nil, err
	}

	if err = base().Distinct("kind").Order("kind").Pluck("kind", &kinds).Error; err != nil {
		return nil, nil, nil, err
	}

	err = base().Distinct("status").Order("status").Pluck("status", &statuses).Error

	return certificates, kinds, statuses, err
}

func jobFromModel(model database.Job) Job {
	return Job{ID: model.ID, CertificateName: model.Certificate.Name, Kind: model.Kind,
		Status: model.Status, Error: model.Error, StartedAt: model.StartedAt, FinishedAt: model.FinishedAt}
}
