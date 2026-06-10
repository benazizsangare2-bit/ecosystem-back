package models

import "time"

type User struct {
	UserID           int       `json:"user_id"`
	Email            string    `json:"email"`
	PhoneNumber      string    `json:"phone_number"`
	FirstName        string    `json:"first_name"`
	LastName         string    `json:"last_name"`
	Role             string    `json:"role"`
	IsEmailVerified  bool      `json:"is_email_verified"`
	ReputationScore  int       `json:"reputation_score"`
	TotalReports     int       `json:"total_reports"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Name        string `json:"name" binding:"required"`
	PhoneNumber string `json:"phone_number" binding:"required"`
	Password    string `json:"password" binding:"required,min=6"`
}

type VerifyRequest struct {
	Token string `json:"token" binding:"required"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	User        User   `json:"user"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type Report struct {
	ReportID         int       `json:"report_id"`
	UserID           int       `json:"user_id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Category         string    `json:"category"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Address          *string   `json:"address,omitempty"`
	PhotoURLs        []string  `json:"photo_urls"`
	ThumbnailURLs    []string  `json:"thumbnail_urls,omitempty"`
	Status           string    `json:"status"`
	DuplicateOf      *int      `json:"duplicate_of,omitempty"`
	DuplicateWarning bool      `json:"duplicate_warning"`
	AdminNotes       *string   `json:"admin_notes,omitempty"`
	UpvoteCount      int       `json:"upvote_count"`
	ViewCount        int       `json:"view_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ReportSummary struct {
	ReportID  int       `json:"report_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateReportResponse struct {
	Report              Report          `json:"report"`
	PossibleDuplicates  []ReportSummary `json:"possible_duplicates,omitempty"`
	DuplicateWarning    bool            `json:"duplicate_warning"`
}

type UpdateReportRequest struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	Category    *string  `json:"category"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Address     *string  `json:"address"`
}

type AdminStatusRequest struct {
	Status      string `json:"status" binding:"required"`
	AdminNotes  string `json:"admin_notes"`
	DuplicateOf *int   `json:"duplicate_of"`
}

type CommentRequest struct {
	Content string `json:"content" binding:"required"`
}

type Comment struct {
	CommentID          int       `json:"comment_id"`
	ReportID           int       `json:"report_id"`
	UserID             int       `json:"user_id"`
	AuthorName         string    `json:"author_name"`
	Comment            string    `json:"comment"`
	IsOfficialResponse bool      `json:"is_official_response"`
	CreatedAt          time.Time `json:"created_at"`
}

type AdminStats struct {
	TotalReports      int            `json:"total_reports"`
	ByStatus          map[string]int   `json:"by_status"`
	ByCategory        map[string]int   `json:"by_category"`
	RecentReports     int            `json:"recent_reports_7d"`
	DuplicateWarnings int            `json:"duplicate_warnings"`
}


type AuditLog struct {
    LogID      int       `json:"log_id"`
    AdminID    int       `json:"admin_id"`
    AdminName  string    `json:"admin_name"`  // Joined from users table
    Action     string    `json:"action"`
    TargetType string    `json:"target_type"`
    TargetID   int       `json:"target_id"`
    OldData    string    `json:"old_data,omitempty"`
    NewData    string    `json:"new_data,omitempty"`
    IPAddress  string    `json:"ip_address"`
    UserAgent  string    `json:"user_agent"`
    CreatedAt  time.Time `json:"created_at"`
}

var ValidCategories = map[string]bool{
	"illegal_dumping":      true,
	"overflowing_waste":    true,
	"air_pollution":        true,
	"water_contamination":  true,
	"noise_pollution":      true,
	"deforestation":        true,
	"bad_roads":            true,
	"other":                true,
}

var ValidReportStatuses = map[string]bool{
	"pending":        true,
	"under_review":   true,
	"investigating":  true,
	"resolved":       true,
	"rejected":       true,
	"duplicate":      true,
}
