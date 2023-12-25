// Package profiles handles presets for config
package profiles_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"testing"

	_ "github.com/rclone/rclone/backend/all" // import all backends
	"github.com/rclone/rclone/cmd"
	_ "github.com/rclone/rclone/cmd/all" // import all commands
	"github.com/rclone/rclone/cmd/bisync"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	profiles "github.com/rclone/rclone/fs/config/configprofiles"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fstest"
	_ "github.com/rclone/rclone/lib/plugin" // import plugins
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx context.Context

func TestMain(m *testing.M) {
	fstest.TestMain(m)
}

func testSaving(t *testing.T, Cmd, expected string) {
	tempConfig(t)
	r := fstest.NewRun(t)
	r.WriteFile("path1", "hello world", t1)
	r.Mkdir(ctx, r.Fremote)
	path1, path2 := r.Flocal.Root(), r.Fremote.Root()
	section := "profiletest"

	Cmd = fmt.Sprintf(Cmd, path1, path2, section)

	setCmdArgs(Cmd)
	setup()
	defer cleanup()

	ci := fs.GetConfig(ctx)
	assert.Equal(t, "", ci.SaveProfile)
	assert.False(t, config.LoadedData().HasSection(section))

	profiles.AddProfiles(ctx, Root)

	assert.Equal(t, section, ci.SaveProfile)
	assert.True(t, config.LoadedData().HasSection(section))

	if strings.Contains(Cmd, "--profile-include-args") {
		expected = strings.Replace(expected, "{{arg1}}", path1, 1)
		expected = strings.Replace(expected, "{{arg2}}", path2, 1)
	}

	assert.Equal(t, normalize(expected), normalize(profiles.SprintSection(section)))
}

func TestSaveProfile(t *testing.T) {
	Cmd := `rclone bisync %s %s --check-access --max-delete 10 --checkers=16 --drive-pacer-min-sleep=11ms --resilient --no-cleanup -MvvP --drive-skip-gdocs --stats-file-name-length 100 --create-empty-src-dirs --resync --save-profile %s`

	expected := `[profiletest]
type = profile
check_access = true
max_delete = 10
no_cleanup = true
progress = true
verbose = 2
stats_file_name_length = 100
checkers = 16
create_empty_src_dirs = true
drive_pacer_min_sleep = 11ms
drive_skip_gdocs = true
metadata = true
resilient = true
resync = true`

	testSaving(t, Cmd, expected)
}

func TestSaveProfileWithArgs(t *testing.T) {
	Cmd := `rclone bisync %s %s --check-access --max-delete 10 --checkers=16 --drive-pacer-min-sleep=11ms --resilient --no-cleanup -MvvP --drive-skip-gdocs --stats-file-name-length 100 --create-empty-src-dirs --resync --save-profile %s --profile-include-args`

	expected := `[profiletest]
type = profile
check_access = true
max_delete = 10
no_cleanup = true
progress = true
verbose = 2
stats_file_name_length = 100
checkers = 16
create_empty_src_dirs = true
drive_pacer_min_sleep = 11ms
drive_skip_gdocs = true
metadata = true
resilient = true
resync = true
arg_1 = {{arg1}}
arg_2 = {{arg2}}`

	testSaving(t, Cmd, expected)
}

