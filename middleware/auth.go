package middleware

import (
	"groupbuy/response"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func LeaderAuth() gin.HandlerFunc {
	secret := os.Getenv("LEADER_SECRET")
	if secret == "" {
		secret = "tuanzhang2024"
	}
	return func(c *gin.Context) {
		token := c.GetHeader("X-Leader-Token")
		if token == "" {
			token = c.Query("leader_token")
		}
		if token != secret {
			response.Fail(c, response.CodeForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}

func NeighborAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		phone := c.GetHeader("X-Phone")
		if phone == "" {
			phone = c.Query("phone")
		}
		if phone == "" {
			var body struct {
				Phone string `json:"phone"`
			}
			if c.Request.Method != "GET" && c.Request.Method != "DELETE" {
				_ = c.ShouldBindJSON(&body)
				if body.Phone != "" {
					phone = body.Phone
				}
			}
		}
		phone = strings.TrimSpace(phone)
		if phone == "" || len(phone) < 7 {
			response.Fail(c, response.CodeUnauthorized)
			c.Abort()
			return
		}
		c.Set("phone", phone)
		c.Next()
	}
}

func GetPhone(c *gin.Context) string {
	phone, _ := c.Get("phone")
	if p, ok := phone.(string); ok {
		return p
	}
	return ""
}
