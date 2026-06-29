package handlers

import (
	"database/sql"
	"ecosystem/database"
	"ecosystem/middleware"
	"ecosystem/models"
	"ecosystem/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/signintech/gopdf"
)

func GetAdminReports(c *gin.Context) {
	status := c.Query("status")
	category := c.Query("category")
	fromDate := c.Query("from")
	toDate := c.Query("to")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	base := `
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
		WHERE 1=1`
	countBase := `SELECT COUNT(*) FROM reports WHERE 1=1`
	args := []interface{}{}
	n := 1

	appendFilter := func(column string, value string) {
		if value == "" {
			return
		}
		clause := fmt.Sprintf(" AND %s = $%d", column, n)
		base += clause
		countBase += clause
		args = append(args, value)
		n++
	}

	appendFilter("r.status", status)
	appendFilter("r.category", category)
	if fromDate != "" {
		clause := fmt.Sprintf(" AND r.created_at >= $%d", n)
		base += clause
		countBase += clause
		args = append(args, fromDate)
		n++
	}
	if toDate != "" {
		clause := fmt.Sprintf(" AND r.created_at <= $%d", n)
		base += clause
		countBase += clause
		args = append(args, toDate)
		n++
	}

	base += fmt.Sprintf(" ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	pageArgs := append(append([]interface{}{}, args...), limit, offset)

	rows, err := database.DB.Query(base, pageArgs...)
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
	if err := database.DB.QueryRow(countBase, args...).Scan(&total); err != nil {
		utils.InternalError(c, "Database error")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{
		"reports": reports,
		"page":    page,
		"limit":   limit,
		"total":   total,
	}, "Admin reports retrieved")
}

func UpdateReportStatus(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid report id")
		return
	}

	var req models.AdminStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	if !models.ValidReportStatuses[req.Status] {
		utils.BadRequest(c, "invalid status")
		return
	}

	var oldStatus string
	var ownerID int
	var title string
	err = database.DB.QueryRow(`
		SELECT status, user_id, title FROM reports WHERE report_id = $1`,
		reportID,
	).Scan(&oldStatus, &ownerID, &title)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "Report not found")
		return
	}
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}

	_, err = database.DB.Exec(`
		UPDATE reports SET status = $1, admin_notes = $2, duplicate_of = COALESCE($3, duplicate_of),
		                   updated_at = NOW(),
		                   resolved_at = CASE WHEN $1 IN ('resolved', 'rejected', 'duplicate') THEN NOW() ELSE resolved_at END,
		                   resolved_by = CASE WHEN $1 IN ('resolved', 'rejected', 'duplicate') THEN $4 ELSE resolved_by END
		WHERE report_id = $5`,
		req.Status, nullIfEmpty(req.AdminNotes), req.DuplicateOf, adminID, reportID,
	)
	if err != nil {
		utils.InternalError(c, "Failed to update status")
		return
	}

	if req.Status == "investigating" && oldStatus != "investigating" {
		_ = utils.AdjustReputation(ownerID, utils.ReputationApprovedReport)
		_, _ = database.DB.Exec(`
			INSERT INTO notifications (user_id, report_id, title, message)
			VALUES ($1, $2, 'Report approved for public view', $3)`,
			ownerID, reportID,
			"Your report \""+title+"\" is now visible on the public feed.",
		)
	}

	if req.Status == "duplicate" {
		_ = utils.AdjustReputation(ownerID, utils.ReputationDuplicate)
		_ = utils.NotifyUserReportDuplicate(ownerID, reportID, title)
	}

	oldJSON, _ := json.Marshal(gin.H{"status": oldStatus})
	newJSON, _ := json.Marshal(gin.H{"status": req.Status, "admin_notes": req.AdminNotes, "duplicate_of": req.DuplicateOf})
	_, _ = database.DB.Exec(`
		INSERT INTO audit_logs (admin_id, action, target_type, target_id, old_data, new_data, ip_address)
		VALUES ($1, 'update_report_status', 'report', $2, $3, $4, $5)`,
		adminID, reportID, oldJSON, newJSON, c.ClientIP(),
	)

	report, _ := fetchReportByID(reportID)
	utils.Success(c, http.StatusOK, report, "Report status updated")
}

