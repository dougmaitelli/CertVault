package repository

import (
	"context"
	"strings"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm/clause"
)

type AuditRepository struct {
	database *database.Database
}

type AuditFilter struct {
	Query     string
	Actors    []string
	Actions   []string
	Resources []string
	Page      int
	PerPage   int
}

type AuditSearchResult struct {
	Items []Audit
	Total int64
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

func (r *AuditRepository) DeleteBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.database.ORM().WithContext(ctx).
		Where("at < ?", cutoff).
		Delete(&database.AuditEvent{})
	return result.RowsAffected, result.Error
}

func (r *AuditRepository) List(ctx context.Context, limit int) ([]Audit, error) {
	page, err := r.Search(ctx, AuditFilter{Page: 1, PerPage: limit})
	return page.Items, err
}

func (r *AuditRepository) Search(ctx context.Context, filter AuditFilter) (AuditSearchResult, error) {
	query := r.database.ORM().WithContext(ctx).Model(&database.AuditEvent{})
	if value := likeValue(filter.Query); value != "" {
		query = query.Where(`actor LIKE ? ESCAPE '\' OR action LIKE ? ESCAPE '\' OR resource LIKE ? ESCAPE '\' OR detail LIKE ? ESCAPE '\' OR ip LIKE ? ESCAPE '\'`, value, value, value, value, value)
	}
	if len(filter.Actors) > 0 {
		query = query.Where("actor IN ?", filter.Actors)
	}
	if len(filter.Actions) > 0 {
		query = query.Where("action IN ?", filter.Actions)
	}
	if len(filter.Resources) > 0 {
		query = query.Where("resource IN ?", filter.Resources)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return AuditSearchResult{}, err
	}

	var models []database.AuditEvent
	err := query.
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).
		Limit(filter.PerPage).
		Offset((filter.Page - 1) * filter.PerPage).
		Find(&models).Error
	if err != nil {
		return AuditSearchResult{}, err
	}
	audits := make([]Audit, 0, len(models))
	for _, model := range models {
		audits = append(audits, Audit{
			ID: model.ID, At: model.At, Actor: model.Actor, Action: model.Action,
			Resource: model.Resource, Detail: model.Detail, IP: model.IP,
		})
	}
	return AuditSearchResult{Items: audits, Total: total}, nil
}

func (r *AuditRepository) FilterOptions(ctx context.Context) (actors, actions, resources []string, err error) {
	query := r.database.ORM().WithContext(ctx).Model(&database.AuditEvent{})
	if err = query.Distinct("actor").Order("actor").Pluck("actor", &actors).Error; err != nil {
		return nil, nil, nil, err
	}
	if err = query.Distinct("action").Order("action").Pluck("action", &actions).Error; err != nil {
		return nil, nil, nil, err
	}
	err = query.Distinct("resource").Order("resource").Pluck("resource", &resources).Error
	return actors, actions, resources, err
}

func likeValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
	return "%" + value + "%"
}
