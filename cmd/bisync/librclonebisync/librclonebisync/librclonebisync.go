// Package librclonebisync is a basic library for using Bisync through librclone ( https://pkg.go.dev/github.com/rclone/rclone/librclone/librclone )
// rc docs: https://rclone.org/rc/
package librclonebisync

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	_ "github.com/rclone/rclone/backend/all" // import all backends
	_ "github.com/rclone/rclone/cmd/bisync"  // import bisync/*
	"github.com/rclone/rclone/fs"
	_ "github.com/rclone/rclone/fs/operations" // import operations/* rc commands
	"github.com/rclone/rclone/librclone/librclone"
)

// BisyncRequest contains the main parameters for a Bisync command
type BisyncRequest struct {
	Path1                 string `json:"path1,omitempty"`                 // a remote directory string e.g. drive:path1
	Path2                 string `json:"path2,omitempty"`                 // a remote directory string e.g. drive:path1
	DryRun                bool   `json:"dryRun,omitempty"`                // dry-run mode
	Resync                bool   `json:"resync,omitempty"`                // performs the resync run
	CheckAccess           bool   `json:"checkAccess,omitempty"`           // abort if RCLONE_TEST files are not found on both filesystems
	CheckFilename         string `json:"checkFilename,omitempty"`         // file name for checkAccess (default: RCLONE_TEST)
	MaxDelete             int    `json:"maxDelete,omitempty"`             // abort sync if percentage of deleted files is above this threshold (default: 50)
	Force                 bool   `json:"force,omitempty"`                 // Bypass maxDelete safety check and run the sync
	CheckSync             string `json:"checkSync,omitempty"`             // true by default, false disables comparison of final listings, only will skip sync, only compare listings from the last run
	CreateEmptySrcDirs    bool   `json:"createEmptySrcDirs,omitempty"`    // Sync creation and deletion of empty directories. (Not compatible with --remove-empty-dirs)
	RemoveEmptyDirs       bool   `json:"removeEmptyDirs,omitempty"`       // remove empty directories at the final cleanup step
	FiltersFile           string `json:"filtersFile,omitempty"`           // read filtering patterns from a file
	IgnoreListingChecksum bool   `json:"ignoreListingChecksum,omitempty"` // Do not use checksums for listings
	Resilient             bool   `json:"resilient,omitempty"`             // Allow future runs to retry after certain less-serious errors, instead of requiring resync. Use at your own risk!
	Workdir               string `json:"workdir,omitempty"`               // server directory for history files (default: /home/ncw/.cache/rclone/bisync)
	NoCleanup             bool   `json:"noCleanup,omitempty"`             // retain working files
	// Config                ConfigOptions `json:"_config,omitempty"`               // _config options
}

// Main is the "main" options set (or "block") https://rclone.org/rc/#options-set
type Main struct {
	Main ConfigOptions `json:"main,omitempty"` // global config options
}

// ConfigOptions is a modified version of fs.ConfigInfo for easier json unmarshalling
type ConfigOptions struct {
	LogLevel               fs.LogLevel   // Log level DEBUG|INFO|NOTICE|ERROR (7, 6, 5, 3)
	StatsLogLevel          fs.LogLevel   // Log level to show --stats output DEBUG|INFO|NOTICE|ERROR (7, 6, 5, 3)
	UseJSONLog             bool          // Use json log format
	BackupDir              string        // Make backups into hierarchy based in DIR
	Suffix                 string        // Suffix to add to changed files
	SuffixKeepExtension    bool          // Preserve the extension when using --suffix
	CheckSum               bool          // Check for changes with size & checksum (if available, or fallback to size only).
	SizeOnly               bool          // Skip based on size only, not modtime or checksum
	IgnoreTimes            bool          // Don't skip files that match size and time - transfer all files
	IgnoreExisting         bool          // Skip all files that exist on destination
	IgnoreErrors           bool          // Delete even if there are I/O errors
	ModifyWindow           time.Duration // Max time diff to be considered the same
	Checkers               int           // Number of checkers to run in parallel
	Transfers              int           // Number of file transfers to run in parallel
	TrackRenames           bool          // Track file renames.
	TrackRenamesStrategy   string        // Comma separated list of strategies used to track renames
	IgnoreSize             bool          // Ignore size when skipping use modtime or checksum
	IgnoreChecksum         bool          // Skip post copy check of checksums
	IgnoreCaseSync         bool          // Ignore case when synchronizing
	NoUnicodeNormalization bool          // Don't normalize unicode characters in filenames
	MaxBacklog             int           // Maximum number of objects in sync or check backlog
	MaxStatsGroups         int           // Maximum number of stats groups to keep in memory, on max oldest is discarded
	StatsOneLine           bool          // Make the stats fit on one line
	StatsOneLineDate       bool          // Enable --stats-one-line and add current date/time prefix
	StatsOneLineDateFormat string        // Enable --stats-one-line-date and use custom formatted date
	RefreshTimes           bool          // Refresh the modtime of remote files
	Metadata               bool          // If set, preserve metadata when copying objects
}

