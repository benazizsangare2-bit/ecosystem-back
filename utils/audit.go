package utils

import (
	"ecosystem/database"
	"encoding/json"
)

type AuditLogEntry struct {
    AdminID    int
    Action     string
    TargetType string
    TargetID   int
    OldData    interface{}
    NewData    interface{}
    IPAddress  string
    UserAgent  string
}

func LogAdminAction(entry AuditLogEntry) error {
    var oldDataJSON, newDataJSON string
    
    if entry.OldData != nil {
        oldBytes, err := json.Marshal(entry.OldData)
        if err != nil {
            return err
        }
        oldDataJSON = string(oldBytes)
    }
    
    if entry.NewData != nil {
        newBytes, err := json.Marshal(entry.NewData)
        if err != nil {
            return err
        }
        newDataJSON = string(newBytes)
    }
    
    _, err := database.DB.Exec(`
        INSERT INTO audit_logs (admin_id, action, target_type, target_id, old_data, new_data, ip_address, user_agent)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
        entry.AdminID,
        entry.Action,
        entry.TargetType,
        entry.TargetID,
        oldDataJSON,
        newDataJSON,
        entry.IPAddress,
        entry.UserAgent,
    )
    
    return err
}