func GetAdminStats(c *gin.Context) {
	stats := models.AdminStats{
		ByStatus:   make(map[string]int),
		ByCategory: make(map[string]int),
	}

	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM reports`).Scan(&stats.TotalReports)
	_ = database.DB.QueryRow(`
		SELECT COUNT(*) FROM reports WHERE created_at > NOW() - INTERVAL '7 days'`).Scan(&stats.RecentReports)
	_ = database.DB.QueryRow(`
		SELECT COUNT(*) FROM reports WHERE duplicate_warning = true AND status = 'pending'`).Scan(&stats.DuplicateWarnings)

	statusRows, err := database.DB.Query(`SELECT status, COUNT(*) FROM reports GROUP BY status`)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var s string
			var n int
			if statusRows.Scan(&s, &n) == nil {
				stats.ByStatus[s] = n
			}
		}
	}

	catRows, err := database.DB.Query(`SELECT category, COUNT(*) FROM reports GROUP BY category`)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var cat string
			var n int
			if catRows.Scan(&cat, &n) == nil {
				stats.ByCategory[cat] = n
			}
		}
	}

	utils.Success(c, http.StatusOK, stats, "Admin stats retrieved")
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// GetAuditLogs - Returns paginated audit logs for admin dashboard
func GetAuditLogs(c *gin.Context) {
    // Get pagination parameters
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
    
    if page < 1 {
        page = 1
    }
    if limit < 1 || limit > 100 {
        limit = 50
    }
    
    offset := (page - 1) * limit
    
    // Get filters
    action := c.Query("action")
    targetType := c.Query("target_type")
    adminID := c.Query("admin_id")
    
    // Build query with filters
    query := `
        SELECT 
            al.log_id,
            al.admin_id,
            CONCAT_WS(' ', u.first_name, u.last_name) as admin_name,
            al.action,
            COALESCE(al.target_type, '') as target_type,
            COALESCE(al.target_id, 0) as target_id,
            al.old_data,
            al.new_data,
            COALESCE(al.ip_address::text, '') as ip_address,
            COALESCE(al.user_agent, '') as user_agent,
            al.created_at
        FROM audit_logs al
        JOIN users u ON al.admin_id = u.user_id
        WHERE 1=1
    `
    countQuery := `SELECT COUNT(*) FROM audit_logs al WHERE 1=1`
    
    var args []interface{}
    argCounter := 1
    
    // Apply filters
    if action != "" {
        query += fmt.Sprintf(" AND al.action = $%d", argCounter)
        countQuery += fmt.Sprintf(" AND action = $%d", argCounter)
        args = append(args, action)
        argCounter++
    }
    
    if targetType != "" {
        query += fmt.Sprintf(" AND al.target_type = $%d", argCounter)
        countQuery += fmt.Sprintf(" AND target_type = $%d", argCounter)
        args = append(args, targetType)
        argCounter++
    }
    
    if adminID != "" {
        query += fmt.Sprintf(" AND al.admin_id = $%d", argCounter)
        countQuery += fmt.Sprintf(" AND admin_id = $%d", argCounter)
        args = append(args, adminID)
        argCounter++
    }
    
    // Add order and pagination
    query += fmt.Sprintf(" ORDER BY al.created_at DESC LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
    args = append(args, limit, offset)
    
    // Execute query
    rows, err := database.DB.Query(query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit logs"})
        return
    }
    defer rows.Close()
    
    var logs []models.AuditLog
    for rows.Next() {
        var log models.AuditLog
        var oldData, newData sql.NullString
        err := rows.Scan(
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
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan audit logs"})
            return
        }
        
        if oldData.Valid {
            log.OldData = oldData.String
        }
        if newData.Valid {
            log.NewData = newData.String
        }
        
        logs = append(logs, log)
    }
    
    // Get total count
    var total int
    countArgs := args[:len(args)-2] // Remove limit and offset
    err = database.DB.QueryRow(countQuery, countArgs...).Scan(&total)
    if err != nil {
        total = 0
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "logs": logs,
            "pagination": gin.H{
                "page":  page,
                "limit": limit,
                "total": total,
                "pages": (total + limit - 1) / limit,
            },
        },
    })
}

// GetAuditLogActions - Returns list of available action types for filter dropdown
func ListUsers(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := `
		SELECT user_id, email, phone_number, first_name, last_name, role,
		       is_email_verified, reputation_score, total_reports, status, deleted_at, created_at
		FROM users WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM users WHERE 1=1`
	args := []interface{}{}
	n := 1

	if status != "" {
		clause := fmt.Sprintf(" AND status = $%d", n)
		query += clause
		countQuery += clause
		args = append(args, status)
		n++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	pageArgs := append(append([]interface{}{}, args...), limit, offset)

	rows, err := database.DB.Query(query, pageArgs...)
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}
	defer rows.Close()

	type userRow struct {
		models.User
		DeletedAt *time.Time `json:"deleted_at,omitempty"`
	}
	var users []userRow
	for rows.Next() {
		var u userRow
		err := rows.Scan(
			&u.UserID, &u.Email, &u.PhoneNumber, &u.FirstName, &u.LastName,
			&u.Role, &u.IsEmailVerified, &u.ReputationScore, &u.TotalReports,
			&u.Status, &u.DeletedAt, &u.CreatedAt,
		)
		if err != nil {
			utils.InternalError(c, "Failed to scan users")
			return
		}
		users = append(users, u)
	}

	var total int
	if err := database.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		utils.InternalError(c, "Database error")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{
		"users": users,
		"page":  page,
		"limit": limit,
		"total": total,
	}, "Users retrieved")
}

func AdminDeleteUser(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	targetID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	if targetID == adminID {
		utils.BadRequest(c, "Cannot delete your own account through admin panel")
		return
	}

	var oldRole string
	var oldStatus string
	err = database.DB.QueryRow(
		`SELECT role, status FROM users WHERE user_id = $1 AND deleted_at IS NULL`,
		targetID,
	).Scan(&oldRole, &oldStatus)
	if err == sql.ErrNoRows {
		utils.NotFound(c, "User not found or already deleted")
		return
	}
	if err != nil {
		utils.InternalError(c, "Database error")
		return
	}

	result, err := database.DB.Exec(`
		UPDATE users SET deleted_at = NOW(), status = 'deleted', updated_at = NOW()
		WHERE user_id = $1 AND deleted_at IS NULL`,
		targetID,
	)
	if err != nil {
		utils.InternalError(c, "Failed to delete user")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		utils.NotFound(c, "User not found or already deleted")
		return
	}

	oldJSON, _ := json.Marshal(gin.H{"role": oldRole, "status": oldStatus})
	newJSON, _ := json.Marshal(gin.H{"status": "deleted", "deleted_at": "now"})
	_, _ = database.DB.Exec(`
		INSERT INTO audit_logs (admin_id, action, target_type, target_id, old_data, new_data, ip_address, user_agent)
		VALUES ($1, 'admin_delete_user', 'user', $2, $3, $4, $5, $6)`,
		adminID, targetID, oldJSON, newJSON, c.ClientIP(), c.Request.UserAgent(),
	)

	utils.Success(c, http.StatusOK, nil, "User account has been deactivated")
}

func GetAuditLogActions(c *gin.Context) {
    rows, err := database.DB.Query(`
        SELECT DISTINCT action 
        FROM audit_logs 
        ORDER BY action
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch actions"})
        return
    }
    defer rows.Close()
    
    var actions []string
    for rows.Next() {
        var action string
        rows.Scan(&action)
        actions = append(actions, action)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": actions,
    })
}

