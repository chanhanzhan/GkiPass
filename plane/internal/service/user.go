package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"gkipass/plane/internal/model"
)

// UserClaims 用户声明
type UserClaims struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	ExpiresAt   int64    `json:"exp"`
}

// UserService 用户服务
type UserService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewUserService 创建用户服务
func NewUserService(db *sql.DB) *UserService {
	return &UserService{
		db:     db,
		logger: zap.L().Named("user-service"),
	}
}

// CreateUser 创建用户
func (s *UserService) CreateUser(user *model.User) error {
	// 验证用户
	if err := s.validateUser(user); err != nil {
		return err
	}

	// 生成ID
	if user.ID == "" {
		user.ID = uuid.New().String()
	}

	// 设置创建时间和更新时间
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// 哈希密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("哈希密码失败",
			zap.Error(err))
		return fmt.Errorf("哈希密码失败: %w", err)
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 插入用户
	_, err = tx.Exec(`
		INSERT INTO users (id, username, email, password, role, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		user.ID, user.Username, user.Email, hashedPassword, user.Role, user.Enabled, user.CreatedAt, user.UpdatedAt,
	)

	if err != nil {
		tx.Rollback()
		s.logger.Error("创建用户失败",
			zap.String("username", user.Username),
			zap.Error(err))
		return fmt.Errorf("创建用户失败: %w", err)
	}

	// 插入用户权限
	if len(user.Permissions) > 0 {
		for _, permission := range user.Permissions {
			_, err = tx.Exec(`
				INSERT INTO user_permissions (user_id, permission_id, granted_at, granted_by)
				VALUES (?, ?, ?, ?)
			`,
				user.ID, permission, now, user.ID, // 创建者即为授权者
			)

			if err != nil {
				tx.Rollback()
				s.logger.Error("插入用户权限失败",
					zap.String("user_id", user.ID),
					zap.String("permission", permission),
					zap.Error(err))
				return fmt.Errorf("插入用户权限失败: %w", err)
			}
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		s.logger.Error("提交事务失败",
			zap.Error(err))
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.logger.Info("创建用户成功",
		zap.String("id", user.ID),
		zap.String("username", user.Username))

	return nil
}

// GetUser 获取用户
func (s *UserService) GetUser(id string) (*model.User, error) {
	var user model.User

	// 查询用户
	err := s.db.QueryRow(`
		SELECT id, username, email, role, enabled, created_at, updated_at, last_login
		FROM users 
		WHERE id = ?
	`, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.Role, &user.Enabled, &user.CreatedAt, &user.UpdatedAt, &user.LastLogin,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("用户不存在: %s", id)
		}
		s.logger.Error("获取用户失败",
			zap.String("id", id),
			zap.Error(err))
		return nil, fmt.Errorf("获取用户失败: %w", err)
	}

	// 查询用户权限
	permissions, err := s.GetUserPermissions(id)
	if err != nil {
		s.logger.Error("获取用户权限失败",
			zap.String("id", id),
			zap.Error(err))
	} else {
		user.Permissions = permissions
	}

	return &user, nil
}

// GetUserByUsername 通过用户名获取用户
func (s *UserService) GetUserByUsername(username string) (*model.User, error) {
	var user model.User

	// 查询用户
	err := s.db.QueryRow(`
		SELECT id, username, email, password, role, enabled, created_at, updated_at, last_login
		FROM users 
		WHERE username = ?
	`, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.Password, &user.Role, &user.Enabled, &user.CreatedAt, &user.UpdatedAt, &user.LastLogin,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("用户不存在: %s", username)
		}
		s.logger.Error("获取用户失败",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("获取用户失败: %w", err)
	}

	// 查询用户权限
	permissions, err := s.GetUserPermissions(user.ID)
	if err != nil {
		s.logger.Error("获取用户权限失败",
			zap.String("id", user.ID),
			zap.Error(err))
	} else {
		user.Permissions = permissions
	}

	return &user, nil
}

// UpdateUser 更新用户
func (s *UserService) UpdateUser(user *model.User) error {
	// 验证用户
	if err := s.validateUser(user); err != nil {
		return err
	}

	// 设置更新时间
	user.UpdatedAt = time.Now()

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 更新用户
	_, err = tx.Exec(`
		UPDATE users SET
			username = ?, email = ?, role = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`,
		user.Username, user.Email, user.Role, user.Enabled, user.UpdatedAt, user.ID,
	)

	if err != nil {
		tx.Rollback()
		s.logger.Error("更新用户失败",
			zap.String("id", user.ID),
			zap.Error(err))
		return fmt.Errorf("更新用户失败: %w", err)
	}

	// 删除旧的用户权限
	_, err = tx.Exec(`DELETE FROM user_permissions WHERE user_id = ?`, user.ID)
	if err != nil {
		tx.Rollback()
		s.logger.Error("删除用户权限失败",
			zap.String("id", user.ID),
			zap.Error(err))
		return fmt.Errorf("删除用户权限失败: %w", err)
	}

	// 插入新的用户权限
	if len(user.Permissions) > 0 {
		for _, permission := range user.Permissions {
			_, err = tx.Exec(`
				INSERT INTO user_permissions (user_id, permission_id, granted_at, granted_by)
				VALUES (?, ?, ?, ?)
			`,
				user.ID, permission, time.Now(), user.ID, // 更新者即为授权者
			)

			if err != nil {
				tx.Rollback()
				s.logger.Error("插入用户权限失败",
					zap.String("user_id", user.ID),
					zap.String("permission", permission),
					zap.Error(err))
				return fmt.Errorf("插入用户权限失败: %w", err)
			}
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		s.logger.Error("提交事务失败",
			zap.Error(err))
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.logger.Info("更新用户成功",
		zap.String("id", user.ID),
		zap.String("username", user.Username))

	return nil
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(id string) error {
	// 删除用户
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		s.logger.Error("删除用户失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("删除用户失败: %w", err)
	}

	s.logger.Info("删除用户成功",
		zap.String("id", id))

	return nil
}

// ListUsers 列出所有用户
func (s *UserService) ListUsers() ([]*model.User, error) {
	// 查询用户
	rows, err := s.db.Query(`
		SELECT id, username, email, role, enabled, created_at, updated_at, last_login
		FROM users
		ORDER BY username ASC
	`)

	if err != nil {
		s.logger.Error("查询用户失败",
			zap.Error(err))
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	defer rows.Close()

	// 收集用户
	var users []*model.User
	for rows.Next() {
		var user model.User

		if err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.Role, &user.Enabled, &user.CreatedAt, &user.UpdatedAt, &user.LastLogin,
		); err != nil {
			s.logger.Error("扫描用户失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描用户失败: %w", err)
		}

		users = append(users, &user)
	}

	// 查询每个用户的权限
	for _, user := range users {
		permissions, err := s.GetUserPermissions(user.ID)
		if err != nil {
			s.logger.Error("获取用户权限失败",
				zap.String("id", user.ID),
				zap.Error(err))
			continue
		}

		user.Permissions = permissions
	}

	return users, nil
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(id, oldPassword, newPassword string) error {
	// 获取用户
	var hashedPassword string
	err := s.db.QueryRow(`SELECT password FROM users WHERE id = ?`, id).Scan(&hashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("用户不存在: %s", id)
		}
		s.logger.Error("获取用户密码失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("获取用户密码失败: %w", err)
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(oldPassword)); err != nil {
		return fmt.Errorf("旧密码错误")
	}

	// 哈希新密码
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("哈希密码失败",
			zap.Error(err))
		return fmt.Errorf("哈希密码失败: %w", err)
	}

	// 更新密码
	_, err = s.db.Exec(`UPDATE users SET password = ?, updated_at = ? WHERE id = ?`,
		newHashedPassword, time.Now(), id)
	if err != nil {
		s.logger.Error("更新密码失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("更新密码失败: %w", err)
	}

	s.logger.Info("修改密码成功",
		zap.String("id", id))

	return nil
}

// ResetPassword 重置密码
func (s *UserService) ResetPassword(id, newPassword string) error {
	// 哈希新密码
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("哈希密码失败",
			zap.Error(err))
		return fmt.Errorf("哈希密码失败: %w", err)
	}

	// 更新密码
	_, err = s.db.Exec(`UPDATE users SET password = ?, updated_at = ? WHERE id = ?`,
		newHashedPassword, time.Now(), id)
	if err != nil {
		s.logger.Error("重置密码失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("重置密码失败: %w", err)
	}

	s.logger.Info("重置密码成功",
		zap.String("id", id))

	return nil
}

// UpdateLastLogin 更新最后登录时间
func (s *UserService) UpdateLastLogin(id string) error {
	// 更新最后登录时间
	_, err := s.db.Exec(`UPDATE users SET last_login = ? WHERE id = ?`,
		time.Now(), id)
	if err != nil {
		s.logger.Error("更新最后登录时间失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("更新最后登录时间失败: %w", err)
	}

	return nil
}

// GetUserPermissions 获取用户权限
func (s *UserService) GetUserPermissions(userID string) ([]string, error) {
	// 查询用户角色
	var role string
	err := s.db.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("用户不存在: %s", userID)
		}
		s.logger.Error("获取用户角色失败",
			zap.String("id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("获取用户角色失败: %w", err)
	}

	// 查询角色权限
	roleRows, err := s.db.Query(`
		SELECT p.id FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		WHERE rp.role = ?
	`, role)

	if err != nil {
		s.logger.Error("查询角色权限失败",
			zap.String("role", role),
			zap.Error(err))
		return nil, fmt.Errorf("查询角色权限失败: %w", err)
	}
	defer roleRows.Close()

	// 收集角色权限
	var permissions []string
	for roleRows.Next() {
		var permission string
		if err := roleRows.Scan(&permission); err != nil {
			s.logger.Error("扫描角色权限失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描角色权限失败: %w", err)
		}

		permissions = append(permissions, permission)
	}

	// 查询用户特定权限
	userRows, err := s.db.Query(`
		SELECT p.id FROM permissions p
		JOIN user_permissions up ON p.id = up.permission_id
		WHERE up.user_id = ?
	`, userID)

	if err != nil {
		s.logger.Error("查询用户权限失败",
			zap.String("id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("查询用户权限失败: %w", err)
	}
	defer userRows.Close()

	// 收集用户权限
	for userRows.Next() {
		var permission string
		if err := userRows.Scan(&permission); err != nil {
			s.logger.Error("扫描用户权限失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描用户权限失败: %w", err)
		}

		// 检查权限是否已存在
		exists := false
		for _, p := range permissions {
			if p == permission {
				exists = true
				break
			}
		}

		if !exists {
			permissions = append(permissions, permission)
		}
	}

	return permissions, nil
}

// HasPermission 检查用户是否有权限
func (s *UserService) HasPermission(userID, permission string) (bool, error) {
	permissions, err := s.GetUserPermissions(userID)
	if err != nil {
		return false, err
	}

	for _, p := range permissions {
		if p == permission || p == "perm_admin" {
			return true, nil
		}
	}

	return false, nil
}

// GrantProbePermission 授予探测权限
func (s *UserService) GrantProbePermission(userID, nodeID, groupID, grantedBy string, expiration time.Time) error {
	// 检查是否同时指定了节点和组
	if nodeID != "" && groupID != "" {
		return errors.New("不能同时指定节点和组")
	}

	// 检查是否未指定节点和组
	if nodeID == "" && groupID == "" {
		return errors.New("必须指定节点或组")
	}

	// 生成ID
	id := uuid.New().String()

	// 插入探测权限
	_, err := s.db.Exec(`
		INSERT INTO probe_permissions (id, user_id, node_id, group_id, granted_at, granted_by, expiration)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		id, userID, nodeID, groupID, time.Now(), grantedBy, expiration,
	)

	if err != nil {
		s.logger.Error("授予探测权限失败",
			zap.String("user_id", userID),
			zap.String("node_id", nodeID),
			zap.String("group_id", groupID),
			zap.Error(err))
		return fmt.Errorf("授予探测权限失败: %w", err)
	}

	s.logger.Info("授予探测权限成功",
		zap.String("user_id", userID),
		zap.String("node_id", nodeID),
		zap.String("group_id", groupID))

	return nil
}

