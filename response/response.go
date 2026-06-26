package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Code int

const (
	CodeSuccess         Code = 0
	CodeParamError      Code = 10001
	CodeUnauthorized    Code = 10002
	CodeForbidden       Code = 10003
	CodeNotFound        Code = 10004
	CodeCutoffPassed    Code = 10005
	CodeProductOffShelf Code = 10006
	CodeOrderCancelled  Code = 10007
	CodeInternalError   Code = 50000
)

var codeMessages = map[Code]string{
	CodeSuccess:         "ok",
	CodeParamError:      "参数错误",
	CodeUnauthorized:    "未授权，请提供有效鉴权信息",
	CodeForbidden:       "无权限执行此操作",
	CodeNotFound:        "资源不存在",
	CodeCutoffPassed:    "已过截团时间，无法下单或修改",
	CodeProductOffShelf: "商品已下架",
	CodeOrderCancelled:  "订单已撤销，无法操作",
	CodeInternalError:   "服务器内部错误",
}

type R struct {
	Code    Code        `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, R{Code: CodeSuccess, Message: codeMessages[CodeSuccess], Data: data})
}

func Fail(c *gin.Context, code Code) {
	msg, ok := codeMessages[code]
	if !ok {
		msg = "未知错误"
	}
	c.JSON(http.StatusOK, R{Code: code, Message: msg})
}

func FailWithMsg(c *gin.Context, code Code, msg string) {
	c.JSON(http.StatusOK, R{Code: code, Message: msg})
}