func TestUsingProfile(t *testing.T) {
	// first save a profile
	tempConfig(t)
	r := fstest.NewRun(t)
	r.WriteFile("path1", "hello world", t1)
	r.Mkdir(ctx, r.Fremote)
	path1, path2 := r.Flocal.Root(), r.Fremote.Root()
	section := "profiletest"

	Cmd := `rclone bisync %s %s --check-access --max-delete 10 --checkers=16 --drive-pacer-min-sleep=11ms --resilient --no-cleanup -MvvP --drive-skip-gdocs --stats-file-name-length 100 --create-empty-src-dirs --resync --save-profile %s --profile-include-args`

	expected := `[profiletest]
type = profile
check_access = true
max_delete = 10
no_cleanup = true
progress = true
verbose = 2
stats_file_name_length = 100
checkers = 16
create_empty_src_dirs = true
drive_pacer_min_sleep = 11ms
drive_skip_gdocs = true
metadata = true
resilient = true
resync = true
arg_1 = {{arg1}}
arg_2 = {{arg2}}`

	Cmd = fmt.Sprintf(Cmd, path1, path2, section)

	setCmdArgs(Cmd)
	setup()

	ci := fs.GetConfig(ctx)
	assert.Equal(t, "", ci.SaveProfile)
	assert.False(t, config.LoadedData().HasSection(section))

	profiles.AddProfiles(ctx, Root)

	assert.Equal(t, section, ci.SaveProfile)
	assert.True(t, config.LoadedData().HasSection(section))

	if strings.Contains(Cmd, "--profile-include-args") {
		expected = strings.Replace(expected, "{{arg1}}", path1, 1)
		expected = strings.Replace(expected, "{{arg2}}", path2, 1)
	}

	assert.Equal(t, normalize(expected), normalize(profiles.SprintSection(section)))
	cleanup()

	// now use it
	Cmd = fmt.Sprintf(`rclone bisync somepath some:otherpath --use-profile %s`, section)
	setCmdArgs(Cmd)
	setup()

	assertBefore := func() {
		assert.Equal(t, fs.CommaSepList(nil), ci.UseProfile)
		assert.False(t, bisync.Opt.CheckAccess)
		assert.Equal(t, int64(-1), ci.MaxDelete)
		assert.Equal(t, 8, ci.Checkers)
		assert.Nil(t, Root.Flags().Lookup("drive-pacer-min-sleep"))
		assert.False(t, bisync.Opt.Resilient)
		assert.False(t, bisync.Opt.NoCleanup)
		assert.False(t, ci.Metadata)
		assert.Equal(t, fs.LogLevelNotice, ci.LogLevel)
		assert.False(t, ci.Progress)
		assert.Nil(t, Root.Flags().Lookup("drive-skip-gdocs"))
		assert.Equal(t, 45, ci.StatsFileNameLength)
		assert.False(t, bisync.Opt.CreateEmptySrcDirs)
		assert.False(t, bisync.Opt.Resync)
		assert.Equal(t, "", ci.SaveProfile)
		assert.False(t, ci.ProfileIncludeArgs)
	}

	assertAfter := func() {
		assert.Equal(t, fs.CommaSepList{section}, ci.UseProfile)
		assert.True(t, bisync.Opt.CheckAccess)
		assert.Equal(t, int64(10), ci.MaxDelete)
		assert.Equal(t, 16, ci.Checkers)
		assert.Equal(t, "11ms", Root.Flags().Lookup("drive-pacer-min-sleep").Value.String())
		assert.True(t, bisync.Opt.Resilient)
		assert.True(t, bisync.Opt.NoCleanup)
		assert.True(t, ci.Metadata)
		assert.Equal(t, fs.LogLevelDebug, ci.LogLevel)
		assert.True(t, ci.Progress)
		assert.Equal(t, "true", Root.Flags().Lookup("drive-skip-gdocs").Value.String())
		assert.Equal(t, 100, ci.StatsFileNameLength)
		assert.True(t, bisync.Opt.CreateEmptySrcDirs)
		assert.True(t, bisync.Opt.Resync)
		assert.Equal(t, "", ci.SaveProfile)
	}

	assertBefore()
	profiles.AddProfiles(ctx, Root)
	assertAfter()
	assert.False(t, ci.ProfileIncludeArgs)

	assert.Equal(t, []string{"rclone", "bisync", "somepath", "some:otherpath", "--use-profile", section}, os.Args)

	// now again with args
	cleanup()
	Cmd = fmt.Sprintf(`rclone bisync %s %s --use-profile %s --profile-include-args`, path1, path2, section)
	setCmdArgs(Cmd)
	setup()

	assertBefore()
	profiles.AddProfiles(ctx, Root)
	assertAfter()
	assert.True(t, ci.ProfileIncludeArgs)
	assert.Equal(t, []string{"rclone", "bisync", path1, path2}, os.Args)
	cleanup()
}

func TestParents(t *testing.T) {
	tempConfig(t)
	setup()
	defer cleanup()

	opt := profiles.ProfileOptFromCtx(ctx)
	opt.UseProfile = []string{"harry"}

	h := profiles.NewProfile("harry")
	h.Flags["checkers"] = "7"
	h.Parents = []string{"james", "lily"}
	require.NoError(t, h.Save(ctx, &opt))

	j := profiles.NewProfile("james")
	j.Flags["checkers"] = "8"
	j.Flags["checksum"] = "true"
	require.NoError(t, j.Save(ctx, &opt))

	l := profiles.NewProfile("lily")
	l.Flags["checkers"] = "9"
	l.Flags["metadata"] = "true"
	require.NoError(t, l.Save(ctx, &opt))

	parents, err := h.GetParents()
	assert.NoError(t, err)
	assert.Equal(t, []*profiles.Profile{j, l}, parents)

	ci := fs.GetConfig(ctx)
	assert.Equal(t, 8, ci.Checkers)
	assert.False(t, ci.CheckSum)
	assert.False(t, ci.Metadata)
	command, _, err := Root.Find([]string{"check", "someSrc", "someDst"})
	require.NoError(t, err)
	require.NoError(t, h.Use(ctx, command, nil, &opt))
	assert.Equal(t, 7, ci.Checkers)
	assert.True(t, ci.CheckSum)
	assert.True(t, ci.Metadata)

	hp, err := profiles.GetProfile("harry")
	require.NoError(t, err)
	assert.Equal(t, h, hp)
}

