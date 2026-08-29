package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// R 统一响应结构
type R struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PageResult 分页结果
type PageResult struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

func json(c *gin.Context, code int, msg string, data interface{}) {
	c.JSON(code, R{Code: code, Message: msg, Data: data})
}

// OK 200
func OK(c *gin.Context, data interface{}) {
	json(c, http.StatusOK, "ok", data)
}

// Created 201
func Created(c *gin.Context, data interface{}) {
	json(c, http.StatusCreated, "created", data)
}

// BadRequest 400
func BadRequest(c *gin.Context, msg string) {
	json(c, http.StatusBadRequest, msg, nil)
}

// Unauthorized 401
func Unauthorized(c *gin.Context, msg string) {
	json(c, http.StatusUnauthorized, msg, nil)
}

// Forbidden 403
func Forbidden(c *gin.Context, msg string) {
	json(c, http.StatusForbidden, msg, nil)
}

// NotFound 404
func NotFound(c *gin.Context, msg string) {
	json(c, http.StatusNotFound, msg, nil)
}

// Conflict 409
func Conflict(c *gin.Context, msg string) {
	json(c, http.StatusConflict, msg, nil)
}

// InternalError 500
func InternalError(c *gin.Context, msg string) {
	json(c, http.StatusInternalServerError, msg, nil)
}
