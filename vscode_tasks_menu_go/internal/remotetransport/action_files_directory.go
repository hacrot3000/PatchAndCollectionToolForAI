package remotetransport

// Root-issued directory selection is an INDEX, never a remotely supplied
// directory name, relative path or traversal string. The next-page action
// has no arguments or remotely selected offset.
const RemoteActionFilesDirSelect="files.dir.select"
const RemoteActionFilesDirNext="files.dir.next"

// The same eight-item, 512-byte-bounded, names-only E2EE result envelope
// used by files.root is reused for a selected one-level directory. No
// file contents or paths are included in the result.
func ValidRemoteFilesDirSelection(index int)bool{
 return ValidRemoteFilesSelectIndex(index)
}
