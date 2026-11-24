//go:build !windows

package discover

import (
	"os"
	"syscall"
)

const xOK = 0x1 // X_OK

// canExecute asks the kernel whether the current user may execute p, so the
// owner, group and ACLs are honored: a root-only tool isn't listed for other
// users.
func canExecute(p string, fi os.FileInfo) bool {
	if fi.Mode().Perm()&0o111 == 0 {
		return false
	}
	return syscall.Access(p, xOK) == nil
}
