package handlers

import (
	"database/sql"
	"ecosystem/database"
	"ecosystem/middleware"
	"ecosystem/models"
	"ecosystem/utils"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/signintech/gopdf"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func uploadDir() string {
	return utils.GetEnv("UPLOAD_DIR", "./uploads")
}

func CreateReport(c *gin.Context) {
	userID := middleware.GetUserID(c)

	description := strings.TrimSpace(c.PostForm("description"))
	category := strings.TrimSpace(c.PostForm("category"))
	latStr := c.PostForm("latitude")
	lngStr := c.PostForm("longitude")
	address := strings.TrimSpace(c.PostForm("address"))

	if description == "" || category == "" || latStr == "" || lngStr == "" {
		utils.BadRequest(c, "description, category, latitude, and longitude are required")
		return
	}
	if !models.ValidCategories[category] {
		utils.BadRequest(c, "invalid category")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		utils.BadRequest(c, "invalid latitude")
		return
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		utils.BadRequest(c, "invalid longitude")
		return
	}

	file, err := c.FormFile("photo")
	if err != nil {
		utils.BadRequest(c, "photo is required")
		return
	}

	photoPath, thumbPath, err := utils.SaveReportPhoto(file, uploadDir())
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	mode := utils.DuplicateMode()
	var possibleDupes []models.ReportSummary
	duplicateWarning := false

	if mode != "manual" {
		possibleDupes, err = utils.FindProximityDuplicates(userID, lat, lng, description, address)
		if err != nil {
			utils.InternalError(c, "Duplicate check failed")
			return
		}
		if len(possibleDupes) > 0 && mode == "auto_reject" {
			os.Remove(photoPath)
			os.Remove(thumbPath)
			utils.Error(c, http.StatusBadRequest, "A similar report exists nearby. Submission rejected.")
			return
		}
		if len(possibleDupes) > 0 {
			duplicateWarning = true
		}
	}

	title := utils.AutoTitle(description, 80)
	var addrPtr *string
	if address != "" {
		addrPtr = &address
	}

	var reportID int
	err = database.DB.QueryRow(`
		INSERT INTO reports (user_id, title, description, category, latitude, longitude, address,
		                     photo_urls, thumbnail_urls, status, duplicate_warning)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10)
		RETURNING report_id`,
		userID, title, description, category, lat, lng, addrPtr,
		pq.Array([]string{toPublicPath(photoPath)}),
		pq.Array([]string{toPublicPath(thumbPath)}),
		duplicateWarning,
	).Scan(&reportID)
	if err != nil {
		os.Remove(photoPath)
		os.Remove(thumbPath)
		utils.InternalError(c, "Failed to save report")
		return
	}

	_, _ = database.DB.Exec("UPDATE users SET total_reports = total_reports + 1, updated_at = NOW() WHERE user_id = $1", userID)

	if duplicateWarning {
		_ = utils.NotifyAdminsDuplicateWarning(reportID, title)
	}

	report, err := fetchReportByID(reportID)
	if err != nil {
		utils.InternalError(c, "Report created but could not be loaded")
		return
	}

	utils.Success(c, http.StatusCreated, models.CreateReportResponse{
		Report:             report,
		PossibleDuplicates: possibleDupes,
		DuplicateWarning:   duplicateWarning,
	}, "Report submitted successfully")
}

func GetUserReports(c *gin.Context) {
	userID := middleware.GetUserID(c)
	rows, err := database.DB.Query(`
		SELECT r.report_id, r.user_id, r.title, r.description, r.category,
		       r.latitude, r.longitude, r.address,
		       r.photo_urls, r.thumbnail_urls, r.status, r.duplicate_of,
		       r.duplicate_warning, r.admin_notes,
		       r.upvote_count, r.view_count, r.created_at, r.updated_at,
		       orig.title, orig.address,
		       u.first_name, u.last_name, u.email, u.phone_number,
		       u.deleted_at
		FROM reports r
		LEFT JOIN reports orig ON r.duplicate_of = orig.report_id
		LEFT JOIN users u ON r.user_id = u.user_id
		WHERE r.user_id = $1 ORDER BY r.created_at DESC`,
		userID,
	)
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	defer rows.Close()

	reports, err := scanReports(rows)
	if err != nil {
		utils.InternalError(c, "Failed to load reports")
		return
	}
	utils.Success(c, http.StatusOK, reports, "User reports retrieved")
}

func GetReport(c *gin.Context) {
	userID := middleware.GetUserID(c)
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	report, err := fetchReportByID(reportID)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "Report not found")
		return
	}
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}

	role := middleware.GetRole(c)
	if report.UserID != userID && role != "admin" && role != "authority" {
		utils.Forbidden(c, "You can only view your own reports here")
		return
	}

	utils.Success(c, http.StatusOK, report, "Report retrieved")
}

