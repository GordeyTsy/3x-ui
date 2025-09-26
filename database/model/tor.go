package model

import "time"

// TorCredential stores SOCKS authentication credentials assigned to an inbound client when
// Tor upstream routing with per-client isolation is enabled.
type TorCredential struct {
	ID          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundID   int       `json:"inboundId" gorm:"index:idx_tor_credential,priority:1"`
	ClientEmail string    `json:"clientEmail" gorm:"index:idx_tor_credential,priority:2"`
	Username    string    `json:"username"`
	Password    string    `json:"password"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
