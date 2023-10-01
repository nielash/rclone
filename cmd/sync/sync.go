// Package sync provides the sync command.
package sync

import (
	"context"
	"io"
	"os"

	mutex "sync" // renamed as "sync" already in use

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/hash"
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
	destAfter          = ""
	opt                = operations.LoggerOpt{
		HashType: hash.MD5,
	}
)

func init() {
	cmd.Root.AddCommand(commandDefinition)
	cmdFlags := commandDefinition.Flags()
	flags.BoolVarP(cmdFlags, &createEmptySrcDirs, "create-empty-src-dirs", "", createEmptySrcDirs, "Create empty source dirs on destination after sync", "")
	operations.AddLoggerFlags(cmdFlags, &opt, &combined, &missingOnSrc, &missingOnDst, &match, &differ, &errFile, &destAfter)
}

var lock mutex.Mutex

func syncLoggerFn(ctx context.Context, sigil operations.Sigil, src fs.ObjectInfo, dst fs.ObjectInfo, err error) {
	lock.Lock()
	defer lock.Unlock()

	if err == fs.ErrorIsDir && !opt.FilesOnly && opt.DestAfter != nil {
		opt.PrintDestAfter(ctx, sigil, src, dst, err)
		return
	}

	_, srcOk := src.(fs.Object)
	_, dstOk := dst.(fs.Object)
	var filename string
	if !srcOk && !dstOk {
		return
	} else if srcOk && !dstOk {
		filename = src.String()
	} else {
		filename = dst.String()
	}

	if sigil.Writer(opt) != nil {
		operations.SyncFprintf(sigil.Writer(opt), "%s\n", filename)
	}
	if opt.Combined != nil {
		operations.SyncFprintf(opt.Combined, "%c %s\n", sigil, filename)
		fs.Debugf(nil, "Sync Logger: %s: %c %s\n", sigil.String(), sigil, filename)
	}
	if opt.DestAfter != nil {
		opt.PrintDestAfter(ctx, sigil, src, dst, err)
	}
}

// GetSyncLoggerOpt gets the options corresponding to the logger flags
func GetSyncLoggerOpt(ctx context.Context, fdst fs.Fs, command *cobra.Command) (operations.LoggerOpt, func(), error) {
	closers := []io.Closer{}

	opt.LoggerFn = syncLoggerFn
	opt.SetListFormat(ctx, command.Flags())
	opt.NewListJSON(ctx, fdst, "")

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
	if err := open(destAfter, &opt.DestAfter); err != nil {
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
			ctx := context.Background()
			opt, close, err := GetSyncLoggerOpt(ctx, fdst, command)
			if err != nil {
				return err
			}
			defer close()

			if anyNotBlank(combined, missingOnSrc, missingOnDst, match, differ, errFile, destAfter) {
				ctx = operations.WithSyncLogger(ctx, opt)
			}

			if srcFileName == "" {
				return sync.Sync(ctx, fdst, fsrc, createEmptySrcDirs)
			}
			return operations.CopyFile(ctx, fdst, fsrc, srcFileName, srcFileName)
		})
	},
}