func UpdateReport(c *gin.Context) {
	userID := middleware.GetUserID(c)
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	var req models.UpdateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	var ownerID int
	var status string
	var description string
	err = database.DB.QueryRow(`
		SELECT user_id, status, description FROM reports WHERE report_id = $1`,
		reportID,
	).Scan(&ownerID, &status, &description)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "Report not found")
		return
	}
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	role := middleware.GetRole(c)
	if ownerID != userID && role != "admin" && role != "authority" {
		utils.Forbidden(c, "Not your report")
		return
	}
	if status != "pending" && role != "admin" && role != "authority" {
		utils.BadRequest(c, "Only pending reports can be edited")
		return
	}

	title := ""
	if req.Title != nil {
		title = *req.Title
	} else if req.Description != nil {
		title = utils.AutoTitle(*req.Description, 80)
	} else {
		title = utils.AutoTitle(description, 80)
	}

	desc := description
	if req.Description != nil {
		desc = *req.Description
	}
	cat := ""
	if req.Category != nil {
		if !models.ValidCategories[*req.Category] {
			utils.BadRequest(c, "invalid category")
			return
		}
		cat = *req.Category
	}

	_, err = database.DB.Exec(`
		UPDATE reports SET
			title = COALESCE(NULLIF($1, ''), title),
			description = COALESCE(NULLIF($2, ''), description),
			category = COALESCE(NULLIF($3, ''), category),
			latitude = COALESCE($4, latitude),
			longitude = COALESCE($5, longitude),
			address = COALESCE($6, address),
			updated_at = NOW()
		WHERE report_id = $7`,
		title, desc, cat, req.Latitude, req.Longitude, req.Address, reportID,
	)
	if err != nil {
		utils.InternalError(c, "Failed to update report")
		return
	}

	report, _ := fetchReportByID(reportID)
	utils.Success(c, http.StatusOK, report, "Report updated")
}

func DeleteReport(c *gin.Context) {
	userID := middleware.GetUserID(c)
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	var ownerID int
	var status string
	err = database.DB.QueryRow(`SELECT user_id, status FROM reports WHERE report_id = $1`, reportID).Scan(&ownerID, &status)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "Report not found")
		return
	}
	role := middleware.GetRole(c)
	if ownerID != userID && role != "admin" && role != "authority" {
		utils.Forbidden(c, "Not your report")
		return
	}
	if status != "pending" && role != "admin" && role != "authority" {
		utils.BadRequest(c, "Only pending reports can be deleted")
		return
	}

	_, err = database.DB.Exec("DELETE FROM reports WHERE report_id = $1", reportID)
	if err != nil {
		utils.InternalError(c, "Failed to delete report")
		return
	}
	utils.Success(c, http.StatusOK, nil, "Report deleted")
}

func GetPublicReports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	rows, err := database.DB.Query(`
		SELECT r.report_id, r.user_id, r.title, r.description, r.category,
		       r.latitude, r.longitude, r.address,
		       r.photo_urls, r.thumbnail_urls, r.status, r.duplicate_of,
		       r.duplicate_warning, r.admin_notes,
		       r.upvote_count, r.view_count, r.created_at, r.updated_at,
		       orig.title, orig.address,
		       u.first_name, u.last_name, u.email, u.phone_number,
		       u.deleted_at
		FROM reports r
		LEFT JOIN reports orig ON r.duplicate_of = orig.report_id
		LEFT JOIN users u ON r.user_id = u.user_id
		WHERE r.status = 'investigating'
		ORDER BY r.created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	defer rows.Close()

	reports, err := scanReports(rows)
	if err != nil {
		utils.InternalError(c, "Failed to load reports")
		return
	}

	var total int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM reports WHERE status = 'investigating'`).Scan(&total)

	utils.Success(c, http.StatusOK, gin.H{
		"reports": reports,
		"page":    page,
		"limit":   limit,
		"total":   total,
	}, "Public reports retrieved")
}

