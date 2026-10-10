package version

import (
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	// smsRepo is the GitHub repository whose releases this build is updated from.
	smsRepo = "annabellekurka-dev/netbird"

	// smsVersionURL answers with the latest published release of the fork.
	smsVersionURL = "https://api.github.com/repos/" + smsRepo + "/releases/latest"

	// smsReleasesPage is where a user downloads a new release.
	smsReleasesPage = "https://github.com/" + smsRepo + "/releases/latest"

	// maxVersionResponse bounds the answer we read. A release document is a few
	// kilobytes; the plain-text endpoint this replaced answered with a few bytes.
	maxVersionResponse = 1 << 20
)

// smsVersionFromBody extracts the version from the answer of the version
// endpoint. A GitHub release document yields its tag without the leading "v"
// and without a suffix ("v0.82.0-SMS.1" gives "0.82.0"); anything else is taken
// to be the version as plain text.
func smsVersionFromBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if !strings.HasPrefix(text, "{") {
		return text
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return ""
	}

	tag := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if i := strings.IndexAny(tag, "-+"); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

// smsNoRelease reports whether the repository has no release yet, which is the
// normal state before the first one is published and not worth an error.
func smsNoRelease(resp *http.Response) bool {
	if resp.StatusCode != http.StatusNotFound {
		return false
	}
	log.Debugf("no release published at %s yet", smsVersionURL)
	return true
}
