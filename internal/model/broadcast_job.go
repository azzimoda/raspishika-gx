package model

import (
	"encoding/json"
	"time"
)

// BroadcastJob is a manual admin mass-broadcast queued in broadcast_jobs.
// When the admin confirms a broadcast, one row is enqueued per target
// platform; each platform's worker (the telegraph/vk schedule bot process)
// claims its own rows and re-resolves the audience at send time so the
// recipient list is fresh.
type BroadcastJob struct {
	ID         int64      `gorm:"column:id" json:"id"`
	Audience   string     `gorm:"column:audience" json:"audience"`
	Platform   Platform   `gorm:"column:platform" json:"platform"`
	Spec       *string    `gorm:"column:spec" json:"spec"`
	HTML       string     `gorm:"column:html" json:"html"`
	Status     string     `gorm:"column:status" json:"status"`
	CreatedBy  int64      `gorm:"column:created_by" json:"created_by"`
	ClaimedBy  *string    `gorm:"column:claimed_by" json:"claimed_by"`
	Error      *string    `gorm:"column:error" json:"error"`
	FinishedAt *time.Time `gorm:"column:finished_at" json:"finished_at"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (BroadcastJob) TableName() string { return "broadcast_jobs" }

// Job statuses tracked in broadcast_jobs.status.
const (
	JobPending = "pending"
	JobSending = "sending"
	JobDone    = "done"
	JobFailed  = "failed"
)

// Job audience filters, mirroring the migration comment. They are stored in
// broadcast_jobs.audience and resolved against the chat repository.
const (
	AudienceAll          = "all"
	AudiencePrivate      = "private"
	AudienceGroups       = "groups"
	AudienceActive       = "active"
	AudienceByGroup      = "by_group"
	AudienceByDepartment = "by_department"
)

// BroadcastJobSpec carries the audience parameters for the by_group and
// by_department filters, serialized as JSON in broadcast_jobs.spec.
type BroadcastJobSpec struct {
	Group      string `json:"group,omitempty"`
	Department string `json:"department,omitempty"`
}

// MarshalJobSpec serializes the audience spec for storage.
func MarshalJobSpec(spec BroadcastJobSpec) (string, error) {
	raw, err := json.Marshal(spec)
	return string(raw), err
}

// UnmarshalJobSpec parses the audience spec read from the database.
func UnmarshalJobSpec(raw *string) (BroadcastJobSpec, error) {
	var spec BroadcastJobSpec
	if raw == nil || *raw == "" {
		return spec, nil
	}
	err := json.Unmarshal([]byte(*raw), &spec)
	return spec, err
}