func ToggleLike(c *gin.Context) {
	userID := middleware.GetUserID(c)
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	var status string
	var ownerID int
	err = database.DB.QueryRow(`SELECT status, user_id FROM reports WHERE report_id = $1`, reportID).Scan(&status, &ownerID)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "Report not found")
		return
	}
	if status != "investigating" {
		utils.BadRequest(c, "Likes are only allowed on public investigating reports")
		return
	}

	var exists bool
	_ = database.DB.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM report_upvotes WHERE report_id = $1 AND user_id = $2)`,
		reportID, userID,
	).Scan(&exists)

	liked := false
	if exists {
		_, _ = database.DB.Exec(`DELETE FROM report_upvotes WHERE report_id = $1 AND user_id = $2`, reportID, userID)
		_, _ = database.DB.Exec(`UPDATE reports SET upvote_count = GREATEST(0, upvote_count - 1) WHERE report_id = $1`, reportID)
		_, _ = database.DB.Exec(`UPDATE users SET total_upvotes_received = GREATEST(0, total_upvotes_received - 1) WHERE user_id = $1`, ownerID)
	} else {
		_, err = database.DB.Exec(`INSERT INTO report_upvotes (report_id, user_id) VALUES ($1, $2)`, reportID, userID)
		if err != nil {
			utils.InternalError(c, "Failed to like report")
			return
		}
		_, _ = database.DB.Exec(`UPDATE reports SET upvote_count = upvote_count + 1 WHERE report_id = $1`, reportID)
		_, _ = database.DB.Exec(`UPDATE users SET total_upvotes_received = total_upvotes_received + 1 WHERE user_id = $1`, ownerID)
		_ = utils.AdjustReputation(ownerID, utils.ReputationUpvoteReceived)
		liked = true
	}

	var count int
	_ = database.DB.QueryRow(`SELECT upvote_count FROM reports WHERE report_id = $1`, reportID).Scan(&count)
	utils.Success(c, http.StatusOK, gin.H{"liked": liked, "upvote_count": count}, "Like updated")
}

func AddComment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	var req models.CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	var status string
	err = database.DB.QueryRow(`SELECT status FROM reports WHERE report_id = $1`, reportID).Scan(&status)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "Report not found")
		return
	}
	if status != "investigating" {
		utils.BadRequest(c, "Comments are only allowed on public investigating reports")
		return
	}

	var commentID int
	err = database.DB.QueryRow(`
		INSERT INTO report_comments (report_id, user_id, comment)
		VALUES ($1, $2, $3) RETURNING comment_id`,
		reportID, userID, req.Content,
	).Scan(&commentID)
	if err != nil {
		utils.InternalError(c, "Failed to add comment")
		return
	}

	comment, _ := fetchComment(commentID)
	utils.Success(c, http.StatusCreated, comment, "Comment added")
}

func GetComments(c *gin.Context) {
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	rows, err := database.DB.Query(`
		SELECT c.comment_id, c.report_id, c.user_id,
		       COALESCE(u.first_name || ' ' || NULLIF(u.last_name, ''), u.first_name),
		       c.comment, c.is_official_response, c.created_at
		FROM report_comments c
		JOIN users u ON u.user_id = c.user_id
		WHERE c.report_id = $1 ORDER BY c.created_at ASC`,
		reportID,
	)
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var cm models.Comment
		if err := rows.Scan(&cm.CommentID, &cm.ReportID, &cm.UserID, &cm.AuthorName,
			&cm.Comment, &cm.IsOfficialResponse, &cm.CreatedAt); err != nil {
			utils.InternalError(c, "Failed to load comments")
			return
		}
		comments = append(comments, cm)
	}
	utils.Success(c, http.StatusOK, comments, "Comments retrieved")
}

func fetchReportByID(reportID int) (models.Report, error) {
	row := database.DB.QueryRow(`
		SELECT r.report_id, r.user_id, r.title, r.description, r.category,
		       r.latitude, r.longitude, r.address,
		       r.photo_urls, r.thumbnail_urls, r.status, r.duplicate_of,
		       r.duplicate_warning, r.admin_notes,
		       r.upvote_count, r.view_count, r.created_at, r.updated_at,
		       orig.title, orig.address,
		       u.first_name, u.last_name, u.email, u.phone_number,
		       u.deleted_at
		FROM reports r
		LEFT JOIN reports orig ON r.duplicate_of = orig.report_id
		LEFT JOIN users u ON r.user_id = u.user_id
		WHERE r.report_id = $1`, reportID)
	return scanReport(row)
}

func scanReport(row *sql.Row) (models.Report, error) {
	var r models.Report
	var photos, thumbs pq.StringArray
	var dupOf sql.NullInt64
	var adminNotes, address, dupTitle, dupAddress sql.NullString
	var repFN, repLN, repEmail, repPhone sql.NullString
	var deletedAt sql.NullTime

	err := row.Scan(
		&r.ReportID, &r.UserID, &r.Title, &r.Description, &r.Category,
		&r.Latitude, &r.Longitude, &address, &photos, &thumbs, &r.Status,
		&dupOf, &r.DuplicateWarning, &adminNotes, &r.UpvoteCount, &r.ViewCount,
		&r.CreatedAt, &r.UpdatedAt,
		&dupTitle, &dupAddress,
		&repFN, &repLN, &repEmail, &repPhone,
		&deletedAt,
	)
	if err != nil {
		return r, err
	}
	r.PhotoURLs = []string(photos)
	r.ThumbnailURLs = []string(thumbs)
	if dupOf.Valid {
		v := int(dupOf.Int64)
		r.DuplicateOf = &v
	}
	if dupTitle.Valid {
		r.DuplicateOfTitle = &dupTitle.String
	}
	if dupAddress.Valid {
		r.DuplicateOfAddress = &dupAddress.String
	}
	if adminNotes.Valid {
		r.AdminNotes = &adminNotes.String
	}
	if address.Valid {
		r.Address = &address.String
	}
	if deletedAt.Valid {
		r.ReporterFirstName = "User account"
		r.ReporterLastName = "deactivated"
		r.ReporterEmail = ""
		r.ReporterPhoneNumber = ""
	} else {
		if repFN.Valid {
			r.ReporterFirstName = repFN.String
		}
		if repLN.Valid {
			r.ReporterLastName = repLN.String
		}
		if repEmail.Valid {
			r.ReporterEmail = repEmail.String
		}
		if repPhone.Valid {
			r.ReporterPhoneNumber = repPhone.String
		}
	}
	return r, nil
}

func scanReports(rows *sql.Rows) ([]models.Report, error) {
	var reports []models.Report
	for rows.Next() {
		var r models.Report
		var photos, thumbs pq.StringArray
		var dupOf sql.NullInt64
		var adminNotes, address, dupTitle, dupAddress sql.NullString
		var repFN, repLN, repEmail, repPhone sql.NullString
		var deletedAt sql.NullTime
		err := rows.Scan(
			&r.ReportID, &r.UserID, &r.Title, &r.Description, &r.Category,
			&r.Latitude, &r.Longitude, &address, &photos, &thumbs, &r.Status,
			&dupOf, &r.DuplicateWarning, &adminNotes, &r.UpvoteCount, &r.ViewCount,
			&r.CreatedAt, &r.UpdatedAt,
			&dupTitle, &dupAddress,
			&repFN, &repLN, &repEmail, &repPhone,
			&deletedAt,
		)
		if err != nil {
			return nil, err
		}
		r.PhotoURLs = []string(photos)
		r.ThumbnailURLs = []string(thumbs)
		if dupOf.Valid {
			v := int(dupOf.Int64)
			r.DuplicateOf = &v
		}
		if dupTitle.Valid {
			r.DuplicateOfTitle = &dupTitle.String
		}
		if dupAddress.Valid {
			r.DuplicateOfAddress = &dupAddress.String
		}
		if adminNotes.Valid {
			r.AdminNotes = &adminNotes.String
		}
		if address.Valid {
			r.Address = &address.String
		}
		if deletedAt.Valid {
			r.ReporterFirstName = "User account"
			r.ReporterLastName = "deactivated"
			r.ReporterEmail = ""
			r.ReporterPhoneNumber = ""
		} else {
			if repFN.Valid {
				r.ReporterFirstName = repFN.String
			}
			if repLN.Valid {
				r.ReporterLastName = repLN.String
			}
			if repEmail.Valid {
				r.ReporterEmail = repEmail.String
			}
			if repPhone.Valid {
				r.ReporterPhoneNumber = repPhone.String
			}
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func fetchComment(commentID int) (models.Comment, error) {
	var cm models.Comment
	err := database.DB.QueryRow(`
		SELECT c.comment_id, c.report_id, c.user_id,
		       COALESCE(u.first_name || ' ' || NULLIF(u.last_name, ''), u.first_name),
		       c.comment, c.is_official_response, c.created_at
		FROM report_comments c
		JOIN users u ON u.user_id = c.user_id
		WHERE c.comment_id = $1`, commentID,
	).Scan(&cm.CommentID, &cm.ReportID, &cm.UserID, &cm.AuthorName,
		&cm.Comment, &cm.IsOfficialResponse, &cm.CreatedAt)
	return cm, err
}

func toPublicPath(absPath string) string {
	base := uploadDir()
	rel, err := filepath.Rel(base, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	return filepath.ToSlash(rel)
}


//////////////////
// GetReportData - Returns complete report data for printing/export
func GetReportData(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    // Get user role from token
    userRole, _ := c.Get("user_role")
    userID, _ := c.Get("user_id")

    // Build query based on role
    query := `
        SELECT 
            r.report_id,
            r.user_id,
            r.title,
            r.description,
            r.category,
            r.latitude,
            r.longitude,
            r.address,
            r.photo_urls,
            r.photo_hashes,
            r.status,
            r.duplicate_of,
            r.admin_notes,
            r.upvote_count,
            r.view_count,
            r.created_at,
            r.updated_at,
            r.resolved_at,
            u.email as user_email,
            u.phone_number as user_phone,
            u.first_name as user_first_name,
            u.last_name as user_last_name,
            u.reputation_score as user_reputation,
            u.total_reports as user_total_reports,
            (SELECT COUNT(*) FROM report_upvotes WHERE report_id = r.report_id) as upvotes,
            (SELECT COUNT(*) FROM report_comments WHERE report_id = r.report_id) as comment_count,
            dup.title as duplicate_title,
            dup.address as duplicate_address
        FROM reports r
        JOIN users u ON r.user_id = u.user_id
        LEFT JOIN reports dup ON r.duplicate_of = dup.report_id
        WHERE r.report_id = $1
    `

    // If user is not admin, they can only view their own reports
    if userRole != "admin" && userRole != "authority" {
        query += " AND r.user_id = $2"
    }

    var report models.Report
    // Execute query
    var rows *sql.Rows
    if userRole == "admin" || userRole == "authority" {
        rows, err = database.DB.Query(query, reportID)
    } else {
        rows, err = database.DB.Query(query, reportID, userID)
    }

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
        return
    }
    defer rows.Close()

    if !rows.Next() {
        c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
        return
    }

    // Scan report data
    err = rows.Scan(
        &report.ReportID,
        &report.UserID,
        &report.Title,
        &report.Description,
        &report.Category,
        &report.Latitude,
        &report.Longitude,
        &report.Address,
        &report.PhotoURLs,
        &report.Status,
        &report.DuplicateOf,
        &report.AdminNotes,
        &report.UpvoteCount,
        &report.ViewCount,
        &report.CreatedAt,
        &report.UpdatedAt,
        &report.ResolvedAt,
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse report"})
        return
    }

    // Fetch complete audit history for this report
    auditLogs, _ := getReportAuditHistory(reportID)

    // Fetch comments
    comments, _ := getReportComments(reportID)

    // Generate statistics
    stats, _ := getReportStatistics(reportID)

    // Prepare full response
    response := models.FullReportResponse{
        Report:        report,
        AuditHistory:  auditLogs,
        Comments:      comments,
        Statistics:    stats,
        PrintableAt:   time.Now(),
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    response,
    })
}

// GetReportStatistics - Generate statistics for a report
func GetReportStatistics(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    stats, err := getReportStatistics(reportID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate statistics"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    stats,
    })
}

// GetDashboardStats - Admin dashboard statistics with charts
func GetDashboardStats(c *gin.Context) {
    // Check if user is admin
    userRole, _ := c.Get("user_role")
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    stats, err := getDashboardStatistics()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch statistics"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    stats,
    })
}

// GetWeeklyTrends - Weekly report trends for charts
func GetWeeklyTrends(c *gin.Context) {
    // Check if user is admin
    userRole, _ := c.Get("user_role")
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    // Get last 12 weeks of data
    rows, err := database.DB.Query(`
        SELECT 
            DATE_TRUNC('week', created_at) as week,
            COUNT(*) as total_reports,
            COUNT(CASE WHEN status = 'investigating' THEN 1 END) as investigating,
            COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved,
            COUNT(CASE WHEN status = 'rejected' THEN 1 END) as rejected,
            COALESCE(SUM(upvote_count), 0) as total_upvotes
        FROM reports
        WHERE created_at > NOW() - INTERVAL '12 weeks'
        GROUP BY DATE_TRUNC('week', created_at)
        ORDER BY week ASC
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch trends"})
        return
    }
    defer rows.Close()

    var trends []models.WeeklyTrend
    for rows.Next() {
        var t models.WeeklyTrend
        rows.Scan(
            &t.Week,
            &t.TotalReports,
            &t.Investigating,
            &t.Resolved,
            &t.Rejected,
            &t.TotalUpvotes,
        )
        trends = append(trends, t)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    trends,
    })
}

