package utils

import (
	"database/sql"
	"ecosystem/database"
	"ecosystem/models"
	"regexp"
	"strings"
	"time"
)

// 500m radius for Kinshasa urban area
const defaultLatLngEpsilon = 0.0045

func DuplicateEpsilon() float64 {
	return GetEnvAsFloat("DUPLICATE_LAT_EPSILON", defaultLatLngEpsilon)
}

func DuplicateMode() string {
	return GetEnv("DUPLICATE_MODE", "auto_flag")
}

// Normalize address for comparison
func normalizeAddress(addr string) string {
	if addr == "" {
		return ""
	}
	
	addr = strings.ToLower(addr)
	
	// Remove common French street prefixes (generic)
	prefixes := []string{"avenue", "boulevard", "rue", "route", "impasse", "place", "square", "av", "bd"}
	for _, p := range prefixes {
		addr = strings.ReplaceAll(addr, p, "")
	}
	
	// Remove numbers and special chars
	addr = regexp.MustCompile(`[0-9]+`).ReplaceAllString(addr, "")
	addr = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(addr, "")
	
	// Extract last meaningful word (likely the street name)
	words := strings.Fields(addr)
	if len(words) > 0 {
		return words[len(words)-1]
	}
	
	return strings.TrimSpace(addr)
}

func FindProximityDuplicates(userID int, lat, lng float64, address, description string) ([]models.ReportSummary, error) { 
	eps := DuplicateEpsilon()
	normalizedAddr := normalizeAddress(address)
	
	// Limit description for similarity check
	descForCheck := description
	if len(descForCheck) > 200 {
		descForCheck = descForCheck[:200]
	}
	
	rows, err := database.DB.Query(`
		SELECT report_id, title, status, latitude, longitude, created_at
		FROM reports
		WHERE user_id != $1
		  AND created_at > NOW() - INTERVAL '14 days'
		  AND status NOT IN ('rejected', 'duplicate')
		  AND (
			  (latitude BETWEEN $2 AND $3 AND longitude BETWEEN $4 AND $5)
			  OR (similarity($6, description) > 0.4)
			  OR ($7 != '' AND address ILIKE '%' || $7 || '%')
		  )
		ORDER BY created_at DESC
		LIMIT 10`,
		userID, 
		lat-eps, lat+eps, 
		lng-eps, lng+eps, 
		descForCheck,
		normalizedAddr,
	)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []models.ReportSummary
	for rows.Next() {
		var r models.ReportSummary
		if err := rows.Scan(&r.ReportID, &r.Title, &r.Status, &r.Latitude, &r.Longitude, &r.CreatedAt); err != nil {
			return nil, err
		}
		matches = append(matches, r)
	}
	return matches, rows.Err()
}

func NotifyAdminsDuplicateWarning(reportID int, title string) error {
	rows, err := database.DB.Query(`
		SELECT user_id FROM users WHERE role IN ('admin', 'authority') AND status = 'active'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	msg := "A new report was flagged with a possible duplicate near a recent location: " + title
	for rows.Next() {
		var adminID int
		if err := rows.Scan(&adminID); err != nil {
			return err
		}
		_, err := database.DB.Exec(`
			INSERT INTO notifications (user_id, report_id, title, message)
			VALUES ($1, $2, 'Possible duplicate report', $3)`,
			adminID, reportID, msg,
		)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func NotifyUserReportDuplicate(userID, reportID int, title string) error {
	_, err := database.DB.Exec(`
		INSERT INTO notifications (user_id, report_id, title, message)
		VALUES ($1, $2, 'Report marked as duplicate', $3)`,
		userID, reportID,
		"Your report \""+title+"\" was marked as a duplicate by an administrator.",
	)
	return err
}

func IsTokenBlacklisted(jti string) (bool, error) {
	var exists bool
	err := database.DB.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM token_blacklist WHERE jti = $1 AND expires_at > NOW())`,
		jti,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return exists, err
}

func BlacklistToken(jti string, expiresAt time.Time) error {
	_, err := database.DB.Exec(`
		INSERT INTO token_blacklist (jti, expires_at) VALUES ($1, $2)
		ON CONFLICT (jti) DO NOTHING`,
		jti, expiresAt,
	)
	return err
}