package utils

import "strings"

func SplitName(fullName string) (firstName, lastName string) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return "", ""
	}
	parts := strings.SplitN(fullName, " ", 2)
	firstName = parts[0]
	if len(parts) > 1 {
		lastName = strings.TrimSpace(parts[1])
	}
	return firstName, lastName
}

func AutoTitle(description string, maxLen int) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return "Untitled Report"
	}
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return string(runes)
	}
	return string(runes[:maxLen]) + "..."
}