// GetCategoryDistribution - Category distribution for charts
func GetCategoryDistribution(c *gin.Context) {
    // Check if user is admin
    userRole, _ := c.Get("user_role")
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    rows, err := database.DB.Query(`
        SELECT 
            category,
            COUNT(*) as count,
            COALESCE(SUM(upvote_count), 0) as total_upvotes
        FROM reports
        WHERE created_at > NOW() - INTERVAL '30 days'
        GROUP BY category
        ORDER BY count DESC
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch distribution"})
        return
    }
    defer rows.Close()

    var distribution []models.CategoryDistribution
    for rows.Next() {
        var d models.CategoryDistribution
        rows.Scan(&d.Category, &d.Count, &d.TotalUpvotes)
        distribution = append(distribution, d)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    distribution,
    })
}

// ------------------- Helper Functions -------------------

func getReportAuditHistory(reportID int) ([]models.AuditLog, error) {
    rows, err := database.DB.Query(`
        SELECT 
            al.log_id,
            al.admin_id,
            u.first_name || ' ' || u.last_name as admin_name,
            al.action,
            al.target_type,
            al.target_id,
            al.old_data,
            al.new_data,
            al.ip_address,
            al.user_agent,
            al.created_at
        FROM audit_logs al
        JOIN users u ON al.admin_id = u.user_id
        WHERE al.target_type = 'report' AND al.target_id = $1
        ORDER BY al.created_at DESC
    `, reportID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var logs []models.AuditLog
    for rows.Next() {
        var log models.AuditLog
        var oldData, newData sql.NullString
        rows.Scan(
            &log.LogID,
            &log.AdminID,
            &log.AdminName,
            &log.Action,
            &log.TargetType,
            &log.TargetID,
            &oldData,
            &newData,
            &log.IPAddress,
            &log.UserAgent,
            &log.CreatedAt,
        )
        if oldData.Valid {
            log.OldData = oldData.String
        }
        if newData.Valid {
            log.NewData = newData.String
        }
        logs = append(logs, log)
    }
    return logs, nil
}

func getReportComments(reportID int) ([]models.Comment, error) {
    rows, err := database.DB.Query(`
        SELECT 
            c.comment_id,
            c.report_id,
            c.user_id,
            u.first_name || ' ' || u.last_name as author_name,
            c.comment,
            c.is_official_response,
            c.created_at
        FROM report_comments c
        JOIN users u ON c.user_id = u.user_id
        WHERE c.report_id = $1
        ORDER BY c.created_at ASC
    `, reportID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var comments []models.Comment
    for rows.Next() {
        var cmt models.Comment
        rows.Scan(
            &cmt.CommentID,
            &cmt.ReportID,
            &cmt.UserID,
            &cmt.AuthorName,
            &cmt.Comment,
            &cmt.IsOfficialResponse,
            &cmt.CreatedAt,
        )
        comments = append(comments, cmt)
    }
    return comments, nil
}

func getReportStatistics(reportID int) (models.ReportStatistics, error) {
    var stats models.ReportStatistics

    // Get age of report in days
    var createdAt time.Time
    database.DB.QueryRow(`
        SELECT created_at FROM reports WHERE report_id = $1
    `, reportID).Scan(&createdAt)
    stats.AgeInDays = int(time.Since(createdAt).Hours() / 24)

    // Get weekly upvote trend
    rows, err := database.DB.Query(`
        SELECT 
            DATE_TRUNC('week', created_at) as week,
            COUNT(*) as upvotes
        FROM report_upvotes
        WHERE report_id = $1
        GROUP BY DATE_TRUNC('week', created_at)
        ORDER BY week ASC
    `, reportID)
    if err == nil {
        defer rows.Close()
        for rows.Next() {
            var week time.Time
            var count int
            rows.Scan(&week, &count)
            stats.UpvoteTrend = append(stats.UpvoteTrend, models.WeeklyUpvote{
                Week:    week,
                Upvotes: count,
            })
        }
    }

    // Get time to resolution (if resolved)
    var resolvedAt sql.NullTime
    database.DB.QueryRow(`
        SELECT resolved_at FROM reports WHERE report_id = $1 AND status = 'resolved'
    `, reportID).Scan(&resolvedAt)
    if resolvedAt.Valid {
        stats.TimeToResolution = int(resolvedAt.Time.Sub(createdAt).Hours())
    }

    return stats, nil
}

func getDashboardStatistics() (models.DashboardStats, error) {
    var stats models.DashboardStats

    // Total reports
    database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
    `).Scan(&stats.TotalReports)

    // Reports by status
    rows, err := database.DB.Query(`
        SELECT status, COUNT(*) as count
        FROM reports
        GROUP BY status
    `)
    if err == nil {
        defer rows.Close()
        stats.ByStatus = make(map[string]int)
        for rows.Next() {
            var status string
            var count int
            rows.Scan(&status, &count)
            stats.ByStatus[status] = count
        }
    }

    // Reports by category
    rows, err = database.DB.Query(`
        SELECT category, COUNT(*) as count
        FROM reports
        GROUP BY category
    `)
    if err == nil {
        defer rows.Close()
        stats.ByCategory = make(map[string]int)
        for rows.Next() {
            var category string
            var count int
            rows.Scan(&category, &count)
            stats.ByCategory[category] = count
        }
    }

    // Recent reports (last 7 days)
    database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports WHERE created_at > NOW() - INTERVAL '7 days'
    `).Scan(&stats.RecentReports7d)

    // Total users
    database.DB.QueryRow(`
        SELECT COUNT(*) FROM users WHERE status = 'active' AND deleted_at IS NULL
    `).Scan(&stats.TotalUsers)

    // Total upvotes
    database.DB.QueryRow(`
        SELECT COALESCE(SUM(upvote_count), 0) FROM reports
    `).Scan(&stats.TotalUpvotes)

    // Average reports per user
    var avg float64
    database.DB.QueryRow(`
        SELECT COALESCE(AVG(report_count), 0) 
        FROM (SELECT COUNT(*) as report_count FROM reports GROUP BY user_id) as user_reports
    `).Scan(&avg)
    stats.AvgReportsPerUser = avg

    // Top reporters
    rows, err = database.DB.Query(`
        SELECT 
            u.user_id,
            u.first_name || ' ' || u.last_name as name,
            COUNT(r.report_id) as report_count
        FROM users u
        JOIN reports r ON u.user_id = r.user_id
        WHERE u.deleted_at IS NULL
        GROUP BY u.user_id, u.first_name, u.last_name
        ORDER BY report_count DESC
        LIMIT 10
    `)
    if err == nil {
        defer rows.Close()
        for rows.Next() {
            var reporter models.TopReporter
            rows.Scan(&reporter.UserID, &reporter.Name, &reporter.ReportCount)
            stats.TopReporters = append(stats.TopReporters, reporter)
        }
    }

    // Total duplicate warnings
    database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports WHERE duplicate_warning = true
    `).Scan(&stats.DuplicateWarnings)

    return stats, nil
}

