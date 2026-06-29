package models

import "time"

type User struct {
	UserID           int        `json:"user_id"`
	Email            string     `json:"email"`
	PhoneNumber      string     `json:"phone_number"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	Role             string     `json:"role"`
	IsEmailVerified  bool       `json:"is_email_verified"`
	ReputationScore  int        `json:"reputation_score"`
	TotalReports     int        `json:"total_reports"`
	Status           string     `json:"status"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
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
	ReportID           int        `json:"report_id"`
	UserID             int        `json:"user_id"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	Category           string     `json:"category"`
	Latitude           float64    `json:"latitude"`
	Longitude          float64    `json:"longitude"`
	Address            *string    `json:"address,omitempty"`
	PhotoURLs          []string   `json:"photo_urls"`
	PhotoHashes        []string   `json:"photo_hashes,omitempty"`
	ThumbnailURLs      []string   `json:"thumbnail_urls,omitempty"`
	Status             string     `json:"status"`
	DuplicateOf        *int       `json:"duplicate_of,omitempty"`
	DuplicateOfTitle   *string    `json:"duplicate_of_title,omitempty"`
	DuplicateOfAddress *string    `json:"duplicate_of_address,omitempty"`
	DuplicateWarning   bool       `json:"duplicate_warning"`
	AdminNotes         *string    `json:"admin_notes,omitempty"`
	UpvoteCount        int        `json:"upvote_count"`
	ViewCount          int        `json:"view_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	
	// Reporter info (already exists in your model)
	ReporterFirstName  string `json:"reporter_first_name,omitempty"`
	ReporterLastName   string `json:"reporter_last_name,omitempty"`
	ReporterEmail      string `json:"reporter_email,omitempty"`
	ReporterPhoneNumber string `json:"reporter_phone_number,omitempty"`
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

//////////// New structures for admin dashboard reports /////////////
// FullReportResponse - Complete report with all data
type FullReportResponse struct {
    Report        Report        `json:"report"`
    AuditHistory  []AuditLog    `json:"audit_history"`
    Comments      []Comment     `json:"comments"`
    Statistics    ReportStatistics `json:"statistics"`
    PrintableAt   time.Time     `json:"printable_at"`
}

// ReportStatistics - Statistical data for a report
type ReportStatistics struct {
    AgeInDays         int            `json:"age_in_days"`
    TimeToResolution  int            `json:"time_to_resolution_hours,omitempty"`
    UpvoteTrend       []WeeklyUpvote `json:"upvote_trend,omitempty"`
}

// WeeklyUpvote - Upvote trend by week
type WeeklyUpvote struct {
    Week    time.Time `json:"week"`
    Upvotes int       `json:"upvotes"`
}

// DashboardStats - Admin dashboard statistics
type DashboardStats struct {
    TotalReports        int                     `json:"total_reports"`
    ByStatus            map[string]int          `json:"by_status"`
    ByCategory          map[string]int          `json:"by_category"`
    RecentReports7d     int                     `json:"recent_reports_7d"`
    TotalUsers          int                     `json:"total_users"`
    TotalUpvotes        int                     `json:"total_upvotes"`
    AvgReportsPerUser   float64                 `json:"avg_reports_per_user"`
    TopReporters        []TopReporter           `json:"top_reporters"`
    DuplicateWarnings   int                     `json:"duplicate_warnings"`
}

// TopReporter - User with most reports
type TopReporter struct {
    UserID      int    `json:"user_id"`
    Name        string `json:"name"`
    ReportCount int    `json:"report_count"`
}

// WeeklyTrend - Weekly trend data for charts
type WeeklyTrend struct {
    Week          time.Time `json:"week"`
    TotalReports  int       `json:"total_reports"`
    Investigating int       `json:"investigating"`
    Resolved      int       `json:"resolved"`
    Rejected      int       `json:"rejected"`
    TotalUpvotes  int       `json:"total_upvotes"`
}

// CategoryDistribution - Category distribution for pie chart
type CategoryDistribution struct {
    Category     string `json:"category"`
    Count        int    `json:"count"`
    TotalUpvotes int    `json:"total_upvotes"`
}



/////////////
// ReportHistory - Timeline entry
type ReportHistory struct {
    HistoryID      int       `json:"history_id"`
    ReportID       int       `json:"report_id"`
    Status         string    `json:"status"`
    ChangedBy      *int      `json:"changed_by,omitempty"`
    ChangedByName  string    `json:"changed_by_name,omitempty"`
    Notes          string    `json:"notes,omitempty"`
    CreatedAt      time.Time `json:"created_at"`
}

// PrintableReport - Complete report for printing
type PrintableReport struct {
    Report          Report            `json:"report"`
    Timeline        []ReportHistory   `json:"timeline"`
    Statistics      ReportStatistics  `json:"statistics"`
    Attachments     []Attachment      `json:"attachments"`
    GeneratedAt     time.Time         `json:"generated_at"`
    ReferenceNumber string            `json:"reference_number"`
    CaseNumber      string            `json:"case_number"`
    Recipient       string            `json:"recipient,omitempty"`
    Purpose         string            `json:"purpose,omitempty"`
    AdditionalNotes string            `json:"additional_notes,omitempty"`
}

// Attachment - File attachment
type Attachment struct {
    AttachmentID int       `json:"attachment_id"`
    ReportID     int       `json:"report_id"`
    FileName     string    `json:"file_name"`
    FilePath     string    `json:"file_path"`
    FileType     string    `json:"file_type"` // image, video, document
    FileSize     int64     `json:"file_size"`
    UploadedAt   time.Time `json:"uploaded_at"`
}

// PrintPreviewRequest - Customizable print preview
type PrintPreviewRequest struct {
    Recipient         string `json:"recipient"`
    Purpose           string `json:"purpose"`
    AdditionalNotes   string `json:"additional_notes"`
    IncludeImages     bool   `json:"include_images"`
    IncludeStatistics bool   `json:"include_statistics"`
    IncludeTimeline   bool   `json:"include_timeline"`
}

// PrintPreviewResponse - Preview data with reference numbers
type PrintPreviewResponse struct {
    Report          Report            `json:"report"`
    Statistics      ReportStatistics  `json:"statistics,omitempty"`
    Timeline        []ReportHistory   `json:"timeline,omitempty"`
    GeneratedAt     time.Time         `json:"generated_at"`
    ReferenceNumber string            `json:"reference_number"`
    CaseNumber      string            `json:"case_number"`
    QRCodeData      string            `json:"qr_code_data,omitempty"`
}


//// for admin dashboard charts
// SystemReportRequest - Request with date range
type SystemReportRequest struct {
    From string `json:"from"` // Format: 2026-01-01
    To   string `json:"to"`   // Format: 2026-12-31
}

// MonthlyTrend - Monthly data for longer reports
type MonthlyTrend struct {
    Month         string `json:"month"` // "2026-01"
    TotalReports  int    `json:"total_reports"`
    Resolved      int    `json:"resolved"`
    Investigating int    `json:"investigating"`
}