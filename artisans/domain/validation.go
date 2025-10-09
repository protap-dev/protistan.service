package domain

import (
	"fmt"
	"strings"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) ValidateCreateProfile(input *CreateProfileInput) error {
	var errors []string

	// Category validation
	if len(input.CategoryIDs) == 0 {
		errors = append(errors, "at least one service category is required")
	}
	if len(input.CategoryIDs) > 10 {
		errors = append(errors, "maximum 10 service categories allowed")
	}

	// Validate individual category IDs are not empty
	for _, categoryID := range input.CategoryIDs {
		if strings.TrimSpace(categoryID) == "" {
			errors = append(errors, "category IDs cannot be empty")
			break
		}
	}

	// Bio validation
	if err := v.validateBio(input.Bio); err != nil {
		errors = append(errors, err.Error())
	}

	// Experience validation
	if err := v.validateExperience(input.YearsExperience); err != nil {
		errors = append(errors, err.Error())
	}

	// Languages validation
	if err := v.validateLanguages(input.Languages); err != nil {
		errors = append(errors, err.Error())
	}

	// Travel distance validation
	if err := v.validateTravelDistance(input.MaxTravelDistanceKm); err != nil {
		errors = append(errors, err.Error())
	}

	// Location validation
	if err := v.validateLocation(input.PreferredCity, input.PreferredState, input.PreferredCountry); err != nil {
		errors = append(errors, err.Error())
	}

	// Avatar URL validation
	if err := v.validateAvatarURL(input.AvatarURL); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

func (v *Validator) ValidateUpdateProfile(input *UpdateProfileInput) error {
	var errors []string

	// Check if at least one field is provided for update
	if input.CategoryIDs == nil && input.Bio == nil && input.YearsExperience == nil &&
		input.Languages == nil && input.MaxTravelDistanceKm == nil && input.AvatarURL == nil &&
		input.Coordinates == nil && input.PreferredCity == nil && input.PreferredState == nil &&
		input.PreferredCountry == nil {
		return fmt.Errorf("at least one field must be provided for update")
	}

	if input.CategoryIDs != nil {
		if len(*input.CategoryIDs) == 0 {
			errors = append(errors, "at least one service category is required")
		}
		if len(*input.CategoryIDs) > 10 {
			errors = append(errors, "maximum 10 service categories allowed")
		}

		// Validate individual category IDs are not empty
		for _, categoryID := range *input.CategoryIDs {
			if strings.TrimSpace(categoryID) == "" {
				errors = append(errors, "category IDs cannot be empty")
				break
			}
		}
	}

	if input.Bio != nil {
		if err := v.validateBio(*input.Bio); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if input.YearsExperience != nil {
		if err := v.validateExperience(*input.YearsExperience); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if input.Languages != nil {
		if err := v.validateLanguages(*input.Languages); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if input.AvatarURL != nil {
		if err := v.validateAvatarURL(*input.AvatarURL); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Private validation helpers
func (v *Validator) validateBio(bio string) error {
	if bio == "" {
		return fmt.Errorf("bio is required")
	}
	if len(bio) < 10 {
		return fmt.Errorf("bio must be at least 10 characters")
	}
	if len(bio) > 1000 {
		return fmt.Errorf("bio must be less than 1000 characters")
	}
	return nil
}

func (v *Validator) validateExperience(years int) error {
	if years < 0 {
		return fmt.Errorf("years of experience cannot be negative")
	}
	if years > 70 {
		return fmt.Errorf("years of experience cannot exceed 70")
	}
	return nil
}

func (v *Validator) validateLanguages(languages []string) error {
	if len(languages) == 0 {
		return fmt.Errorf("at least one language is required")
	}
	if len(languages) > 10 {
		return fmt.Errorf("maximum 10 languages allowed")
	}

	for _, lang := range languages {
		trimmedLang := strings.TrimSpace(lang)
		if trimmedLang == "" {
			return fmt.Errorf("languages cannot be empty")
		}
		if len(trimmedLang) < 2 {
			return fmt.Errorf("language must be at least 2 characters")
		}
		if len(trimmedLang) > 50 {
			return fmt.Errorf("language must be no more than 50 characters")
		}
	}
	return nil
}

func (v *Validator) validateTravelDistance(distance float64) error {
	if distance < 0 {
		return fmt.Errorf("travel distance cannot be negative")
	}
	if distance > 500 {
		return fmt.Errorf("travel distance cannot exceed 500km")
	}
	return nil
}

func (v *Validator) validateAvatarURL(url string) error {
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("avatar URL must start with http:// or https://")
	}
	return nil
}

func (v *Validator) validateLocation(city, state, country string) error {
	trimmedCity := strings.TrimSpace(city)
	trimmedState := strings.TrimSpace(state)
	trimmedCountry := strings.TrimSpace(country)

	if trimmedCity == "" || trimmedState == "" {
		return fmt.Errorf("preferred city and state are required")
	}
	if len(trimmedCity) < 2 || len(trimmedState) < 2 {
		return fmt.Errorf("invalid city or state")
	}
	if trimmedCountry == "" {
		return fmt.Errorf("preferred country is required")
	}
	return nil
}