///////////////////
// GetReportHistory - Get timeline of status changes
func GetReportHistory(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    // Check if user has access
    userID, _ := c.Get("user_id")
    userRole, _ := c.Get("user_role")
    
    // Verify report exists and user has access
    var reportUserID int
    database.DB.QueryRow(`
        SELECT user_id FROM reports WHERE report_id = $1
    `, reportID).Scan(&reportUserID)
    
    if userRole != "admin" && userRole != "authority" && reportUserID != userID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
        return
    }

    rows, err := database.DB.Query(`
        SELECT 
            history_id,
            report_id,
            status,
            changed_by,
            COALESCE(u.first_name || ' ' || u.last_name, 'System') as changed_by_name,
            notes,
            created_at
        FROM report_history rh
        LEFT JOIN users u ON rh.changed_by = u.user_id
        WHERE report_id = $1
        ORDER BY created_at ASC
    `, reportID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch history"})
        return
    }
    defer rows.Close()

    var history []models.ReportHistory
    for rows.Next() {
        var h models.ReportHistory
        var changedBy sql.NullInt64
        err := rows.Scan(
            &h.HistoryID,
            &h.ReportID,
            &h.Status,
            &changedBy,
            &h.ChangedByName,
            &h.Notes,
            &h.CreatedAt,
        )
        if err != nil {
            continue
        }
        if changedBy.Valid {
            id := int(changedBy.Int64)
            h.ChangedBy = &id
        }
        history = append(history, h)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    history,
    })
}

