package handlers

import (
	"database/sql"
	"ecosystem/database"
	"ecosystem/middleware"
	"ecosystem/models"
	"ecosystem/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
		SELECT report_id, user_id, title, description, category, latitude, longitude, address,
		       photo_urls, thumbnail_urls, status, duplicate_of, duplicate_warning, admin_notes,
		       upvote_count, view_count, created_at, updated_at
		FROM reports WHERE user_id = $1 ORDER BY created_at DESC`,
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
		SELECT report_id, user_id, title, description, category, latitude, longitude, address,
		       photo_urls, thumbnail_urls, status, duplicate_of, duplicate_warning, admin_notes,
		       upvote_count, view_count, created_at, updated_at
		FROM reports WHERE status = 'investigating'
		ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
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
		SELECT report_id, user_id, title, description, category, latitude, longitude, address,
		       photo_urls, thumbnail_urls, status, duplicate_of, duplicate_warning, admin_notes,
		       upvote_count, view_count, created_at, updated_at
		FROM reports WHERE report_id = $1`, reportID)
	return scanReport(row)
}

func scanReport(row *sql.Row) (models.Report, error) {
	var r models.Report
	var photos, thumbs pq.StringArray
	var dupOf sql.NullInt64
	var adminNotes, address sql.NullString

	err := row.Scan(
		&r.ReportID, &r.UserID, &r.Title, &r.Description, &r.Category,
		&r.Latitude, &r.Longitude, &address, &photos, &thumbs, &r.Status,
		&dupOf, &r.DuplicateWarning, &adminNotes, &r.UpvoteCount, &r.ViewCount,
		&r.CreatedAt, &r.UpdatedAt,
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
	if adminNotes.Valid {
		r.AdminNotes = &adminNotes.String
	}
	if address.Valid {
		r.Address = &address.String
	}
	return r, nil
}

func scanReports(rows *sql.Rows) ([]models.Report, error) {
	var reports []models.Report
	for rows.Next() {
		var r models.Report
		var photos, thumbs pq.StringArray
		var dupOf sql.NullInt64
		var adminNotes, address sql.NullString
		err := rows.Scan(
			&r.ReportID, &r.UserID, &r.Title, &r.Description, &r.Category,
			&r.Latitude, &r.Longitude, &address, &photos, &thumbs, &r.Status,
			&dupOf, &r.DuplicateWarning, &adminNotes, &r.UpvoteCount, &r.ViewCount,
			&r.CreatedAt, &r.UpdatedAt,
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
		if adminNotes.Valid {
			r.AdminNotes = &adminNotes.String
		}
		if address.Valid {
			r.Address = &address.String
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
