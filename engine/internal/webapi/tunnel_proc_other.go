//go:build !linux

package webapi

// tunnelProcessRunning вне Linux не проверяется: туннель считается выключенным.
func tunnelProcessRunning() bool {
	return false
}
