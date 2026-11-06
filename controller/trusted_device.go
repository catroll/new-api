package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetTrustedDevices lists the browsers that may request an SMS login code for
// the account's phone without solving the image challenge.
func GetTrustedDevices(c *gin.Context) {
	userID := c.GetInt("id")
	devices, err := service.ListTrustedDevices(userID, service.TrustedDeviceIDFromRequest(c))
	if err != nil {
		writeTrustedDeviceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": devices})
}

// DeleteTrustedDevice revokes one device. Revoking the current device also
// clears its cookie so the browser does not keep sending a dead token.
func DeleteTrustedDevice(c *gin.Context) {
	userID := c.GetInt("id")
	deviceID := strings.TrimSpace(c.Param("id"))
	if err := service.RevokeTrustedDevice(userID, deviceID); err != nil {
		writeTrustedDeviceError(c, err)
		return
	}
	current := service.TrustedDeviceIDFromRequest(c) == deviceID
	if current {
		service.ClearTrustedDeviceCookie(c)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"revoked": deviceID, "current": current}})
}

// RevokeAllTrustedDevices forgets every device for the account.
func RevokeAllTrustedDevices(c *gin.Context) {
	userID := c.GetInt("id")
	if err := service.RevokeAllTrustedDevices(userID); err != nil {
		writeTrustedDeviceError(c, err)
		return
	}
	service.ClearTrustedDeviceCookie(c)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func writeTrustedDeviceError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrTrustedDeviceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "trusted device not found"})
		return
	}
	if errors.Is(err, service.ErrTrustedDeviceLimit) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "too many trusted devices"})
		return
	}
	common.SysLog(common.LogText("trusted device error: %v", err))
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "trusted device operation failed"})
}
