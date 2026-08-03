package repository

import (
	"context"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm/clause"
)

type AuditRepository struct {
	database *database.Database
}

func (r *AuditRepository) Record(ctx context.Context, actor, action, resource, detail, ip string) {
	_ = r.database.ORM().WithContext(ctx).Create(&database.AuditEvent{
		At:       time.Now().UTC(),
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Detail:   detail,
		IP:       ip,
	}).Error
}

func (r *AuditRepository) List(ctx context.Context, limit int) ([]Audit, error) {
	var models []database.AuditEvent
	err := r.database.ORM().WithContext(ctx).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	audits := make([]Audit, 0, len(models))
	for _, model := range models {
		audits = append(audits, Audit{
			ID: model.ID, At: model.At, Actor: model.Actor, Action: model.Action,
			Resource: model.Resource, Detail: model.Detail, IP: model.IP,
		})
	}
	return audits, nil
}
