package utils

import (
	"errors"
	"regexp"
	"strings"
)

// DRC mobile numbers: +243 or 0 prefix, then 9 digits starting with 8 or 9.
var drcPhoneRegex = regexp.MustCompile(`^(\+?243|0)?[89]\d{8}$`)

func NormalizeDRCPhone(input string) (string, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' {
			return -1
		}
		return r
	}, strings.TrimSpace(input))

	if cleaned == "" {
		return "", errors.New("phone number is required")
	}

	if !drcPhoneRegex.MatchString(cleaned) {
		return "", errors.New("invalid DRC phone number format (use +243XXXXXXXXX or 0XXXXXXXXX)")
	}

	switch {
	case strings.HasPrefix(cleaned, "+243"):
		return cleaned, nil
	case strings.HasPrefix(cleaned, "243"):
		return "+" + cleaned, nil
	case strings.HasPrefix(cleaned, "0"):
		return "+243" + cleaned[1:], nil
	default:
		return "+243" + cleaned, nil
	}
}
