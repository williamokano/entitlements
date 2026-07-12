package service

import (
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/entitlements/internal/modules/catalog/internal/domain"
)

// PlanView is a read model of a plan.
type PlanView struct {
	ID     uuid.UUID
	Key    string
	Name   string
	Status string
	Public bool
}

// VersionView is a read model of a plan version.
type VersionView struct {
	ID            uuid.UUID
	PlanID        uuid.UUID
	Version       int
	Status        string
	Currency      string
	Prices        []domain.Price
	Trial         domain.TrialConfig
	GraceDays     int
	FeatureGrants map[string]any
	PublishedAt   *time.Time
}

func toPlanView(p *domain.Plan) PlanView {
	return PlanView{ID: p.ID, Key: p.Key, Name: p.Name, Status: string(p.Status), Public: p.Public}
}

func toPlanViews(plans []*domain.Plan) []PlanView {
	out := make([]PlanView, 0, len(plans))
	for _, p := range plans {
		out = append(out, toPlanView(p))
	}
	return out
}

func toVersionView(v *domain.PlanVersion) VersionView {
	prices := v.Prices
	if prices == nil {
		prices = []domain.Price{}
	}
	grants := v.FeatureGrants
	if grants == nil {
		grants = map[string]any{}
	}
	return VersionView{
		ID:            v.ID,
		PlanID:        v.PlanID,
		Version:       v.Version,
		Status:        string(v.Status),
		Currency:      v.Currency,
		Prices:        prices,
		Trial:         v.Trial,
		GraceDays:     v.GraceDays,
		FeatureGrants: grants,
		PublishedAt:   v.PublishedAt,
	}
}
