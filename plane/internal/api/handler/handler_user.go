package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/model"
	"gkipass/plane/internal/service"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
	logger      *zap.Logger
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      zap.L().Named("user-handler"),
	}
}

// RegisterRoutes 注册路由
func (h *UserHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/users", h.ListUsers).Methods("GET")
	r.HandleFunc("/users", h.CreateUser).Methods("POST")
	r.HandleFunc("/users/{id}", h.GetUser).Methods("GET")
	r.HandleFunc("/users/{id}", h.UpdateUser).Methods("PUT")
	r.HandleFunc("/users/{id}", h.DeleteUser).Methods("DELETE")
	r.HandleFunc("/users/{id}/password", h.ChangePassword).Methods("PUT")
	r.HandleFunc("/users/{id}/password/reset", h.ResetPassword).Methods("POST")
	r.HandleFunc("/users/{id}/permissions", h.GetUserPermissions).Methods("GET")
	r.HandleFunc("/users/{id}/probe-permissions", h.GetUserProbePermissions).Methods("GET")
	r.HandleFunc("/users/{id}/probe-permissions", h.GrantProbePermission).Methods("POST")
	r.HandleFunc("/users/{id}/probe-permissions/{permission_id}", h.RevokeProbePermission).Methods("DELETE")
}

// ListUsers 列出所有用户
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	query := r.URL.Query()
	role := query.Get("role")
	enabled := query.Get("enabled")

	// 获取用户
	users, err := h.userService.ListUsers()
	if err != nil {
		h.logger.Error("列出用户失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取用户列表失败", err)
		return
	}

	// 过滤用户
	var filteredUsers []*model.User
	for _, user := range users {
		// 过滤角色
		if role != "" && string(user.Role) != role {
			continue
		}

		// 过滤启用状态
		if enabled != "" {
			enabledBool, err := strconv.ParseBool(enabled)
			if err == nil && user.Enabled != enabledBool {
				continue
			}
		}

		filteredUsers = append(filteredUsers, user)
	}

	response.Success(w, filteredUsers)
}

// CreateUser 创建用户
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 检查权限
	hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
	if err != nil {
		h.logger.Error("检查权限失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
		return
	}

	if !hasPermission {
		response.Forbidden(w, "没有用户管理权限", nil)
		return
	}

	// 解析请求体
	var user model.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 创建用户
	if err := h.userService.CreateUser(&user); err != nil {
		h.logger.Error("创建用户失败",
			zap.String("username", user.Username),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "创建用户失败", err)
		return
	}

	// 清除密码
	user.Password = ""

	response.Created(w, user)
}

// GetUser 获取用户
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 获取用户
	user, err := h.userService.GetUser(id)
	if err != nil {
		h.logger.Error("获取用户失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusNotFound, "用户不存在", err)
		return
	}

	response.Success(w, user)
}

// UpdateUser 更新用户
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	if claims.UserID != id {
		hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
		if err != nil {
			h.logger.Error("检查权限失败", zap.Error(err))
			response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
			return
		}

		if !hasPermission {
			response.Forbidden(w, "没有用户管理权限", nil)
			return
		}
	}

	// 解析请求体
	var user model.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 确保ID一致
	user.ID = id

	// 更新用户
	if err := h.userService.UpdateUser(&user); err != nil {
		h.logger.Error("更新用户失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "更新用户失败", err)
		return
	}

	response.Success(w, user)
}

// DeleteUser 删除用户
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
	if err != nil {
		h.logger.Error("检查权限失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
		return
	}

	if !hasPermission {
		response.Forbidden(w, "没有用户管理权限", nil)
		return
	}

	// 不能删除自己
	if claims.UserID == id {
		response.Error(w, http.StatusBadRequest, "不能删除自己", nil)
		return
	}

	// 删除用户
	if err := h.userService.DeleteUser(id); err != nil {
		h.logger.Error("删除用户失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "删除用户失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "用户已删除",
		"id":      id,
	})
}

// ChangePassword 修改密码
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	if claims.UserID != id {
		hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
		if err != nil {
			h.logger.Error("检查权限失败", zap.Error(err))
			response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
			return
		}

		if !hasPermission {
			response.Forbidden(w, "没有用户管理权限", nil)
			return
		}
	}

	// 解析请求体
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 修改密码
	if err := h.userService.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		h.logger.Error("修改密码失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "修改密码失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "密码已修改",
		"id":      id,
	})
}

