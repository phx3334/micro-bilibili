package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int         `json:"code"`
	Data interface{} `json:"data"`
	Msg  string      `json:"msg"`
}

const (
	ERROR   = 4
	SUCCESS = 3
)

func Result(code int, data interface{}, msg string, c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Data: data,
		Msg:  msg,
	})
}

func OkWithMsg(c *gin.Context, msg string) {
	Result(SUCCESS, nil, msg, c)
}

func OkWithData(c *gin.Context, data interface{}) {
	Result(SUCCESS, data, "success", c)
}

// OkWithDetail 返回成功响应，包含自定义数据和消息。
// 命名与 OkWithData / OkWithMsg 保持一致（With 首字母大写）。
func OkWithDetail(c *gin.Context, data interface{}, msg string) {
	Result(SUCCESS, data, msg, c)
}

func FailWithMsg(c *gin.Context, msg string) {
	Result(ERROR, nil, msg, c)
}

func NoAuth(message string, c *gin.Context) {
	Result(ERROR, gin.H{"reload": true}, message, c)
}
