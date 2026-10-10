package version

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMSVersionFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"plain text", "10.0.0", "10.0.0"},
		{"plain text with newline", "0.81.0\n", "0.81.0"},
		{"release tag", `{"tag_name":"v0.82.0"}`, "0.82.0"},
		{"release tag with suffix", `{"tag_name":"v0.82.0-SMS.1","name":"x"}`, "0.82.0"},
		{"release tag with metadata", `{"tag_name":"0.82.0+SMS"}`, "0.82.0"},
		{"release document with other fields", `{"url":"u","tag_name":" v1.2.3 ","draft":false,"assets":[{"name":"a"}]}`, "1.2.3"},
		{"no tag", `{"message":"Not Found"}`, ""},
		{"broken json", `{"tag_name":`, ""},
		{"empty", ``, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, smsVersionFromBody([]byte(tc.body)))
		})
	}
}

// restoreUpdateState puts the package variables the update tests change back.
func restoreUpdateState(t *testing.T) {
	oldVersion, oldURL := version, versionURL
	t.Cleanup(func() { version, versionURL = oldVersion, oldURL })
}

func serveRelease(t *testing.T, status int, body string) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(svr.Close)
	versionURL = svr.URL
}

func TestSMSNewerReleaseIsReported(t *testing.T) {
	restoreUpdateState(t)
	version = "0.80.0+SMS"
	serveRelease(t, http.StatusOK, `{"tag_name":"v0.82.0-SMS.1"}`)

	u := NewUpdate(httpAgent)
	require.True(t, u.fetchVersion())
	require.NotNil(t, u.LatestVersion())
	assert.Equal(t, "0.82.0", u.LatestVersion().String())
	assert.True(t, u.isUpdateAvailable())
}

// The label after "+" is build metadata and must not make a build look older or
// newer than the release it was built from.
func TestSMSSameReleaseIsNotReported(t *testing.T) {
	restoreUpdateState(t)
	version = "0.80.0+SMS"
	serveRelease(t, http.StatusOK, `{"tag_name":"v0.80.0-SMS.1"}`)

	u := NewUpdate(httpAgent)
	require.True(t, u.fetchVersion())
	assert.False(t, u.isUpdateAvailable())
}

func TestSMSOlderReleaseIsNotReported(t *testing.T) {
	restoreUpdateState(t)
	version = "0.82.0+SMS"
	serveRelease(t, http.StatusOK, `{"tag_name":"v0.80.0"}`)

	u := NewUpdate(httpAgent)
	require.True(t, u.fetchVersion())
	assert.False(t, u.isUpdateAvailable())
}

func TestSMSNoReleaseYetIsQuiet(t *testing.T) {
	restoreUpdateState(t)
	version = "0.80.0+SMS"
	serveRelease(t, http.StatusNotFound, `{"message":"Not Found"}`)

	u := NewUpdate(httpAgent)
	assert.False(t, u.fetchVersion())
	assert.Nil(t, u.LatestVersion())
	assert.False(t, u.isUpdateAvailable())
}

func TestSMSDownloadPointsAtTheFork(t *testing.T) {
	assert.Contains(t, DownloadUrl(), "github.com/annabellekurka-dev/netbird/releases")
	assert.NotContains(t, DownloadUrl(), "netbird.io")
	assert.NotContains(t, smsVersionURL, "netbird.io")
}
