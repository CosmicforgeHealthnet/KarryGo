package customer

import "github.com/gin-gonic/gin"

type CustomerIdentity struct {
	UserID     interface{}
	CustomerID interface{}
	Role       string
}

func CustomerIdentityFromContext(c *gin.Context) CustomerIdentity {
	userID, _ := c.Get(ContextUserIDKey)
	customerID, _ := c.Get(ContextCustomerIDKey)
	role, _ := c.Get(ContextRoleKey)

	roleString, _ := role.(string)

	return CustomerIdentity{
		UserID:     userID,
		CustomerID: customerID,
		Role:       roleString,
	}
}