// ResetPassword 重置密码
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
	if err != nil {
		h.logger.Error("检查权限失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
		return
	}

	if !hasPermission {
		response.Forbidden(w, "没有用户管理权限", nil)
		return
	}

	// 解析请求体
	var req struct {
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 重置密码
	if err := h.userService.ResetPassword(id, req.NewPassword); err != nil {
		h.logger.Error("重置密码失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "重置密码失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "密码已重置",
		"id":      id,
	})
}

// GetUserPermissions 获取用户权限
func (h *UserHandler) GetUserPermissions(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	if claims.UserID != id {
		hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
		if err != nil {
			h.logger.Error("检查权限失败", zap.Error(err))
			response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
			return
		}

		if !hasPermission {
			response.Forbidden(w, "没有用户管理权限", nil)
			return
		}
	}

	// 获取用户权限
	permissions, err := h.userService.GetUserPermissions(id)
	if err != nil {
		h.logger.Error("获取用户权限失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取用户权限失败", err)
		return
	}

	response.Success(w, permissions)
}

// GetUserProbePermissions 获取用户探测权限
func (h *UserHandler) GetUserProbePermissions(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	if claims.UserID != id {
		hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
		if err != nil {
			h.logger.Error("检查权限失败", zap.Error(err))
			response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
			return
		}

		if !hasPermission {
			response.Forbidden(w, "没有用户管理权限", nil)
			return
		}
	}

	// 获取用户探测权限
	permissions, err := h.userService.GetUserProbePermissions(id)
	if err != nil {
		h.logger.Error("获取用户探测权限失败",
			zap.String("id", id),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "获取用户探测权限失败", err)
		return
	}

	response.Success(w, permissions)
}

// GrantProbePermission 授予探测权限
func (h *UserHandler) GrantProbePermission(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]

	// 检查权限
	hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
	if err != nil {
		h.logger.Error("检查权限失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
		return
	}

	if !hasPermission {
		response.Forbidden(w, "没有用户管理权限", nil)
		return
	}

	// 解析请求体
	var req struct {
		NodeID     string `json:"node_id"`
		GroupID    string `json:"group_id"`
		Expiration string `json:"expiration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("解析请求体失败", zap.Error(err))
		response.Error(w, http.StatusBadRequest, "无效的请求数据", err)
		return
	}

	// 解析过期时间
	var expiration time.Time
	if req.Expiration != "" {
		var err error
		expiration, err = time.Parse(time.RFC3339, req.Expiration)
		if err != nil {
			h.logger.Error("解析过期时间失败", zap.Error(err))
			response.Error(w, http.StatusBadRequest, "无效的过期时间格式", err)
			return
		}
	}

	// 授予探测权限
	if err := h.userService.GrantProbePermission(id, req.NodeID, req.GroupID, claims.UserID, expiration); err != nil {
		h.logger.Error("授予探测权限失败",
			zap.String("user_id", id),
			zap.String("node_id", req.NodeID),
			zap.String("group_id", req.GroupID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "授予探测权限失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message":    "探测权限已授予",
		"user_id":    id,
		"node_id":    req.NodeID,
		"group_id":   req.GroupID,
		"expiration": expiration,
	})
}

// RevokeProbePermission 撤销探测权限
func (h *UserHandler) RevokeProbePermission(w http.ResponseWriter, r *http.Request) {
	// 获取当前用户
	claims := service.GetUserClaimsFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w, "未认证", nil)
		return
	}

	// 获取路径参数
	vars := mux.Vars(r)
	id := vars["id"]
	permissionID := vars["permission_id"]

	// 检查权限
	hasPermission, err := h.userService.HasPermission(claims.UserID, "perm_user_manage")
	if err != nil {
		h.logger.Error("检查权限失败", zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "检查权限失败", err)
		return
	}

	if !hasPermission && claims.UserID != id {
		response.Forbidden(w, "没有用户管理权限", nil)
		return
	}

	// 撤销探测权限
	if err := h.userService.RevokeProbePermission(permissionID); err != nil {
		h.logger.Error("撤销探测权限失败",
			zap.String("id", permissionID),
			zap.Error(err))
		response.Error(w, http.StatusInternalServerError, "撤销探测权限失败", err)
		return
	}

	response.Success(w, map[string]interface{}{
		"message": "探测权限已撤销",
		"id":      permissionID,
	})
}
