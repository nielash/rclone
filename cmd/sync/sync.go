// Package sync provides the sync command.
package sync

import (
	"context"
	"io"
	"os"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/fs/sync"
	"github.com/spf13/cobra"
)

var (
	createEmptySrcDirs = false
	combined           = ""
	missingOnSrc       = ""
	missingOnDst       = ""
	match              = ""
	differ             = ""
	errFile            = ""
	opt                = operations.LoggerOpt{}
)

func init() {
	cmd.Root.AddCommand(commandDefinition)
	cmdFlags := commandDefinition.Flags()
	flags.BoolVarP(cmdFlags, &createEmptySrcDirs, "create-empty-src-dirs", "", createEmptySrcDirs, "Create empty source dirs on destination after sync", "")
	flags.StringVarP(cmdFlags, &combined, "combined", "", combined, "Make a combined report of changes to this file", "")
	flags.StringVarP(cmdFlags, &missingOnSrc, "missing-on-src", "", missingOnSrc, "Report all files missing from the source to this file", "")
	flags.StringVarP(cmdFlags, &missingOnDst, "missing-on-dst", "", missingOnDst, "Report all files missing from the destination to this file", "")
	flags.StringVarP(cmdFlags, &match, "match", "", match, "Report all matching files to this file", "")
	flags.StringVarP(cmdFlags, &differ, "differ", "", differ, "Report all non-matching files to this file", "")
	flags.StringVarP(cmdFlags, &errFile, "error", "", errFile, "Report all files with errors (hashing or reading) to this file", "")
}

func SyncLoggerFn(ctx context.Context, sigil rune, src fs.ObjectInfo, dst fs.ObjectInfo, err error) {
	_, srcOk := src.(fs.Object)
	_, dstOk := dst.(fs.Object)
	var filename string
	if !srcOk && !dstOk {
		return
	} else if srcOk && !dstOk {
		filename = src.Remote()
	} else {
		filename = dst.String()
	}

	if operations.SigilToOpt(sigil, opt) != nil {
		operations.SyncFprintfWrapper(operations.SigilToOpt(sigil, opt), "%s\n", filename)
	}
	if opt.Combined != nil && sigil != operations.Completed {
		operations.SyncFprintfWrapper(opt.Combined, "%c %s\n", sigil, filename)
		fs.Debugf(nil, "Sync Logger: %s: %c %s\n", operations.TranslateSigil(sigil), sigil, filename)
	}
}

// GetSyncLoggerOpt gets the options corresponding to the logger flags
func GetSyncLoggerOpt() (operations.LoggerOpt, func(), error) {
	closers := []io.Closer{}

	opt.LoggerFn = SyncLoggerFn

	open := func(name string, pout *io.Writer) error {
		if name == "" {
			return nil
		}
		if name == "-" {
			*pout = os.Stdout
			return nil
		}
		out, err := os.Create(name)
		if err != nil {
			return err
		}
		*pout = out
		closers = append(closers, out)
		return nil
	}

	if err := open(combined, &opt.Combined); err != nil {
		return opt, nil, err
	}
	if err := open(missingOnSrc, &opt.MissingOnSrc); err != nil {
		return opt, nil, err
	}
	if err := open(missingOnDst, &opt.MissingOnDst); err != nil {
		return opt, nil, err
	}
	if err := open(match, &opt.Match); err != nil {
		return opt, nil, err
	}
	if err := open(differ, &opt.Differ); err != nil {
		return opt, nil, err
	}
	if err := open(errFile, &opt.Error); err != nil {
		return opt, nil, err
	}

	close := func() {
		for _, closer := range closers {
			err := closer.Close()
			if err != nil {
				fs.Errorf(nil, "Failed to close report output: %v", err)
			}
		}
	}

	return opt, close, nil
}

func anyNotBlank(s ...string) bool {
	for _, x := range s {
		if x != "" {
			return true
		}
	}
	return false
}

var commandDefinition = &cobra.Command{
	Use:   "sync source:path dest:path",
	Short: `Make source and dest identical, modifying destination only.`,
	Long: `
Sync the source to the destination, changing the destination
only.  Doesn't transfer files that are identical on source and
destination, testing by size and modification time or MD5SUM.
Destination is updated to match source, including deleting files
if necessary (except duplicate objects, see below). If you don't
want to delete files from destination, use the
[copy](/commands/rclone_copy/) command instead.

**Important**: Since this can cause data loss, test first with the
` + "`--dry-run` or the `--interactive`/`-i`" + ` flag.

    rclone sync --interactive SOURCE remote:DESTINATION

Note that files in the destination won't be deleted if there were any
errors at any point.  Duplicate objects (files with the same name, on
those providers that support it) are also not yet handled.

It is always the contents of the directory that is synced, not the
directory itself. So when source:path is a directory, it's the contents of
source:path that are copied, not the directory name and contents.  See
extended explanation in the [copy](/commands/rclone_copy/) command if unsure.

If dest:path doesn't exist, it is created and the source:path contents
go there.

It is not possible to sync overlapping remotes. However, you may exclude
the destination from the sync with a filter rule or by putting an 
exclude-if-present file inside the destination directory and sync to a
destination that is inside the source directory.

**Note**: Use the ` + "`-P`" + `/` + "`--progress`" + ` flag to view real-time transfer statistics

**Note**: Use the ` + "`rclone dedupe`" + ` command to deal with "Duplicate object/directory found in source/destination - ignoring" errors.
See [this forum post](https://forum.rclone.org/t/sync-not-clearing-duplicates/14372) for more info.
`,
	Annotations: map[string]string{
		"groups": "Sync,Copy,Filter,Listing,Important",
	},
	Run: func(command *cobra.Command, args []string) {
		cmd.CheckArgs(2, 2, command, args)
		fsrc, srcFileName, fdst := cmd.NewFsSrcFileDst(args)
		cmd.Run(true, true, command, func() error {
			opt, close, err := GetSyncLoggerOpt()
			if err != nil {
				return err
			}
			defer close()

			ctx := context.Background()

			if anyNotBlank(combined, missingOnSrc, missingOnDst, match, differ, errFile) {
				ctx = operations.NewSyncLogger(ctx, opt)
			}

			if srcFileName == "" {
				return sync.Sync(ctx, fdst, fsrc, createEmptySrcDirs)
			}
			return operations.CopyFile(ctx, fdst, fsrc, srcFileName, srcFileName)
		})
	},
}
