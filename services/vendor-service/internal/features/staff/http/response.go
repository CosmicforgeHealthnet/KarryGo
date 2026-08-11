package staffhttp

import "github.com/gin-gonic/gin"

func respondOK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"success": true, "data": data})
}
