//go:build !linux && !darwin

package server

import "os"

// Remote Agent application transport supports Linux/macOS. Other platforms
// refuse metadata-ambiguous entries instead of following unsafe paths.
func remotePinnedDirectoryEntryMode(_ *os.File,_ string)(os.FileMode,error){
 return os.ModeIrregular,nil
}
