// Package bilib provides common stuff for bisync and bisync_test
package bilib

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/fs/operations"
)

// FsPath converts Fs to a suitable rclone argument
func FsPath(f fs.Info) string {
	name, path, slash := f.Name(), f.Root(), "/"
	if name == "local" {
		slash = string(os.PathSeparator)
		if runtime.GOOS == "windows" {
			path = strings.ReplaceAll(path, "/", slash)
			path = strings.TrimPrefix(path, `\\?\`)
		}
	} else {
		path = name + ":" + path
	}
	if !strings.HasSuffix(path, slash) {
		path += slash
	}
	return path
}

// CanonicalPath converts a remote to a suitable base file name
func CanonicalPath(remote string) string {
	trimmed := strings.Trim(remote, `\/`)
	return nonCanonicalChars.ReplaceAllString(trimmed, "_")
}

var nonCanonicalChars = regexp.MustCompile(`[\s\\/:?*]`)

// SessionName makes a unique base name for the sync operation
func SessionName(fs1, fs2 fs.Fs, overrideName string, limit int) string {
	s := ""
	if overrideName != "" {
		s = CanonicalPath(overrideName)
	} else {
		s = StripHexString(CanonicalPath(FsPath(fs1))) + ".." + StripHexString(CanonicalPath(FsPath(fs2)))
	}

	if limit > -1 {
		trueLimit := limit - 18 // reserve space for ".path1.lst-dry-new" etc.
		if len(s) < trueLimit {
			return s
		}
		if overrideName != "" {
			return StringToHash(s)
		}
		// try hashing all but last path segment
		hashedS := HashDir(fs1) + ".." + HashDir(fs2)
		if len(hashedS) < trueLimit {
			fs.Debugf(s, "converted to: %v", hashedS)
			return hashedS
		}
		// still too long, so just hash the whole thing
		fs.Debugf(s, "too long; converted to: %v", StringToHash(s))
		return StringToHash(s)
	}
	return s
}

// HashDir is like StripHexString(CanonicalPath(FsPath(f))) except
// all but the last path segment will be replaced with an MD5 hash.
func HashDir(f fs.Fs) string {
	s := FsPath(f)
	s = filepath.Join(StringToHash(filepath.Dir(s)), filepath.Base(s))
	return StripHexString(CanonicalPath(s))
}

// StripHexString strips the (first) canonical {hexstring} suffix
func StripHexString(path string) string {
	open := strings.IndexRune(path, '{')
	close := strings.IndexRune(path, '}')
	if open >= 0 && close > open {
		return path[:open] + path[close+1:] // (trailing underscore)
	}
	return path
}

// HasHexString returns true if path contains at least one canonical {hexstring} suffix
func HasHexString(path string) bool {
	open := strings.IndexRune(path, '{')
	if open >= 0 && strings.IndexRune(path, '}') > open {
		return true
	}
	return false
}

// BasePath joins the workDir with the SessionName, stripping {hexstring} suffix if necessary
func BasePath(ctx context.Context, workDir string, fs1, fs2 fs.Fs, overrideName string, limit int) string {
	suffixedSession := CanonicalPath(FsPath(fs1)) + ".." + CanonicalPath(FsPath(fs2))
	suffixedBasePath := filepath.Join(workDir, suffixedSession)
	listing1 := suffixedBasePath + ".path1.lst"
	listing2 := suffixedBasePath + ".path2.lst"

	sessionName := SessionName(fs1, fs2, overrideName, limit)
	basePath := filepath.Join(workDir, sessionName)

	// Normalize to non-canonical version for overridden configs
	// to ensure that backend-specific flags don't change the listing filename.
	// For backward-compatibility, we first check if we found a listing file with the suffixed version.
	// If so, we rename it (and overwrite non-suffixed version, if any.)
	// If not, we carry on with the non-suffixed version.
	// We should only find a suffixed version if bisync v1.66 or older created it.
	if HasHexString(suffixedSession) && FileExists(listing1) {
		fs.Infof(listing1, "renaming to: %s", basePath+".path1.lst")
		if !operations.SkipDestructive(ctx, listing1, "rename to "+basePath+".path1.lst") {
			_ = os.Rename(listing1, basePath+".path1.lst")
		}
	}
	if HasHexString(suffixedSession) && FileExists(listing2) {
		fs.Infof(listing2, "renaming to: %s", basePath+".path2.lst")
		if !operations.SkipDestructive(ctx, listing1, "rename to "+basePath+".path2.lst") {
			_ = os.Rename(listing2, basePath+".path2.lst")
		} else {
			return suffixedBasePath
		}
	}
	return basePath
}

// StringToHash returns an MD5 hash of s
func StringToHash(s string) string {
	ht := hash.MD5
	hasher, err := hash.NewMultiHasherTypes(hash.NewHashSet(ht))
	if err != nil {
		fs.Errorf(s, "hash unsupported: %v", err)
	}

	_, err = hasher.Write([]byte(s))
	if err != nil {
		fs.Errorf(s, "failed to write to hasher: %v", err)
	}

	sum, err := hasher.SumString(ht, false)
	if err != nil {
		fs.Errorf(s, "hasher returned an error: %v", err)
	}
	return sum
}
