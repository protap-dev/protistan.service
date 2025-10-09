package customers

import (
	"errors"
	"strings"
)

// AddressValidator handles address input validation
type AddressValidator interface {
	ValidateAddress(req *AddAddressRequest) error
	ValidateUpdateAddress(req *UpdateAddressRequest) error
}

type addressValidator struct{}

func NewAddressValidator() AddressValidator {
	return &addressValidator{}
}

func (v *addressValidator) ValidateAddress(req *AddAddressRequest) error {
	// Check required fields
	if req.Label == "" {
		return errors.New("label is required")
	}
	if req.StreetAddress == "" {
		return errors.New("street address is required")
	}
	if req.City == "" {
		return errors.New("city is required")
	}
	if req.State == "" {
		return errors.New("state is required")
	}

	// Validate label
	validLabels := []string{"home", "office", "other"}
	isValidLabel := false
	for _, label := range validLabels {
		if req.Label == label {
			isValidLabel = true
			break
		}
	}
	if !isValidLabel {
		return errors.New("invalid label (must be: home, office, other)")
	}

	// Validate address length
	if len(strings.TrimSpace(req.StreetAddress)) < 5 {
		return errors.New("street address must be at least 5 characters")
	}
	if len(strings.TrimSpace(req.StreetAddress)) > 500 {
		return errors.New("street address must be no more than 500 characters")
	}

	// Validate coordinates if provided
	if req.Longitude != 0 || req.Latitude != 0 {
		if req.Longitude < -180 || req.Longitude > 180 {
			return errors.New("longitude must be between -180 and 180")
		}
		if req.Latitude < -90 || req.Latitude > 90 {
			return errors.New("latitude must be between -90 and 90")
		}
	}

	return nil
}

func (v *addressValidator) ValidateUpdateAddress(req *UpdateAddressRequest) error {
	// Validate label if provided
	if req.Label != nil {
		validLabels := []string{"home", "office", "other"}
		isValidLabel := false
		for _, label := range validLabels {
			if *req.Label == label {
				isValidLabel = true
				break
			}
		}
		if !isValidLabel {
			return errors.New("invalid label (must be: home, office, other)")
		}
	}

	// Validate address length if provided
	if req.StreetAddress != nil {
		streetLen := len(strings.TrimSpace(*req.StreetAddress))
		if streetLen < 5 {
			return errors.New("street address must be at least 5 characters")
		}
		if streetLen > 500 {
			return errors.New("street address must be no more than 500 characters")
		}
	}

	// Validate coordinates if provided
	if req.Longitude != nil || req.Latitude != nil {
		if req.Longitude != nil {
			if *req.Longitude < -180 || *req.Longitude > 180 {
				return errors.New("longitude must be between -180 and 180")
			}
		}
		if req.Latitude != nil {
			if *req.Latitude < -90 || *req.Latitude > 90 {
				return errors.New("latitude must be between -90 and 90")
			}
		}
	}

	return nil
}
