//go:build windows

package common

import "io/fs"

/*START ALPHA CHANGE 31 -- add Windows-safe file ownership verification implementation --*/
func verifyFileOwnerAndPermissions(_ string, _ fs.FileInfo, _ fs.FileMode) error {
	return nil
}

/*FINISH ALPHA CHANGE 31 -- add Windows-safe file ownership verification implementation --*/
