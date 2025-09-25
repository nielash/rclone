package bilib

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/rclone/rclone/fs/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/rclone/rclone/backend/all"
)

const (
	longPath1 = "/var/folders/q0/wmf37v850txck86cpnvwm_zw0000gn/T/TestBisyncRemoteRemote713233314/001/095146hu/ext_paths/path1/subdir with spaces/"
	longPath2 = "/var/folders/q0/wmf37v850txck86cpnvwm_zw0000gn/T/TestBisyncRemoteRemote713233314/001/095146hu/ext_paths/path2/subdir with spaces/"
)

func TestSessionName(t *testing.T) {
	for _, test := range []struct {
		path1        string
		path2        string
		overrideName string
		limit        int
		want         string
	}{
		{longPath1, longPath2, "", -1, "var_folders_q0_wmf37v850txck86cpnvwm_zw0000gn_T_TestBisyncRemoteRemote713233314_001_095146hu_ext_paths_path1_subdir_with_spaces..var_folders_q0_wmf37v850txck86cpnvwm_zw0000gn_T_TestBisyncRemoteRemote713233314_001_095146hu_ext_paths_path2_subdir_with_spaces"},
		{longPath1, longPath2, "banana", -1, "banana"},
		{longPath1, longPath2, "banana", 100, "banana"},
		{longPath1, longPath2, "", 255, "f0483e2c37a8e04395becd5bd44db1e1_subdir_with_spaces..8dd0244c7c48ace190d526c2f51e0a14_subdir_with_spaces"},
		{longPath1, longPath2, "", 100, "a9aa9a88083860f6e0a215652b05dd4e"},
		{":memory:", ":memory:", "", 100, "_memory_.._memory_"},
		{":memory:path1", ":memory:path2", "", 100, "_memory_path1.._memory_path2"},
		{":memory:path1/sub", ":memory:path2/sub", "", 100, "_memory_path1_sub.._memory_path2_sub"},
		{":memory:/path1/sub", ":memory:/path2/sub", "", 100, "_memory_path1_sub.._memory_path2_sub"},
		{":memory:path1" + longPath1, ":memory:path2" + longPath2, "", -1, "_memory_path1_var_folders_q0_wmf37v850txck86cpnvwm_zw0000gn_T_TestBisyncRemoteRemote713233314_001_095146hu_ext_paths_path1_subdir_with_spaces.._memory_path2_var_folders_q0_wmf37v850txck86cpnvwm_zw0000gn_T_TestBisyncRemoteRemote713233314_001_095146hu_ext_paths_path2_subdir_with_spaces"},
		{":memory:path1" + longPath1, ":memory:path2" + longPath2, "", 255, "3d396dfa9f2e39c0ea0627b9947af1f8_subdir_with_spaces..ca9a912944fb09dc83e86a72ae2d5134_subdir_with_spaces"},
		{":memory:path1" + longPath1, ":memory:path2" + longPath2, "", 100, "8fac7ed147f241be3859eea051c51c94"},
		{longPath1, longPath2, "bananabananabananabananabananabananabananabananabananabanana", 50, "a52abf9d0d7bc5b0b7676ed63431b833"},
		{"/illegal: *chars?", "/illegal\\ chars\\", "", -1, "illegal___chars_..illegal__chars"},
	} {
		fs1, err := cache.Get(context.Background(), test.path1)
		require.NoError(t, err)
		fs2, err := cache.Get(context.Background(), test.path2)
		require.NoError(t, err)

		if runtime.GOOS == "windows" {
			if test.limit > -1 {
				continue // skip as hash would be different due to different slash
			}
			if fs1.Features().IsLocal && test.overrideName == "" {
				prefix := fs1.Root()[4:5] + "__" // drive letter
				test.want = prefix + test.want
			}
			if fs2.Features().IsLocal && test.overrideName == "" {
				prefix := fs2.Root()[4:5] + "__" // drive letter
				test.want = strings.Replace(test.want, "..", ".."+prefix, 1)
			}
		}

		got := SessionName(fs1, fs2, test.overrideName, test.limit)
		assert.Equal(t, test.want, got)
		assert.True(t, lengthOk(got, test.limit))
	}
}

func lengthOk(s string, limit int) bool {
	if limit == -1 {
		return true
	}
	return len(s) <= limit-18
}

func TestStringToHash(t *testing.T) {
	for _, test := range []struct {
		s    string
		want string
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"a", "0cc175b9c0f1b6a831c399e269772661"},
		{"abc", "900150983cd24fb0d6963f7d28e17f72"},
		{"message digest", "f96b697d7cb7938d525a2f31aaf161d0"},
		{"abcdefghijklmnopqrstuvwxyz", "c3fcd3d76192e4007dfb496cca67e13b"},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", "d174ab98d277d9f5a5611c2c9f419d9f"},
		{"12345678901234567890123456789012345678901234567890123456789012345678901234567890", "57edf4a22be3c955ac49da2e2107b67a"},
	} {

		got := StringToHash(test.s)
		assert.Equal(t, test.want, got)
	}
}
