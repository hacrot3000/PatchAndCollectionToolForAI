package server

import (
	"errors"
	"strings"
)

func normalizeFileTransferArchiveMergePolicy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = "fail"
	}
	switch value {
	case "fail", "overwrite":
		return value, nil
	default:
		return "", errors.New("archive merge policy must be fail or overwrite")
	}
}

func fileTransferManualExtractCommandsWithPolicy(format, archivePath, destination string, roots []string, mergePolicy string) map[string]string {
	mergePolicy, err := normalizeFileTransferArchiveMergePolicy(mergePolicy)
	if err != nil || mergePolicy == "fail" {
		return fileTransferManualExtractCommands(format, archivePath, destination, roots)
	}

	archiveQ := remoteArchiveShellQuote(archivePath)
	destQ := remoteArchiveShellQuote(destination)
	posixExtract := "tar -xzf " + archiveQ + " -C " + destQ + " --no-same-owner --no-same-permissions"
	if format == "zip" {
		posixExtract = "unzip -oq -- " + archiveQ + " -d " + destQ
	}
	posix := "set -eu; test -d " + destQ + "; " + posixExtract + " && rm -f -- " + archiveQ

	psArchive := fileTransferPowerShellQuote(archivePath)
	psDest := fileTransferPowerShellQuote(destination)
	psExtract := "tar -xzf $archive -C $dest; if ($LASTEXITCODE -ne 0) { throw 'tar extraction failed' }"
	if format == "zip" {
		psExtract = "Expand-Archive -LiteralPath $archive -DestinationPath $dest -Force -ErrorAction Stop"
	}
	powershell := "$archive=" + psArchive + "; $dest=" + psDest +
		"; if (-not (Test-Path -LiteralPath $dest -PathType Container)) { throw 'Destination directory not found' }; " +
		psExtract + "; Remove-Item -LiteralPath $archive"
	return map[string]string{"posix": posix, "powershell": powershell}
}

func remoteArchiveExtractCommandWithPolicy(format, archivePath, destination string, roots []string, mergePolicy string) (string, error) {
	mergePolicy, err := normalizeFileTransferArchiveMergePolicy(mergePolicy)
	if err != nil {
		return "", err
	}
	if mergePolicy == "fail" {
		return remoteArchiveExtractIntoExistingCommand(format, archivePath, destination, roots)
	}
	if format != "tar.gz" && format != "zip" {
		return "", errors.New("unsupported remote archive format")
	}
	return fileTransferManualExtractCommandsWithPolicy(format, archivePath, destination, roots, mergePolicy)["posix"], nil
}
