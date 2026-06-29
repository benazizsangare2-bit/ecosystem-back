package handlers

import (
	"database/sql"
	"ecosystem/database"
	"ecosystem/middleware"
	"ecosystem/models"
	"ecosystem/utils"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	phone, err := utils.NormalizeDRCPhone(req.PhoneNumber)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	var exists bool
	if err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", req.Email).Scan(&exists); err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	if exists {
		utils.Error(c, http.StatusConflict, "Email already registered")
		return
	}

	if err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE phone_number=$1)", phone).Scan(&exists); err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	if exists {
		utils.Error(c, http.StatusConflict, "Phone number already registered")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		utils.InternalError(c, "Failed to hash password")
		return
	}

	firstName, lastName := utils.SplitName(req.Name)
	var userID int
	err = database.DB.QueryRow(`
		INSERT INTO users (email, phone_number, first_name, last_name, password_hash, role, status)
		VALUES ($1, $2, $3, $4, $5, 'citizen', 'active')
		RETURNING user_id`,
		req.Email, phone, firstName, lastName, string(hashedPassword),
	).Scan(&userID)
	if err != nil {
		utils.InternalError(c, "Failed to create user")
		return
	}

	verifyToken, err := utils.GeneratePurposeToken(userID, req.Email, "citizen", utils.PurposeEmailVerify, 24*time.Hour)
	if err != nil {
		utils.InternalError(c, "Failed to generate verification token")
		return
	}

	link := utils.FrontendBaseURL() + "/verify?token=" + verifyToken
	html := "<h1>Welcome to EnvTrack!</h1><p>Verify your email:</p><a href='" + link + "'>" + link + "</a><p>Expires in 24 hours.</p>"
	if err := utils.SendEmail(req.Email, "Verify Your Email - EnvTrack", html); err != nil {
		println("Failed to send email:", err.Error())
	}

	utils.Success(c, http.StatusCreated, gin.H{"user_id": userID}, "Registration successful. Please check your email to verify your account.")
}

func VerifyEmail(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		var req models.VerifyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, "token is required")
			return
		}
		token = req.Token
	}
	verifyEmailWithToken(c, token)
}

func verifyEmailWithToken(c *gin.Context, token string) {
	claims, err := utils.ParseToken(token)
	if err != nil || claims.Purpose != utils.PurposeEmailVerify {
		utils.BadRequest(c, "Invalid or expired verification link")
		return
	}

	result, err := database.DB.Exec(`
		UPDATE users SET is_email_verified = true, updated_at = NOW()
		WHERE user_id = $1 AND is_email_verified = false`,
		claims.UserID,
	)
	if err != nil {
		utils.InternalError(c, "Failed to verify email")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		utils.BadRequest(c, "Email already verified or user not found")
		return
	}

	utils.Success(c, http.StatusOK, nil, "Email verified successfully. You can now login.")
}

func Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	var user models.User
	var passwordHash string
	err := database.DB.QueryRow(`
		SELECT user_id, email, phone_number, first_name, last_name, role, is_email_verified,
		       reputation_score, total_reports, status, password_hash, created_at
		FROM users WHERE email = $1`,
		req.Email,
	).Scan(
		&user.UserID, &user.Email, &user.PhoneNumber, &user.FirstName, &user.LastName,
		&user.Role, &user.IsEmailVerified, &user.ReputationScore, &user.TotalReports,
		&user.Status, &passwordHash, &user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		utils.Unauthorized(c, "Invalid email or password")
		return
	}
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}

	if user.Status != "active" {
		utils.Unauthorized(c, "Account is suspended or banned")
		return
	}
	if !user.IsEmailVerified {
		utils.Unauthorized(c, "Please verify your email before logging in")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		utils.Unauthorized(c, "Invalid email or password")
		return
	}

	token, expiresAt, err := utils.GenerateAccessToken(user.UserID, user.Email, user.Role)
	if err != nil {
		utils.InternalError(c, "Failed to generate token")
		return
	}

	_, _ = database.DB.Exec("UPDATE users SET last_login = NOW() WHERE user_id = $1", user.UserID)

	utils.Success(c, http.StatusOK, models.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresAt,
		User:        user,
	}, "Login successful")
}

