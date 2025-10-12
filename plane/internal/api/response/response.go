package response

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Response API响应结构
type Response struct {
	Success   bool        `json:"success"`
	Code      int         `json:"code"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// JSON 返回JSON响应
func JSON(w http.ResponseWriter, statusCode int, data interface{}) {
	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// 构建响应
	resp := Response{
		Success:   statusCode >= 200 && statusCode < 300,
		Code:      statusCode,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	// 序列化响应
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Error 返回错误响应
func Error(w http.ResponseWriter, statusCode int, message string, err error) {
	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// 构建响应
	resp := Response{
		Success:   false,
		Code:      statusCode,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}

	// 添加错误信息
	if err != nil {
		resp.Error = err.Error()
	}

	// 序列化响应
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Success 返回成功响应
func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, data)
}

// Created 返回资源创建成功响应
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, data)
}

// NoContent 返回无内容响应
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// BadRequest 返回请求错误响应
func BadRequest(w http.ResponseWriter, message string, err error) {
	Error(w, http.StatusBadRequest, message, err)
}

// Unauthorized 返回未授权响应
func Unauthorized(w http.ResponseWriter, message string, err error) {
	Error(w, http.StatusUnauthorized, message, err)
}

// Forbidden 返回禁止访问响应
func Forbidden(w http.ResponseWriter, message string, err error) {
	Error(w, http.StatusForbidden, message, err)
}

// NotFound 返回资源不存在响应
func NotFound(w http.ResponseWriter, message string, err error) {
	Error(w, http.StatusNotFound, message, err)
}

// InternalServerError 返回服务器错误响应
func InternalServerError(w http.ResponseWriter, message string, err error) {
	Error(w, http.StatusInternalServerError, message, err)
}

// Gin兼容函数
// GinSuccess 返回成功响应（Gin版本）
func GinSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success:   true,
		Code:      http.StatusOK,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

// GinError 返回错误响应（Gin版本）
func GinError(c *gin.Context, statusCode int, message string, err error) {
	resp := Response{
		Success:   false,
		Code:      statusCode,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(statusCode, resp)
}

// GinBadRequest 返回请求错误响应（Gin版本）
func GinBadRequest(c *gin.Context, message string, err ...error) {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	GinError(c, http.StatusBadRequest, message, e)
}

// GinUnauthorized 返回未授权响应（Gin版本）
func GinUnauthorized(c *gin.Context, message string, err ...error) {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	GinError(c, http.StatusUnauthorized, message, e)
}

// GinForbidden 返回禁止访问响应（Gin版本）
func GinForbidden(c *gin.Context, message string, err ...error) {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	GinError(c, http.StatusForbidden, message, e)
}

// GinNotFound 返回资源不存在响应（Gin版本）
func GinNotFound(c *gin.Context, message string, err ...error) {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	GinError(c, http.StatusNotFound, message, e)
}

// GinInternalError 返回服务器错误响应（Gin版本）
func GinInternalError(c *gin.Context, message string, err error) {
	GinError(c, http.StatusInternalServerError, message, err)
}

// GinSuccessWithMessage 返回带消息的成功响应（Gin版本）
func GinSuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Success:   true,
		Code:      http.StatusOK,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

// InternalError 是GinInternalError的别名（可选错误参数）
func InternalError(c *gin.Context, message string, err ...error) {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	GinInternalError(c, message, e)
}

// SuccessWithMessage 是GinSuccessWithMessage的别名
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	GinSuccessWithMessage(c, message, data)
}
