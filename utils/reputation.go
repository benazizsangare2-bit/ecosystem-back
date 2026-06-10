package utils

import "ecosystem/database"

func AdjustReputation(userID, delta int) error {
	_, err := database.DB.Exec(`
		UPDATE users SET reputation_score = GREATEST(0, reputation_score + $1), updated_at = NOW()
		WHERE user_id = $2`,
		delta, userID,
	)
	return err
}

const (
	ReputationApprovedReport = 10
	ReputationUpvoteReceived = 1
	ReputationDuplicate      = -5
)
