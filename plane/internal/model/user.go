package model

import (
	"time"
)

// UserRole 用户角色
type UserRole string

const (
	RoleAdmin  UserRole = "admin"  // 管理员
	RoleUser   UserRole = "user"   // 普通用户
	RoleGuest  UserRole = "guest"  // 访客
	RoleSystem UserRole = "system" // 系统
)

// User 用户
type User struct {
	// 基本信息
	ID        string    `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	Email     string    `json:"email" db:"email"`
	Password  string    `json:"-" db:"password"`
	Role      UserRole  `json:"role" db:"role"`
	Enabled   bool      `json:"enabled" db:"enabled"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	LastLogin time.Time `json:"last_login" db:"last_login"`

	// 权限
	Permissions []string `json:"permissions" db:"-"`
}

// Permission 权限
type Permission struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// UserPermission 用户权限
type UserPermission struct {
	UserID       string    `json:"user_id" db:"user_id"`
	PermissionID string    `json:"permission_id" db:"permission_id"`
	GrantedAt    time.Time `json:"granted_at" db:"granted_at"`
	GrantedBy    string    `json:"granted_by" db:"granted_by"`
}

// RolePermission 角色权限
type RolePermission struct {
	Role         UserRole  `json:"role" db:"role"`
	PermissionID string    `json:"permission_id" db:"permission_id"`
	GrantedAt    time.Time `json:"granted_at" db:"granted_at"`
}

// ProbePermission 探测权限
type ProbePermission struct {
	UserID     string    `json:"user_id" db:"user_id"`
	NodeID     string    `json:"node_id" db:"node_id"`
	GroupID    string    `json:"group_id" db:"group_id"`
	GrantedAt  time.Time `json:"granted_at" db:"granted_at"`
	GrantedBy  string    `json:"granted_by" db:"granted_by"`
	Expiration time.Time `json:"expiration" db:"expiration"`
}
