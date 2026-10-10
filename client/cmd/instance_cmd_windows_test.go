package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/profilemanager"
)

func TestMergeEnvReplacesByNameWithoutCase(t *testing.T) {
	base := []string{"PATH=C:\\x", "nb_state_dir=C:\\old", "OTHER=1"}

	got := mergeEnv(base, []string{"NB_STATE_DIR=C:\\new"})

	assert.Equal(t, []string{"PATH=C:\\x", "OTHER=1", "NB_STATE_DIR=C:\\new"}, got)
	assert.Equal(t, "nb_state_dir=C:\\old", base[1], "the input is not modified")
}

func TestPsQuote(t *testing.T) {
	assert.Equal(t, `'C:\Users\smoldyv\x.lnk'`, psQuote(`C:\Users\smoldyv\x.lnk`))
	assert.Equal(t, `'C:\Users\O''Brien'`, psQuote(`C:\Users\O'Brien`), "an apostrophe cannot end the string")
	assert.Equal(t, `'a; calc'`, psQuote(`a; calc`), "text stays text")
}

func TestLastLines(t *testing.T) {
	assert.Equal(t, "c | d", lastLines("a\nb\nc\nd\n", 2))
	assert.Equal(t, "only", lastLines("only", 5))
	assert.Equal(t, "", lastLines("  \n", 3))
}

func TestRemoveInstanceDirOnlyDeletesInstanceFolders(t *testing.T) {
	programData := t.TempDir()
	t.Setenv("PROGRAMDATA", programData)

	mustMake := func(rel string) string {
		p := filepath.Join(programData, rel)
		require.NoError(t, os.MkdirAll(p, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(p, "f.txt"), []byte("x"), 0o600))
		return p
	}

	own := mustMake("Netbird-private")
	require.NoError(t, removeInstanceDir(own))
	assert.NoDirExists(t, own)

	for _, rel := range []string{"Netbird", "Netbird-", "Other", `Netbird-x\Netbird-y`, "Microsoft"} {
		dir := mustMake(rel)
		assert.Error(t, removeInstanceDir(dir), "must refuse %s", rel)
		assert.DirExists(t, dir, "%s must still be there", rel)
	}

	assert.Error(t, removeInstanceDir(programData), "ProgramData itself")
	assert.Error(t, removeInstanceDir(filepath.Join(os.TempDir(), "Netbird-x")), "a folder outside ProgramData")
}

func TestReadProfileSettings(t *testing.T) {
	dir := t.TempDir()

	_, _, ok := readProfileSettings(dir)
	assert.False(t, ok, "no profile yet")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "default.json"), []byte(`{"WgIface":"wt3","WgPort":51823,"Other":1}`), 0o600))
	iface, port, ok := readProfileSettings(dir)
	require.True(t, ok)
	assert.Equal(t, "wt3", iface)
	assert.Equal(t, 51823, port)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "default.json"), []byte(`not json`), 0o600))
	_, _, ok = readProfileSettings(dir)
	assert.False(t, ok, "a broken file is not trusted")
}

func TestResolveProfileSource(t *testing.T) {
	programData := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(programData, "Netbird"), 0o755))
	def := filepath.Join(programData, "Netbird", "default.json")

	got, err := resolveProfileSource("", programData)
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = resolveProfileSource("default", programData)
	assert.Error(t, err, "the default profile does not exist yet")

	require.NoError(t, os.WriteFile(def, []byte(`{"WgIface":"wt0"}`), 0o600))
	got, err = resolveProfileSource("default", programData)
	require.NoError(t, err)
	assert.Equal(t, def, got)

	other := filepath.Join(programData, "other.json")
	require.NoError(t, os.WriteFile(other, []byte(`{}`), 0o600))
	got, err = resolveProfileSource(other, programData)
	require.NoError(t, err)
	assert.Equal(t, other, got)

	bad := filepath.Join(programData, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`not json`), 0o600))
	_, err = resolveProfileSource(bad, programData)
	assert.Error(t, err)
	_, err = resolveProfileSource(filepath.Join(programData, "missing.json"), programData)
	assert.Error(t, err)
}

// The copied profile keeps its identity, gets the new interface and port, and is
// written without a byte-order mark, which NetBird's own reader would reject.
func TestPrepareProfileKeepsIdentityAndSetsInterface(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "default.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))

	iface, port := "wt0", 51820
	_, err := profilemanager.UpdateOrCreateConfig(profilemanager.ConfigInput{ConfigPath: src, InterfaceName: &iface, WireguardPort: &port})
	require.NoError(t, err)

	dst := filepath.Join(dir, "Netbird-work", "default.json")
	require.NoError(t, prepareProfile(planStep{From: src, To: dst, Interface: "wt1", Port: 51821}))

	read := func(path string) (map[string]any, []byte) {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		return m, data
	}

	srcCfg, _ := read(src)
	dstCfg, dstRaw := read(dst)

	assert.Equal(t, "wt1", dstCfg["WgIface"])
	assert.EqualValues(t, 51821, dstCfg["WgPort"])
	assert.NotEmpty(t, dstCfg["PrivateKey"])
	assert.Equal(t, srcCfg["PrivateKey"], dstCfg["PrivateKey"], "the copy is the same identity")
	assert.False(t, bytes.HasPrefix(dstRaw, []byte{0xEF, 0xBB, 0xBF}), "no byte-order mark")
	assert.True(t, strings.HasPrefix(string(dstRaw), "{"))

	srcCfg2, _ := read(src)
	assert.Equal(t, "wt0", srcCfg2["WgIface"], "the source is not touched")
}

func TestRunStepWithUnknownKind(t *testing.T) {
	err := runStep(&bytes.Buffer{}, "x.exe", planStep{Kind: stepKind(99)})
	assert.Error(t, err)
}