// GetAttachments - Get all attachments for a report
func GetAttachments(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    // Check access
    userID, _ := c.Get("user_id")
    userRole, _ := c.Get("user_role")
    
    var reportUserID int
    database.DB.QueryRow(`
        SELECT user_id FROM reports WHERE report_id = $1
    `, reportID).Scan(&reportUserID)
    
    if userRole != "admin" && userRole != "authority" && reportUserID != userID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
        return
    }

    // Get photos from reports table
    var photoUrls []string
    database.DB.QueryRow(`
        SELECT photo_urls FROM reports WHERE report_id = $1
    `, reportID).Scan(&photoUrls)

    attachments := []models.Attachment{}
    for i, url := range photoUrls {
        attachments = append(attachments, models.Attachment{
            AttachmentID: i + 1,
            ReportID:     reportID,
            FileName:     url,
            FilePath:     "/uploads/" + url,
            FileType:     "image",
            UploadedAt:   time.Now(),
        })
    }

    // Check for additional attachments table (if it exists)
    // If you have a separate attachments table, query it here

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    attachments,
    })
}

// GetPrintableReport - Full report data with everything for printing
func GetPrintableReport(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    // Check access
    userID, _ := c.Get("user_id")
    userRole, _ := c.Get("user_role")
    
    var reportUserID int
    database.DB.QueryRow(`
        SELECT user_id FROM reports WHERE report_id = $1
    `, reportID).Scan(&reportUserID)
    
    if userRole != "admin" && userRole != "authority" && reportUserID != userID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
        return
    }

    // Get full report data
    report, err := getCompleteReport(reportID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
        return
    }

    // Get history
    history, _ := getReportHistoryData(reportID)

    // Get statistics
    stats, _ := getReportStatistics(reportID)

    // Get attachments
    attachments, _ := getReportAttachments(reportID)

    // Generate case/reference numbers
    caseNumber := fmt.Sprintf("ET-%d-%s", reportID, time.Now().Format("2006-01-02"))
    referenceNumber := fmt.Sprintf("REF-%d-%d", reportID, time.Now().UnixNano()%10000)

    printable := models.PrintableReport{
        Report:          report,
        Timeline:        history,
        Statistics:      stats,
        Attachments:     attachments,
        GeneratedAt:     time.Now(),
        ReferenceNumber: referenceNumber,
        CaseNumber:      caseNumber,
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    printable,
    })
}