// NewBisyncRequest returns a new BisyncRequest with default values
func NewBisyncRequest() BisyncRequest {
	return BisyncRequest{
		Path1:                 "",
		Path2:                 "",
		DryRun:                false,
		Resync:                false,
		CheckAccess:           false,
		CheckFilename:         "RCLONE_TEST",
		MaxDelete:             50,
		Force:                 false,
		CheckSync:             "true",
		CreateEmptySrcDirs:    false,
		RemoveEmptyDirs:       false,
		FiltersFile:           "",
		IgnoreListingChecksum: false,
		Resilient:             false,
		// Config:                GetDefaultOptions(),
	}
}

// BisyncRPC runs bisync via librclone
func BisyncRPC(br BisyncRequest) (output string, status int) {
	librclone.Initialize()
	defer librclone.Finalize()

	bisyncRequestJSON, err := json.MarshalIndent(br, "", "\t")
	if err != nil {
		fmt.Println(err)
	}

	return librclone.RPC("sync/bisync", string(bisyncRequestJSON))
}

// PrintBisyncRPC runs bisync via librclone and logs to stdout
func PrintBisyncRPC(br BisyncRequest) (output string, status int) {
	librclone.Initialize()
	defer librclone.Finalize()

	bisyncRequestJSON, err := json.MarshalIndent(br, "", "\t")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintln(os.Stdout, string(bisyncRequestJSON))

	output, status = librclone.RPC("sync/bisync", string(bisyncRequestJSON))
	fmt.Fprintln(os.Stdout, output)
	fmt.Fprintln(os.Stdout, "status: ", status)

	return output, status
}

// WriterBisyncRPC runs bisync via librclone and writes to w
func WriterBisyncRPC(br BisyncRequest, w io.Writer) (output string, status int) {
	librclone.Initialize()
	defer librclone.Finalize()

	bisyncRequestJSON, err := json.MarshalIndent(br, "", "\t")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintln(w, string(bisyncRequestJSON))

	output, status = librclone.RPC("sync/bisync", string(bisyncRequestJSON))
	fmt.Fprintln(w, output)
	fmt.Fprintln(w, "status: ", status)

	return output, status
}

// CheckOptions returns the currently set options (read-only)
// https://rclone.org/rc/#options-get
func CheckOptions() (output string, status int) {
	librclone.Initialize()
	defer librclone.Finalize()

	output, status = librclone.RPC("options/get", "")
	fmt.Fprintln(os.Stdout, output)
	fmt.Fprintln(os.Stdout, "status: ", status)

	return output, status
}

// GetDefaultOptions returns a new ConfigOptions struct with default values
// for passing back to SetOptions
func GetDefaultOptions() ConfigOptions {
	return ConfigOptions{
		LogLevel:       fs.LogLevelNotice,
		StatsLogLevel:  fs.LogLevelInfo,
		ModifyWindow:   time.Nanosecond,
		Checkers:       8,
		Transfers:      4,
		MaxStatsGroups: 1000,
		MaxBacklog:     10000,
		BackupDir:      "",
		// TODO: add more
	}
}

// SetOptions sets global config options https://rclone.org/rc/#options-set
func SetOptions(ci ConfigOptions) (output string, status int) {
	librclone.Initialize()
	defer librclone.Finalize()

	main := Main{
		Main: ci,
	}

	ciRequestJSON, err := json.MarshalIndent(main, "", "\t")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintln(os.Stdout, string(ciRequestJSON))

	output, status = librclone.RPC("options/set", string(ciRequestJSON))
	fmt.Fprintln(os.Stdout, output)
	fmt.Fprintln(os.Stdout, "status: ", status)

	return output, status
}