///////// general report of the system for admin dashboard
// GetSystemReport - Complete system-wide statistics for reporting
func GetSystemReport(c *gin.Context) {
    // Check admin role
    userRole := middleware.GetRole(c)
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    // Get time range from query params (default: last 30 days)
    from := c.Query("from")
    to := c.Query("to")
    if from == "" {
        from = time.Now().AddDate(0, -1, 0).Format("2006-01-02") // 1 month ago
    }
    if to == "" {
        to = time.Now().Format("2006-01-02")
    }

    // Get all statistics with date filters
    stats, err := getSystemStatsWithDateRange(from, to)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate stats"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "report": gin.H{
                "generated_at":      time.Now().Format("2006-01-02 15:04:05"),
                "date_range":        gin.H{"from": from, "to": to},
                "reference_number":  fmt.Sprintf("SYS-REP-%d-%s", time.Now().UnixNano()%10000, time.Now().Format("20060102")),
            },
            "statistics": stats,
            "charts": gin.H{
                "weekly_trends":      getWeeklyTrendsWithDateRange(from, to),
                "category_distribution": getCategoryDistributionWithDateRange(from, to),
                "status_distribution": getStatusDistributionWithDateRange(from, to),
                "monthly_trends":     getMonthlyTrendsWithDateRange(from, to),
            },
            "top_reporters":   getTopReportersWithDateRange(from, to),
            "performance": gin.H{
                "avg_resolution_time": getAverageResolutionTime(from, to),
                "resolution_rate":     getResolutionRate(from, to),
                "duplicate_rate":      getDuplicateRate(from, to),
            },
        },
    })
}