func TestRoundTrip(t *testing.T) {
	tempConfig(t)
	setup()
	defer cleanup()

	opt := profiles.ProfileOpt{
		SaveProfile: "potato",
		UseProfile:  []string{"potato"},
		StrictFlags: false,
		IncludeArgs: true,
	}

	p := &profiles.Profile{
		Name: "potato",
		Args: []string{"banana", "pickle"},
		Flags: map[string]string{
			"foo":   "bar",
			"a-b-c": "one 2 three",
			"john":  "smith",
		},
		Parents: []string{"salad", "chips"},
	}
	require.NoError(t, p.Save(ctx, &opt))

	readback, err := profiles.GetProfile("potato")
	require.NoError(t, err)
	assert.Equal(t, p, readback)
}

func TestSetProfileCommand(t *testing.T) {
	tempConfig(t)

	section := "setprofile_command_test"
	Cmd := `rclone setprofile %s %s --check-access --max-delete 10 --checkers=16 --drive-pacer-min-sleep=11ms --resilient --no-cleanup -MvvP --drive-skip-gdocs --stats-file-name-length 100 --create-empty-src-dirs --resync --save-profile %s --profile-include-args --flags-from bisync`
	Cmd = fmt.Sprintf(Cmd, "some/src", "some/dst", section)

	setCmdArgs(Cmd)
	setup()
	defer cleanup()

	expected := `[setprofile_command_test]
type = profile
check_access = true
max_delete = 10
no_cleanup = true
progress = true
verbose = 2
stats_file_name_length = 100
checkers = 16
create_empty_src_dirs = true
drive_pacer_min_sleep = 11ms
drive_skip_gdocs = true
metadata = true
resilient = true
resync = true
arg_1 = some/src
arg_2 = some/dst`

	assert.False(t, config.LoadedData().HasSection(section))

	// Capture the panic error/os.exit() using recover
	defer func() {
		if r := recover(); r != nil {
			// exit occurred (expected behavior)
			assert.True(t, bisync.Opt.Resilient)
			assert.Equal(t, normalize(expected), normalize(profiles.SprintSection(section)))

		}
	}()

	profiles.AddProfiles(context.Background(), Root)
	err := Root.Execute()
	require.NoError(t, err)

	// fail if no exit occurred
	t.Fail()
}

/* Helpers */

func setCmdArgs(cmd string) {
	os.Args = strings.Split(cmd, " ")
	log.Print(os.Args)
	fs.Debugf("args", "%v", os.Args)
}

func tempConfig(t *testing.T) (tempDir, configPath string) {
	// create temp config file
	tempDir = t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "rclone.conf")
	assert.NoError(t, err)
	configPath = tempFile.Name()
	assert.NoError(t, tempFile.Close())

	err = config.SetConfigPath(configPath)
	require.NoError(t, err, "error setting configpath %s", configPath)
	err = fs.ConfigFileSet("test", "testname", t.Name())
	require.NoError(t, err, "error writing to configfile %s", configPath)
	return tempDir, configPath
}

func setup() {
	setupRootCommand(Root)
	cmd.AddBackendFlags()
	ctx = context.Background()
}

func cleanup() {
	log.Print("cleaning up...")
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	ci := fs.GetConfig(ctx)
	*ci = *fs.NewConfig()
	Root.ResetFlags()
	resetFlagGroups()
	bisync.Opt = bisync.Options{}
}

func resetFlagGroups() {
	flags.All = flags.NewGroups()
	flags.All.NewGroup("Copy", "Flags for anything which can Copy a file.")
	flags.All.NewGroup("Sync", "Flags just used for `rclone sync`.")
	flags.All.NewGroup("Important", "Important flags useful for most commands.")
	flags.All.NewGroup("Check", "Flags used for `rclone check`.")
	flags.All.NewGroup("Networking", "General networking and HTTP stuff.")
	flags.All.NewGroup("Performance", "Flags helpful for increasing performance.")
	flags.All.NewGroup("Config", "General configuration of rclone.")
	flags.All.NewGroup("Debugging", "Flags for developers.")
	flags.All.NewGroup("Filter", "Flags for filtering directory listings.")
	flags.All.NewGroup("Listing", "Flags for listing directories.")
	flags.All.NewGroup("Logging", "Logging and statistics.")
	flags.All.NewGroup("Metadata", "Flags to control metadata.")
	flags.All.NewGroup("RC", "Flags to control the Remote Control API.")
}

func normalize(section string) []string {
	s := strings.Split(section, "\n")
	slices.Sort[[]string, string](s)
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