// PrintPreview - Generate print preview with custom options
func PrintPreview(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    var req models.PrintPreviewRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // Use defaults
        req.Recipient = "Environmental Authority"
        req.Purpose = "Official Environmental Incident Report"
        req.IncludeImages = true
        req.IncludeStatistics = true
        req.IncludeTimeline = true
    }

    // Check access
    userRole, _ := c.Get("user_role")
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    // Get full report data
    report, err := getCompleteReport(reportID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
        return
    }

    // Generate case/reference numbers
    caseNumber := fmt.Sprintf("ET-%d-%s", reportID, time.Now().Format("2006-01-02"))
    referenceNumber := fmt.Sprintf("REF-%d-%d", reportID, time.Now().UnixNano()%10000)

    // QR Code data (URL to verify report authenticity)
    qrData := fmt.Sprintf("https://envtrack.gov/reports/%d/verify/%s", reportID, referenceNumber)

    response := models.PrintPreviewResponse{
        Report:          report,
        GeneratedAt:     time.Now(),
        ReferenceNumber: referenceNumber,
        CaseNumber:      caseNumber,
        QRCodeData:      qrData,
    }

    if req.IncludeStatistics {
        stats, _ := getReportStatistics(reportID)
        response.Statistics = stats
    }

    if req.IncludeTimeline {
        history, _ := getReportHistoryData(reportID)
        response.Timeline = history
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    response,
        "preview_options": gin.H{
            "recipient":         req.Recipient,
            "purpose":           req.Purpose,
            "additional_notes":  req.AdditionalNotes,
            "include_images":    req.IncludeImages,
            "include_statistics": req.IncludeStatistics,
            "include_timeline":  req.IncludeTimeline,
        },
    })
}

// DownloadReportPDF - Generate and download PDF
func DownloadReportPDF(c *gin.Context) {
    reportID, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
        return
    }

    // Check access
    userRole, _ := c.Get("user_role")
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    // Get complete report data
    report, err := getCompleteReport(reportID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
        return
    }

    history, _ := getReportHistoryData(reportID)

    // Generate PDF using gopdf
    pdf := gopdf.GoPdf{}
    pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
    pdf.AddPage()

    // Add title
    pdf.SetFont("Helvetica", "B", 18)
    pdf.SetX(30)
    pdf.SetY(50)
    pdf.Cell(nil, "ENVTRACK - OFFICIAL ENVIRONMENTAL REPORT")

    // Add metadata
    pdf.SetFont("Helvetica", "", 10)
    pdf.SetX(30)
    pdf.SetY(80)
    pdf.Cell(nil, fmt.Sprintf("Report ID: %d", report.ReportID))
    pdf.SetX(30)
    pdf.SetY(95)
    pdf.Cell(nil, fmt.Sprintf("Generated: %s", time.Now().Format("2006-01-02 15:04:05")))
    pdf.SetX(30)
    pdf.SetY(110)
    pdf.Cell(nil, fmt.Sprintf("Status: %s", report.Status))

    // Add content
    yPos := 140
    pdf.SetFont("Helvetica", "B", 14)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Report Details")
    yPos += 20

    // Title
    pdf.SetFont("Helvetica", "B", 12)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, fmt.Sprintf("Title: %s", report.Title))
    yPos += 20

    // Description
    pdf.SetFont("Helvetica", "", 11)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.MultiCell(nil, fmt.Sprintf("Description: %s", report.Description))
    yPos += 40

    // Location
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    addr := ""
    if report.Address != nil {
        addr = *report.Address
    }
    pdf.Cell(nil, fmt.Sprintf("Location: %s (%f, %f)", 
        addr, report.Latitude, report.Longitude))
    yPos += 20

    // Category
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, fmt.Sprintf("Category: %s", report.Category))
    yPos += 20

    // Reporter info
    pdf.SetFont("Helvetica", "B", 12)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Reporter Information")
    yPos += 20

    pdf.SetFont("Helvetica", "", 11)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, fmt.Sprintf("Name: %s %s", 
        report.ReporterFirstName, report.ReporterLastName))
    yPos += 18
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, fmt.Sprintf("Email: %s", report.ReporterEmail))
    yPos += 18
    if report.ReporterPhoneNumber != "" {
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, fmt.Sprintf("Phone: %s", report.ReporterPhoneNumber))
        yPos += 20
    }

    // Timeline
    if len(history) > 0 {
        pdf.SetFont("Helvetica", "B", 12)
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, "Timeline")
        yPos += 20

        pdf.SetFont("Helvetica", "", 10)
        for _, h := range history {
            pdf.SetX(30)
            pdf.SetY(float64(yPos))
            pdf.Cell(nil, fmt.Sprintf("%s - %s", 
                h.CreatedAt.Format("2006-01-02 15:04"), 
                h.Status))
            yPos += 15
        }
    }

    // Save PDF
    pdfFilename := fmt.Sprintf("report_%s_%d.pdf", time.Now().Format("20060102"), reportID)
    pdfPath := "/tmp/" + pdfFilename
    err = pdf.WritePdf(pdfPath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF"})
        return
    }

    c.FileAttachment(pdfPath, pdfFilename)
}