// Helper functions (implement these)
func GetSystemReportPDF(c *gin.Context) {
    userRole := middleware.GetRole(c)
    if userRole != "admin" && userRole != "authority" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    from := c.Query("from")
    to := c.Query("to")
    if from == "" {
        from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
    }
    if to == "" {
        to = time.Now().Format("2006-01-02")
    }

    stats, err := getSystemStatsWithDateRange(from, to)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report PDF"})
        return
    }

    weekly := getWeeklyTrendsWithDateRange(from, to)
    category := getCategoryDistributionWithDateRange(from, to)
    status := getStatusDistributionWithDateRange(from, to)
    top := getTopReportersWithDateRange(from, to)

    pdf := gopdf.GoPdf{}
    pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
    if err := pdf.AddTTFFont("Ubuntu-L", "assets/fonts/Ubuntu-L.ttf"); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register font for PDF"})
        return
    }
    pdf.AddPage()
    if err := pdf.SetFont("Ubuntu-L", "B", 18); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    pdf.SetX(30)
    pdf.SetY(50)
    pdf.Cell(nil, "System Report")

    if err := pdf.SetFont("Ubuntu-L", "", 10); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    pdf.SetX(30)
    pdf.SetY(75)
    pdf.Cell(nil, fmt.Sprintf("Date range: %s to %s", from, to))
    pdf.SetX(30)
    pdf.SetY(90)
    pdf.Cell(nil, fmt.Sprintf("Generated: %s", time.Now().Format("2006-01-02 15:04:05")))

    yPos := 115
    if err := pdf.SetFont("Ubuntu-L", "B", 12); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Executive Summary")
    yPos += 18
    if err := pdf.SetFont("Ubuntu-L", "", 10); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }

    lines := []string{
        fmt.Sprintf("Total reports: %v", stats["total_reports"]),
        fmt.Sprintf("Active users: %v", stats["total_users"]),
        fmt.Sprintf("Total upvotes: %v", stats["total_upvotes"]),
        fmt.Sprintf("Average reports per user: %.2f", stats["avg_reports_per_user"]),
        fmt.Sprintf("Duplicate warnings: %v", stats["duplicate_warnings"]),
        fmt.Sprintf("Resolution rate: %.1f%%", stats["resolution_rate"]),
        fmt.Sprintf("Duplicate rate: %.1f%%", stats["duplicate_rate"]),
        fmt.Sprintf("Average resolution time: %.1f hours", stats["avg_resolution_time"]),
    }

    for _, line := range lines {
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, line)
        yPos += 14
    }

    yPos += 10
    pdf.SetFont("Helvetica", "B", 12)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Top Reporters")
    yPos += 16
    pdf.SetFont("Helvetica", "", 10)
    for _, reporter := range top {
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, fmt.Sprintf("%s - %d reports", reporter.Name, reporter.ReportCount))
        yPos += 14
        if yPos > 760 {
            pdf.AddPage()
            yPos = 50
        }
    }

    yPos += 12
    if err := pdf.SetFont("Ubuntu-L", "B", 12); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Top Categories")
    yPos += 16
    if err := pdf.SetFont("Ubuntu-L", "", 10); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    for _, item := range category {
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, fmt.Sprintf("%s: %d", item.Category, item.Count))
        yPos += 14
        if yPos > 760 {
            pdf.AddPage()
            yPos = 50
        }
    }

    yPos += 12
    if err := pdf.SetFont("Ubuntu-L", "B", 12); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Weekly Trends")
    yPos += 16
    if err := pdf.SetFont("Ubuntu-L", "", 10); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PDF font"})
        return
    }
    for _, trend := range weekly {
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, fmt.Sprintf("%s - %d reports, %d resolved, %d rejected, %d investigating, %d upvotes",
            trend.Week.Format("2006-01-02"), trend.TotalReports, trend.Resolved, trend.Rejected, trend.Investigating, trend.TotalUpvotes))
        yPos += 14
        if yPos > 760 {
            pdf.AddPage()
            yPos = 50
        }
    }

    yPos += 12
    pdf.SetFont("Helvetica", "B", 12)
    pdf.SetX(30)
    pdf.SetY(float64(yPos))
    pdf.Cell(nil, "Status Distribution")
    yPos += 16
    pdf.SetFont("Helvetica", "", 10)
    for _, item := range status {
        pdf.SetX(30)
        pdf.SetY(float64(yPos))
        pdf.Cell(nil, fmt.Sprintf("%s: %d", item["status"], item["count"]))
        yPos += 14
        if yPos > 760 {
            pdf.AddPage()
            yPos = 50
        }
    }

    fileName := fmt.Sprintf("system-report-%s.pdf", time.Now().Format("20060102"))
    pdfPath := filepath.Join(os.TempDir(), fileName)
    if err := pdf.WritePdf(pdfPath); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF"})
        return
    }

    c.FileAttachment(pdfPath, fileName)
}