// RevokeProbePermission 撤销探测权限
func (s *UserService) RevokeProbePermission(id string) error {
	// 删除探测权限
	_, err := s.db.Exec(`DELETE FROM probe_permissions WHERE id = ?`, id)
	if err != nil {
		s.logger.Error("撤销探测权限失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("撤销探测权限失败: %w", err)
	}

	s.logger.Info("撤销探测权限成功",
		zap.String("id", id))

	return nil
}

// GetUserProbePermissions 获取用户的探测权限
func (s *UserService) GetUserProbePermissions(userID string) ([]*model.ProbePermission, error) {
	// 查询探测权限
	rows, err := s.db.Query(`
		SELECT id, user_id, node_id, group_id, granted_at, granted_by, expiration
		FROM probe_permissions
		WHERE user_id = ?
	`, userID)

	if err != nil {
		s.logger.Error("查询用户探测权限失败",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("查询用户探测权限失败: %w", err)
	}
	defer rows.Close()

	// 收集探测权限
	var permissions []*model.ProbePermission
	for rows.Next() {
		var permission model.ProbePermission
		var nodeID, groupID sql.NullString
		var expiration sql.NullTime

		if err := rows.Scan(
			&permission.UserID, &nodeID, &groupID, &permission.GrantedAt, &permission.GrantedBy, &expiration,
		); err != nil {
			s.logger.Error("扫描探测权限失败",
				zap.Error(err))
			return nil, fmt.Errorf("扫描探测权限失败: %w", err)
		}

		if nodeID.Valid {
			permission.NodeID = nodeID.String
		}

		if groupID.Valid {
			permission.GroupID = groupID.String
		}

		if expiration.Valid {
			permission.Expiration = expiration.Time
		}

		permissions = append(permissions, &permission)
	}

	return permissions, nil
}

// CanProbe 检查用户是否有探测权限
func (s *UserService) CanProbe(userID, nodeID string) (bool, error) {
	// 查询用户角色
	var role string
	err := s.db.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("用户不存在: %s", userID)
		}
		s.logger.Error("获取用户角色失败",
			zap.String("id", userID),
			zap.Error(err))
		return false, fmt.Errorf("获取用户角色失败: %w", err)
	}

	// 管理员有所有权限
	if role == "admin" {
		return true, nil
	}

	// 检查用户是否有探测权限
	hasProbe, err := s.HasPermission(userID, "perm_probe")
	if err != nil {
		return false, err
	}

	if !hasProbe {
		return false, nil
	}

	// 查询节点所属的组
	rows, err := s.db.Query(`
		SELECT group_id FROM node_group_nodes WHERE node_id = ?
	`, nodeID)

	if err != nil {
		s.logger.Error("查询节点所属组失败",
			zap.String("node_id", nodeID),
			zap.Error(err))
		return false, fmt.Errorf("查询节点所属组失败: %w", err)
	}
	defer rows.Close()

	// 收集组ID
	var groupIDs []string
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			s.logger.Error("扫描组ID失败",
				zap.Error(err))
			return false, fmt.Errorf("扫描组ID失败: %w", err)
		}

		groupIDs = append(groupIDs, groupID)
	}

	// 检查是否有节点或组的探测权限
	var count int
	query := `
		SELECT COUNT(*) FROM probe_permissions
		WHERE user_id = ? AND (
			node_id = ? OR
			group_id IN (` + placeholders(len(groupIDs)) + `)
		) AND (expiration IS NULL OR expiration > ?)
	`

	// 构建参数
	args := make([]interface{}, 0, len(groupIDs)+3)
	args = append(args, userID, nodeID)
	for _, groupID := range groupIDs {
		args = append(args, groupID)
	}
	args = append(args, time.Now())

	err = s.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		s.logger.Error("查询探测权限失败",
			zap.String("user_id", userID),
			zap.String("node_id", nodeID),
			zap.Error(err))
		return false, fmt.Errorf("查询探测权限失败: %w", err)
	}

	return count > 0, nil
}

// validateUser 验证用户
func (s *UserService) validateUser(user *model.User) error {
	if user.Username == "" {
		return errors.New("用户名不能为空")
	}

	if user.Email == "" {
		return errors.New("邮箱不能为空")
	}

	if user.Role == "" {
		return errors.New("角色不能为空")
	}

	return nil
}

// WithUserClaims 将用户声明添加到上下文
func WithUserClaims(ctx context.Context, claims *UserClaims) context.Context {
	return context.WithValue(ctx, userClaimsKey, claims)
}

// GetUserClaimsFromContext 从上下文获取用户声明
func GetUserClaimsFromContext(ctx context.Context) *UserClaims {
	claims, ok := ctx.Value(userClaimsKey).(*UserClaims)
	if !ok {
		return nil
	}
	return claims
}

// userClaimsKey 用户声明上下文键
type userClaimsKeyType struct{}

var userClaimsKey = userClaimsKeyType{}

// placeholders 生成占位符
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}

	result := "?"
	for i := 1; i < n; i++ {
		result += ",?"
	}

	return result
}
