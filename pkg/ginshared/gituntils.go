package ginshared

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func ReportBadrequest(c *gin.Context, err error) {
	errorDetails, ok := err.(validator.ValidationErrors)
	switch {
	case ok:
		errStrings := make([]string, len(errorDetails))
		for index, item := range errorDetails {
			errStrings[index] = fmt.Sprintf("%s, %s", item.Field(), item.Error())
		}
		c.JSON(http.StatusBadRequest, errStrings)
	default:
		c.JSON(http.StatusBadRequest, err.Error())
	}
}