func Logout(c *gin.Context) {
	jti := middleware.GetJTI(c)
	if jti != "" {
		exp, ok := c.Get(middleware.ContextTokenExp)
		expiry := time.Now().Add(time.Duration(utils.GetEnvAsInt("JWT_EXPIRY_HOURS", 24)) * time.Hour)
		if ok {
			if t, ok := exp.(time.Time); ok {
				expiry = t
			}
		}
		_ = utils.BlacklistToken(jti, expiry)
	}
	utils.Success(c, http.StatusOK, nil, "Logged out successfully")
}

func ForgotPassword(c *gin.Context) {
	var req models.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	var userID int
	var role string
	err := database.DB.QueryRow(`
		SELECT user_id, role FROM users WHERE email = $1 AND status = 'active'`,
		req.Email,
	).Scan(&userID, &role)

	if err == nil {
		token, err := utils.GeneratePurposeToken(userID, req.Email, role, utils.PurposePasswordReset, time.Hour)
		if err == nil {
			link := utils.FrontendBaseURL() + "/reset-password?token=" + token
			html := "<p>Reset your password:</p><a href='" + link + "'>" + link + "</a><p>Expires in 1 hour.</p>"
			_ = utils.SendEmail(req.Email, "Password Reset - EnvTrack", html)
		}
	}

	utils.Success(c, http.StatusOK, nil, "If that email exists, a reset link has been sent.")
}

func ResetPassword(c *gin.Context) {
	var req models.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	claims, err := utils.ParseToken(req.Token)
	if err != nil || claims.Purpose != utils.PurposePasswordReset {
		utils.BadRequest(c, "Invalid or expired reset token")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.InternalError(c, "Failed to hash password")
		return
	}

	_, err = database.DB.Exec(`
		UPDATE users SET password_hash = $1, updated_at = NOW() WHERE user_id = $2`,
		string(hashed), claims.UserID,
	)
	if err != nil {
		utils.InternalError(c, "Failed to update password")
		return
	}

	utils.Success(c, http.StatusOK, nil, "Password reset successfully")
}

func DeleteAccount(c *gin.Context) {
	userID := middleware.GetUserID(c)
	jti := middleware.GetJTI(c)

	result, err := database.DB.Exec(`
		UPDATE users SET deleted_at = NOW(), status = 'deleted', updated_at = NOW()
		WHERE user_id = $1 AND deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		utils.InternalError(c, "Failed to delete account")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		utils.NotFound(c, "Account not found or already deleted")
		return
	}

	if jti != "" {
		exp, ok := c.Get(middleware.ContextTokenExp)
		expiry := time.Now().Add(time.Duration(utils.GetEnvAsInt("JWT_EXPIRY_HOURS", 24)) * time.Hour)
		if ok {
			if t, ok := exp.(time.Time); ok {
				expiry = t
			}
		}
		_ = utils.BlacklistToken(jti, expiry)
	}

	_, _ = database.DB.Exec(`
		INSERT INTO audit_logs (admin_id, action, target_type, target_id, ip_address, user_agent)
		VALUES ($1, 'delete_account', 'user', $2, $3, $4)`,
		userID, userID, c.ClientIP(), c.Request.UserAgent(),
	)

	utils.Success(c, http.StatusOK, nil, "Your account has been deactivated. Your reports are no longer publicly associated with you.")
}

func ChangePassword(c *gin.Context) {
	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	var passwordHash string
	err := database.DB.QueryRow("SELECT password_hash FROM users WHERE user_id = $1", userID).Scan(&passwordHash)
	if err != nil {
		utils.InternalError(c, "User not found")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.OldPassword)); err != nil {
		utils.BadRequest(c, "Current password is incorrect")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.InternalError(c, "Failed to hash password")
		return
	}

	_, err = database.DB.Exec("UPDATE users SET password_hash = $1, updated_at = NOW() WHERE user_id = $2", string(hashed), userID)
	if err != nil {
		utils.InternalError(c, "Failed to update password")
		return
	}

	utils.Success(c, http.StatusOK, nil, "Password changed successfully")
}
