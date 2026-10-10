package version

import (
	"golang.org/x/sys/windows/registry"
	"runtime"
)

const (
	urlWinExe    = smsReleasesPage
	urlWinExeArm = smsReleasesPage
)

var regKeyAppPath = "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\Netbird"

// DownloadUrl return with the proper download link
func DownloadUrl() string {
	_, err := registry.OpenKey(registry.LOCAL_MACHINE, regKeyAppPath, registry.QUERY_VALUE)
	if err != nil {
		return downloadURL
	}

	url := urlWinExe
	if runtime.GOARCH == "arm64" {
		url = urlWinExeArm
	}

	return url
}
