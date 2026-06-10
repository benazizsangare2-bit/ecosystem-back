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
	"strconv"

	"github.com/gin-gonic/gin"
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
		SELECT report_id, user_id, title, description, category, latitude, longitude, address,
		       photo_urls, thumbnail_urls, status, duplicate_of, duplicate_warning, admin_notes,
		       upvote_count, view_count, created_at, updated_at
		FROM reports WHERE 1=1`
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

	appendFilter("status", status)
	appendFilter("category", category)
	if fromDate != "" {
		clause := fmt.Sprintf(" AND created_at >= $%d", n)
		base += clause
		countBase += clause
		args = append(args, fromDate)
		n++
	}
	if toDate != "" {
		clause := fmt.Sprintf(" AND created_at <= $%d", n)
		base += clause
		countBase += clause
		args = append(args, toDate)
		n++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
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