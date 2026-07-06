package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
)

func Errors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		e := c.Errors.Last().Err
		status := apperr.HTTPStatus(e)
		code := "internal"
		ae, ok := e.(*apperr.Error)
		if ok {
			code = ae.Code
		}
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":       code,
				"message":    e.Error(),
				"request_id": c.GetString(RequestIDKey),
			},
		})
	}
}
