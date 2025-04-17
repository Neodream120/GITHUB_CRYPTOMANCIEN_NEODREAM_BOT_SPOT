package types

import "time"

// TimeUnit représente une unité de temps pour les intervalles
type TimeUnit string

const (
	Minutes TimeUnit = "minutes"
	Hours   TimeUnit = "hours"
	Days    TimeUnit = "days"
)

// TaskConfig représente la configuration d'une tâche planifiée
type TaskConfig struct {
	Name            string
	Type            string
	Interval        time.Duration
	IntervalValue   int
	IntervalUnit    TimeUnit
	Enabled         bool
	SpecificTime    string
	Exchange        string
	BuyOffset       float64
	SellOffset      float64
	Percent         float64
	LastRunTime     time.Time
	NextScheduledAt time.Time
}