// ------------------- Helper Functions -------------------

func getCompleteReport(reportID int) (models.Report, error) {
    var report models.Report
    var photos, hashes pq.StringArray
    var address, adminNotes, dupTitle, dupAddress sql.NullString
    var dupOf sql.NullInt64
    var resolvedAt sql.NullTime
    
    err := database.DB.QueryRow(`
        SELECT 
            r.report_id,
            r.user_id,
            r.title,
            r.description,
            r.category,
            r.latitude,
            r.longitude,
            r.address,
            r.photo_urls,
            r.photo_hashes,
            r.status,
            r.duplicate_of,
            r.admin_notes,
            r.upvote_count,
            r.view_count,
            r.created_at,
            r.updated_at,
            r.resolved_at,
            u.email as reporter_email,
            u.phone_number as reporter_phone_number,
            u.first_name as reporter_first_name,
            u.last_name as reporter_last_name,
            dup.title as duplicate_of_title,
            dup.address as duplicate_of_address
        FROM reports r
        JOIN users u ON r.user_id = u.user_id
        LEFT JOIN reports dup ON r.duplicate_of = dup.report_id
        WHERE r.report_id = $1
    `, reportID).Scan(
        &report.ReportID,
        &report.UserID,
        &report.Title,
        &report.Description,
        &report.Category,
        &report.Latitude,
        &report.Longitude,
        &address,
        &photos,
        &hashes,
        &report.Status,
        &dupOf,
        &adminNotes,
        &report.UpvoteCount,
        &report.ViewCount,
        &report.CreatedAt,
        &report.UpdatedAt,
        &resolvedAt,
        &report.ReporterEmail,
        &report.ReporterPhoneNumber,
        &report.ReporterFirstName,
        &report.ReporterLastName,
        &dupTitle,
        &dupAddress,
    )
    if err != nil {
        return report, err
    }
    
    report.PhotoURLs = []string(photos)
    report.PhotoHashes = []string(hashes)
    if address.Valid {
        report.Address = &address.String
    }
    if adminNotes.Valid {
        report.AdminNotes = &adminNotes.String
    }
    if dupOf.Valid {
        v := int(dupOf.Int64)
        report.DuplicateOf = &v
    }
    if resolvedAt.Valid {
        report.ResolvedAt = &resolvedAt.Time
    }
    if dupTitle.Valid {
        report.DuplicateOfTitle = &dupTitle.String
    }
    if dupAddress.Valid {
        report.DuplicateOfAddress = &dupAddress.String
    }
    
    return report, nil
}

func getReportHistoryData(reportID int) ([]models.ReportHistory, error) {
    rows, err := database.DB.Query(`
        SELECT 
            history_id,
            report_id,
            status,
            changed_by,
            COALESCE(u.first_name || ' ' || u.last_name, 'System') as changed_by_name,
            notes,
            created_at
        FROM report_history rh
        LEFT JOIN users u ON rh.changed_by = u.user_id
        WHERE report_id = $1
        ORDER BY created_at ASC
    `, reportID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var history []models.ReportHistory
    for rows.Next() {
        var h models.ReportHistory
        var changedBy sql.NullInt64
        rows.Scan(
            &h.HistoryID,
            &h.ReportID,
            &h.Status,
            &changedBy,
            &h.ChangedByName,
            &h.Notes,
            &h.CreatedAt,
        )
        if changedBy.Valid {
            id := int(changedBy.Int64)
            h.ChangedBy = &id
        }
        history = append(history, h)
    }
    return history, nil
}

func getReportAttachments(reportID int) ([]models.Attachment, error) {
    var photoUrls []string
    database.DB.QueryRow(`
        SELECT photo_urls FROM reports WHERE report_id = $1
    `, reportID).Scan(&photoUrls)

    attachments := []models.Attachment{}
    for i, url := range photoUrls {
        attachments = append(attachments, models.Attachment{
            AttachmentID: i + 1,
            ReportID:     reportID,
            FileName:     url,
            FilePath:     "/uploads/" + url,
            FileType:     "image",
            UploadedAt:   time.Now(),
        })
    }
    return attachments, nil
}