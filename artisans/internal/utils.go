package internal

// GetActiveRole safely dereferences ActiveRole pointer
func GetActiveRole(activeRole *string) string {
	if activeRole != nil {
		return *activeRole
	}
	return ""
}