func getSystemStatsWithDateRange(from, to string) (map[string]interface{}, error) {
    start, end := parseDateRange(from, to)
    stats := map[string]interface{}{}

    var totalReports, totalUpvotes, duplicateWarnings, recentReports7d int
    var avgReportsPerUser float64
    var totalUsers int

    if err := database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
    `, start, end).Scan(&totalReports); err != nil {
        return nil, err
    }
    if err := database.DB.QueryRow(`
        SELECT COALESCE(SUM(upvote_count), 0) FROM reports
        WHERE created_at >= $1 AND created_at < $2
    `, start, end).Scan(&totalUpvotes); err != nil {
        return nil, err
    }
    if err := database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE duplicate_warning = true AND created_at >= $1 AND created_at < $2
    `, start, end).Scan(&duplicateWarnings); err != nil {
        return nil, err
    }
    if err := database.DB.QueryRow(`
        SELECT COALESCE(AVG(report_count), 0)
        FROM (
            SELECT COUNT(*) as report_count
            FROM reports
            WHERE created_at >= $1 AND created_at < $2
            GROUP BY user_id
        ) as user_reports
    `, start, end).Scan(&avgReportsPerUser); err != nil {
        return nil, err
    }
    if err := database.DB.QueryRow(`
        SELECT COUNT(*) FROM users
        WHERE status = 'active' AND deleted_at IS NULL
    `).Scan(&totalUsers); err != nil {
        return nil, err
    }
    if err := database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
          AND created_at >= NOW() - INTERVAL '7 days'
    `, start, end).Scan(&recentReports7d); err != nil {
        return nil, err
    }

    stats["total_reports"] = totalReports
    stats["total_upvotes"] = totalUpvotes
    stats["duplicate_warnings"] = duplicateWarnings
    stats["avg_reports_per_user"] = avgReportsPerUser
    stats["total_users"] = totalUsers
    stats["recent_reports_7d"] = recentReports7d
    stats["resolution_rate"] = getResolutionRate(from, to)
    stats["duplicate_rate"] = getDuplicateRate(from, to)
    stats["avg_resolution_time"] = getAverageResolutionTime(from, to)

    byStatus := make(map[string]int)
    rows, err := database.DB.Query(`
        SELECT status, COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY status
    `, start, end)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var status string
        var count int
        if rows.Scan(&status, &count) == nil {
            byStatus[status] = count
        }
    }

    byCategory := make(map[string]int)
    rows, err = database.DB.Query(`
        SELECT category, COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY category
    `, start, end)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var category string
        var count int
        if rows.Scan(&category, &count) == nil {
            byCategory[category] = count
        }
    }

    stats["by_status"] = byStatus
    stats["by_category"] = byCategory
    return stats, nil
}

func getWeeklyTrendsWithDateRange(from, to string) []models.WeeklyTrend {
    start, end := parseDateRange(from, to)
    rows, err := database.DB.Query(`
        SELECT DATE_TRUNC('week', created_at) as week,
               COUNT(*) as total_reports,
               COUNT(CASE WHEN status = 'investigating' THEN 1 END) as investigating,
               COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved,
               COUNT(CASE WHEN status = 'rejected' THEN 1 END) as rejected,
               COALESCE(SUM(upvote_count), 0) as total_upvotes
        FROM reports
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY week
        ORDER BY week ASC
    `, start, end)
    if err != nil {
        return nil
    }
    defer rows.Close()

    var trends []models.WeeklyTrend
    for rows.Next() {
        var t models.WeeklyTrend
        if rows.Scan(&t.Week, &t.TotalReports, &t.Investigating, &t.Resolved, &t.Rejected, &t.TotalUpvotes) == nil {
            trends = append(trends, t)
        }
    }
    return trends
}

func getMonthlyTrendsWithDateRange(from, to string) []models.MonthlyTrend {
    start, end := parseDateRange(from, to)
    rows, err := database.DB.Query(`
        SELECT TO_CHAR(created_at, 'YYYY-MM') as month,
               COUNT(*) as total_reports,
               COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved,
               COUNT(CASE WHEN status = 'investigating' THEN 1 END) as investigating
        FROM reports
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY month
        ORDER BY month ASC
    `, start, end)
    if err != nil {
        return nil
    }
    defer rows.Close()

    var trends []models.MonthlyTrend
    for rows.Next() {
        var t models.MonthlyTrend
        if rows.Scan(&t.Month, &t.TotalReports, &t.Resolved, &t.Investigating) == nil {
            trends = append(trends, t)
        }
    }
    return trends
}

func getCategoryDistributionWithDateRange(from, to string) []models.CategoryDistribution {
    start, end := parseDateRange(from, to)
    rows, err := database.DB.Query(`
        SELECT category, COUNT(*) as count, COALESCE(SUM(upvote_count), 0) as total_upvotes
        FROM reports
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY category
        ORDER BY count DESC
    `, start, end)
    if err != nil {
        return nil
    }
    defer rows.Close()

    var distribution []models.CategoryDistribution
    for rows.Next() {
        var d models.CategoryDistribution
        if rows.Scan(&d.Category, &d.Count, &d.TotalUpvotes) == nil {
            distribution = append(distribution, d)
        }
    }
    return distribution
}

func getStatusDistributionWithDateRange(from, to string) []gin.H {
    start, end := parseDateRange(from, to)
    rows, err := database.DB.Query(`
        SELECT status, COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY status
        ORDER BY COUNT(*) DESC
    `, start, end)
    if err != nil {
        return nil
    }
    defer rows.Close()

    var result []gin.H
    for rows.Next() {
        var status string
        var count int
        if rows.Scan(&status, &count) == nil {
            result = append(result, gin.H{"status": status, "count": count})
        }
    }
    return result
}

func getTopReportersWithDateRange(from, to string) []models.TopReporter {
    start, end := parseDateRange(from, to)
    rows, err := database.DB.Query(`
        SELECT u.user_id, u.first_name || ' ' || u.last_name as name, COUNT(r.report_id) as report_count
        FROM users u
        JOIN reports r ON u.user_id = r.user_id
        WHERE r.created_at >= $1 AND r.created_at < $2
          AND u.deleted_at IS NULL
        GROUP BY u.user_id, u.first_name, u.last_name
        ORDER BY report_count DESC
        LIMIT 10
    `, start, end)
    if err != nil {
        return nil
    }
    defer rows.Close()

    var result []models.TopReporter
    for rows.Next() {
        var reporter models.TopReporter
        if rows.Scan(&reporter.UserID, &reporter.Name, &reporter.ReportCount) == nil {
            result = append(result, reporter)
        }
    }
    return result
}

func getAverageResolutionTime(from, to string) float64 {
    start, end := parseDateRange(from, to)
    var avg float64
    _ = database.DB.QueryRow(`
        SELECT COALESCE(AVG(EXTRACT(EPOCH FROM resolved_at - created_at) / 3600), 0)
        FROM reports
        WHERE status = 'resolved'
          AND created_at >= $1 AND created_at < $2
          AND resolved_at IS NOT NULL
    `, start, end).Scan(&avg)
    return avg
}

func getResolutionRate(from, to string) float64 {
    start, end := parseDateRange(from, to)
    var resolved float64
    var total float64
    _ = database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
    `, start, end).Scan(&total)
    if total == 0 {
        return 0
    }
    _ = database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE status = 'resolved'
          AND created_at >= $1 AND created_at < $2
    `, start, end).Scan(&resolved)
    return (resolved / total) * 100
}

func getDuplicateRate(from, to string) float64 {
    start, end := parseDateRange(from, to)
    var duplicates float64
    var total float64
    _ = database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE created_at >= $1 AND created_at < $2
    `, start, end).Scan(&total)
    if total == 0 {
        return 0
    }
    _ = database.DB.QueryRow(`
        SELECT COUNT(*) FROM reports
        WHERE status = 'duplicate'
          AND created_at >= $1 AND created_at < $2
    `, start, end).Scan(&duplicates)
    return (duplicates / total) * 100
}

func parseDateRange(from, to string) (time.Time, time.Time) {
    loc := time.Now().Location()
    start, err := time.ParseInLocation("2006-01-02", from, loc)
    if err != nil {
        start = time.Now().AddDate(0, -1, 0)
    }
    end, err := time.ParseInLocation("2006-01-02", to, loc)
    if err != nil {
        end = time.Now()
    }
    end = end.AddDate(0, 0, 1)
    return start, end
}