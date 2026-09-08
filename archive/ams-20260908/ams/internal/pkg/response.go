package pkg

import "github.com/gin-gonic/gin"

// Response 统一响应格式
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	TraceID string      `json:"trace_id,omitempty"`
}

// PageResult 分页结果
type PageResult struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Code:    0,
		Message: "ok",
		Data:    data,
		TraceID: c.GetString("trace_id"),
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(201, Response{
		Code:    0,
		Message: "created",
		Data:    data,
		TraceID: c.GetString("trace_id"),
	})
}

func BadRequest(c *gin.Context, msg string) {
	c.JSON(400, Response{
		Code:    400,
		Message: msg,
		TraceID: c.GetString("trace_id"),
	})
}

func Unauthorized(c *gin.Context, msg string) {
	c.JSON(401, Response{
		Code:    401,
		Message: msg,
		TraceID: c.GetString("trace_id"),
	})
}

func Forbidden(c *gin.Context, msg string) {
	c.JSON(403, Response{
		Code:    403,
		Message: msg,
		TraceID: c.GetString("trace_id"),
	})
}

func NotFound(c *gin.Context, msg string) {
	c.JSON(404, Response{
		Code:    404,
		Message: msg,
		TraceID: c.GetString("trace_id"),
	})
}

func InternalError(c *gin.Context, msg string) {
	c.JSON(500, Response{
		Code:    500,
		Message: msg,
		TraceID: c.GetString("trace_id"),
	})
}
