package models

import "time"

// OrgEventType represents the type of organization event
type OrgEventType string

const (
	OrgCreated     OrgEventType = "org.created"
	OrgDeactivated OrgEventType = "org.deactivated"
)

// OrgEvent represents an organization lifecycle event
type OrgEvent struct {
	EventType    OrgEventType `json:"event_type"`
	EventID      string       `json:"event_id"`
	Timestamp    time.Time    `json:"timestamp"`
	Organization Organization `json:"organization"`
}

// Organization represents organization data in events
type Organization struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
