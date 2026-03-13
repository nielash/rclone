package terminal

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/rclone/rclone/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// see also bisync rc tests in /cmd/bisync/bisync_rc_test.go

const (
	msgWithoutColors = "potato"
	msgWithColors    = MagentaFg + msgWithoutColors + Reset
)

type MockTerm struct {
	isTerminal bool
	f          *os.File
}

func (m MockTerm) IsTerminal(fd int) bool {
	return m.isTerminal
}

func (m MockTerm) F() *os.File {
	return m.f
}

func TestTerminalColorsTrue(t *testing.T) {
	once = sync.Once{}
	mock := MockTerm{isTerminal: true}
	require.True(t, mock.IsTerminal(int(os.Stdout.Fd())))

	var err error
	mock.f, err = os.CreateTemp(t.TempDir(), "file1")
	assert.NoError(t, err)
	defaultTerminal = mock

	ci := fs.GetConfig(context.Background())

	Start()
	assert.True(t, ShouldUseColors(ci))

	WriteString(msgWithColors)

	b, err := os.ReadFile(defaultTerminal.F().Name())
	assert.NoError(t, err)
	assert.NoError(t, defaultTerminal.F().Close())
	assert.Contains(t, string(b), msgWithColors)
}

func TestTerminalColorsJSONLog(t *testing.T) {
	once = sync.Once{}
	mock := MockTerm{isTerminal: true}
	require.True(t, mock.IsTerminal(int(os.Stdout.Fd())))

	var err error
	mock.f, err = os.CreateTemp(t.TempDir(), "file2")
	assert.NoError(t, err)
	defaultTerminal = mock

	ci := fs.GetConfig(context.Background())
	ci.UseJSONLog = true
	defer func() { fs.GetConfig(context.Background()).UseJSONLog = false }() // restore the prior value after the test

	Start()
	assert.False(t, ShouldUseColors(ci))

	WriteString(msgWithColors)

	b, err := os.ReadFile(defaultTerminal.F().Name())
	assert.NoError(t, err)
	assert.NoError(t, defaultTerminal.F().Close())
	assert.Contains(t, string(b), msgWithoutColors)
	assert.NotContains(t, string(b), msgWithColors)
}

func TestColorFlag(t *testing.T) {
	once = sync.Once{}
	mock := MockTerm{isTerminal: true}
	require.True(t, mock.IsTerminal(int(os.Stdout.Fd())))

	var err error
	mock.f, err = os.CreateTemp(t.TempDir(), "file3")
	assert.NoError(t, err)
	defaultTerminal = mock

	ci := fs.GetConfig(context.Background())

	Start()

	defer func() { _ = fs.GetConfig(context.Background()).TerminalColorMode.Set("AUTO") }() // restore the prior value after the test
	assert.NoError(t, ci.TerminalColorMode.Set("ALWAYS"))
	assert.True(t, ShouldUseColors(ci))

	assert.NoError(t, ci.TerminalColorMode.Set("AUTO"))
	assert.True(t, ShouldUseColors(ci))

	assert.NoError(t, ci.TerminalColorMode.Set("NEVER"))
	assert.False(t, ShouldUseColors(ci))
}

func TestNonTerminal(t *testing.T) {
	once = sync.Once{}
	mock := MockTerm{isTerminal: false}
	require.False(t, mock.IsTerminal(int(os.Stdout.Fd())))

	var err error
	mock.f, err = os.CreateTemp(t.TempDir(), "file4")
	assert.NoError(t, err)
	defaultTerminal = mock

	ci := fs.GetConfig(context.Background())

	Start()

	defer func() { _ = fs.GetConfig(context.Background()).TerminalColorMode.Set("AUTO") }() // restore the prior value after the test
	assert.NoError(t, ci.TerminalColorMode.Set("ALWAYS"))
	assert.True(t, ShouldUseColors(ci))

	assert.NoError(t, ci.TerminalColorMode.Set("AUTO"))
	assert.False(t, ShouldUseColors(ci))

	assert.NoError(t, ci.TerminalColorMode.Set("NEVER"))
	assert.False(t, ShouldUseColors(ci))
}
