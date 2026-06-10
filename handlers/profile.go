package handlers

import (
	"database/sql"
	"ecosystem/database"
	"ecosystem/middleware"
	"ecosystem/models"
	"ecosystem/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var user models.User
	err := database.DB.QueryRow(`
		SELECT user_id, email, phone_number, first_name, last_name, role,
		       is_email_verified, reputation_score, total_reports, status, created_at
		FROM users WHERE user_id = $1`,
		userID,
	).Scan(
		&user.UserID, &user.Email, &user.PhoneNumber, &user.FirstName, &user.LastName,
		&user.Role, &user.IsEmailVerified, &user.ReputationScore, &user.TotalReports,
		&user.Status, &user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "User not found")
		return
	}
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	utils.Success(c, http.StatusOK, user, "Profile retrieved")
}
