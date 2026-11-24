//go:build windows

package discover

import "os"

// canExecute trusts the file extension on Windows, which commandName has
// already checked against PATHEXT.
func canExecute(string, os.FileInfo) bool { return true }
