package middleware

import (
	"strings"

	"gkipass/plane/db"
	"gkipass/plane/internal/auth"

	"github.com/gin-gonic/gin"
)

// CKAuth CK 认证中间件（用户）
func CKAuth(dbManager *db.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Header 或 Query 获取 CK
		ck := c.GetHeader("X-Connection-Key")
		if ck == "" {
			ck = c.GetHeader("Authorization")
			if strings.HasPrefix(ck, "CK ") {
				ck = strings.TrimPrefix(ck, "CK ")
			}
		}
		if ck == "" {
			ck = c.Query("ck")
		}

		if ck == "" {
			c.JSON(401, gin.H{"error": "Connection Key required"})
			c.Abort()
			return
		}

		// 验证 CK
		ckObj, err := dbManager.DB.SQLite.GetConnectionKeyByKey(ck)
		if err != nil || ckObj == nil {
			c.JSON(401, gin.H{"error": "Invalid Connection Key"})
			c.Abort()
			return
		}

		if err := auth.ValidateCK(ckObj.Key); err != nil {
			c.JSON(401, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// 仅允许用户类型的 CK
		if ckObj.Type != "user" {
			c.JSON(403, gin.H{"error": "Invalid CK type"})
			c.Abort()
			return
		}

		// 获取用户信息
		user, err := dbManager.DB.SQLite.GetUser(ckObj.NodeID) // NodeID 存储的是 UserID
		if err != nil || user == nil {
			c.JSON(401, gin.H{"error": "User not found"})
			c.Abort()
			return
		}

		if !user.Enabled {
			c.JSON(403, gin.H{"error": "User disabled"})
			c.Abort()
			return
		}

		// 设置用户信息到上下文
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Set("ck", ck)

		c.Next()
	}
}

// NodeCKAuth 节点 CK 认证中间件
func NodeCKAuth(dbManager *db.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ck := c.Query("ck")
		if ck == "" {
			ck = c.GetHeader("X-Connection-Key")
		}

		if ck == "" {
			c.JSON(401, gin.H{"error": "Connection Key required"})
			c.Abort()
			return
		}

		// 验证 CK
		ckObj, err := dbManager.DB.SQLite.GetConnectionKeyByKey(ck)
		if err != nil || ckObj == nil {
			c.JSON(401, gin.H{"error": "Invalid Connection Key"})
			c.Abort()
			return
		}

		if err := auth.ValidateCK(ckObj.Key); err != nil {
			c.JSON(401, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// 仅允许节点类型的 CK
		if ckObj.Type != "node" {
			c.JSON(403, gin.H{"error": "Invalid CK type"})
			c.Abort()
			return
		}

		// 获取节点信息
		node, err := dbManager.DB.SQLite.GetNode(ckObj.NodeID)
		if err != nil || node == nil {
			c.JSON(401, gin.H{"error": "Node not found"})
			c.Abort()
			return
		}

		// 设置节点信息到上下文
		c.Set("node_id", node.ID)
		c.Set("node_name", node.Name)
		c.Set("user_id", node.UserID)

		c.Next()
	}
}
