package repository

import "time"

// ArtisanVerificationDBModel represents the artisan_verifications table
type ArtisanVerificationDBModel struct {
	ID                    string    `gorm:"primarykey;type:uuid;default:generate_uuid()"`
	ArtisanID             string    `gorm:"column:artisan_id"`
	VerificationStatus    string    `gorm:"column:verification_status"`
	VerificationMethod    string    `gorm:"column:verification_method"`
	VerifiedBy            *string    `gorm:"column:verified_by"`
	VerificationNotes     string    `gorm:"column:verification_notes"`
	VerificationUpdatedAt time.Time `gorm:"column:verification_updated_at"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (ArtisanVerificationDBModel) TableName() string {
	return "artisan_verifications"
}
