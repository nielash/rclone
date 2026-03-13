package bisync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rclone/rclone/fs"
	fslog "github.com/rclone/rclone/fs/log"
	"github.com/rclone/rclone/fs/rc"
	"github.com/rclone/rclone/lib/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBisyncRC(t *testing.T) {
	ctx, ci := fs.AddConfig(context.Background())
	assert.Equal(t, fs.TerminalColorModeAuto, ci.TerminalColorMode)

	out := testBisyncRc(ctx, t)

	// rc output should not have colors by default
	assert.NotContains(t, out["output"], terminal.MagentaFg)
}

func TestBisyncRCWithColors(t *testing.T) {
	ctx, ci := fs.AddConfig(context.Background())
	assert.NoError(t, ci.TerminalColorMode.Set("ALWAYS"))

	out := testBisyncRc(ctx, t)

	assert.Contains(t, out["output"], terminal.MagentaFg)
}

func testBisyncRc(ctx context.Context, t *testing.T) rc.Params {
	in := rc.Params{}
	path1 := filepath.Join(t.TempDir(), "path1")
	path2 := filepath.Join(t.TempDir(), "path2")
	in["path1"] = path1
	in["path2"] = path2
	in["resync"] = "true"
	require.NoError(t, os.MkdirAll(path1, 0777))
	require.NoError(t, os.MkdirAll(path2, 0777))
	require.NoError(t, os.WriteFile(path1+"/somefile.txt", []byte("hello"), 0777))
	require.NoError(t, os.WriteFile(path2+"/someotherfile.txt", []byte("hey there"), 0777))

	out, err := rc.Calls.Get("sync/bisync").Fn(ctx, in)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	// t.Log(out)

	assert.FileExists(t, path2+"/somefile.txt")
	assert.FileExists(t, path1+"/someotherfile.txt")

	assert.NotEmpty(t, out["output"])
	assert.NotEmpty(t, out["session"])
	assert.NotEmpty(t, out["workDir"])
	assert.NotEmpty(t, out["basePath"])
	assert.NotEmpty(t, out["listing1"])
	assert.NotEmpty(t, out["listing2"])
	assert.Equal(t, fslog.Opt.File, out["logFile"])

	return out
}
