// Package syncdirtimes provides the syncdirtimes command.
package syncdirtimes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/accounting"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/filter"
	"github.com/rclone/rclone/fs/march"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/lib/terminal"
	"github.com/spf13/cobra"
)

var (
	preferOlder     = false
	preferNewer     = false
	destMinAge      = fs.DurationOff
	destMaxAge      = fs.DurationOff
	destModTimeTo   time.Time
	destModTimeFrom time.Time
	err, firstErr   error
	marchErrLock    sync.Mutex
	f1, f2          fs.Fs
	ci              *fs.ConfigInfo
	fi              *filter.Filter
	touchDir        func(ctx context.Context, t time.Time, d fs.Directory) error
)

type dirTimesSync struct{}

func init() {
	cmd.Root.AddCommand(commandDefinition)
	cmdFlags := commandDefinition.Flags()
	flags.BoolVarP(cmdFlags, &preferNewer, "prefer-newer", "", preferNewer, "The newer time should win", "")
	flags.BoolVarP(cmdFlags, &preferOlder, "prefer-older", "", preferOlder, "The older time should win", "")
	flags.FVarP(cmdFlags, &destMinAge, "min-age", "", "Skip if DEST is younger than this", "")
	flags.FVarP(cmdFlags, &destMaxAge, "max-age", "", "Skip if DEST is older than this", "")
}

// rclone syncdirtimes src:path dst:path --prefer-older -vP --dry-run --modify-window 1s

var commandDefinition = &cobra.Command{
	Use:   "syncdirtimes src:path dst:path",
	Short: `Sync directory modtimes.`,
	Annotations: map[string]string{
		"groups": "Important",
	},
	Run: func(command *cobra.Command, args []string) {
		cmd.CheckArgs(2, 2, command, args)
		fsrc, srcFileName, fdst := cmd.NewFsSrcFileDst(args)
		cmd.Run(true, true, command, func() error {
			if srcFileName == "" {
				return SyncDirTimes(context.Background(), fdst, fsrc)
			}
			return fs.ErrorDirNotFound
		})
	},
}

// SyncDirTimes syncs directory modtimes (only) from src to dst
func SyncDirTimes(ctx context.Context, fdst, fsrc fs.Fs) error {
	ci = fs.GetConfig(ctx)
	f1, f2 = fsrc, fdst
	dts := dirTimesSync{}
	fi = filter.GetConfig(ctx)
	touchDir = f2.Features().TouchDir
	if touchDir == nil {
		return fmt.Errorf(Color(terminal.RedFg, "%v: dest remote does not support setting dir modtime"), f2)
	}

	// Filter flags
	if destMinAge.IsSet() {
		destModTimeTo = time.Now().Add(-time.Duration(destMinAge))
		fs.Debugf(nil, "--min-age %v to %v", destMinAge, destModTimeTo)
	}
	if destMaxAge.IsSet() {
		destModTimeFrom = time.Now().Add(-time.Duration(destMaxAge))
		if !destModTimeTo.IsZero() && destModTimeTo.Before(destModTimeFrom) {
			log.Fatal("filter: --min-age can't be larger than --max-age")
		}
		fs.Debugf(nil, "--max-age %v to %v", destMaxAge, destModTimeFrom)
	}

	if (preferNewer || ci.UpdateOlder) && preferOlder {
		return errors.New("can't use --prefer-newer AND --prefer-older. There would be nothing to update")
	}

	// set up a march over fdst and fsrc
	m := &march.March{
		Ctx:                    ctx,
		Fdst:                   fdst,
		Fsrc:                   fsrc,
		Dir:                    "",
		NoTraverse:             false,
		Callback:               &dts,
		DstIncludeAll:          false,
		NoCheckDest:            false,
		NoUnicodeNormalization: ci.NoUnicodeNormalization,
	}
	fs.Debugf(nil, "starting to march!")
	err = m.Run(ctx)

	fs.Debugf(nil, "march completed. err: %v", err)
	if err == nil {
		err = firstErr
	}
	// Print nothing to transfer message if there were no transfers and no errors
	if accounting.Stats(ctx).GetTransfers() == 0 && err == nil {
		fs.Infof(nil, Color(terminal.GreenFg, "There was nothing to transfer"))
	}

	return err
}

// SrcOnly have an object which is on src only
func (dts *dirTimesSync) SrcOnly(o fs.DirEntry) (recurse bool) {
	if isDir(o) {
		fs.Debugf(o, "dir found on source only - skipping")
	}
	return isDir(o)
}

// DstOnly have an object which is on dst only
func (dts *dirTimesSync) DstOnly(o fs.DirEntry) (recurse bool) {
	if isDir(o) {
		fs.Debugf(o, "dir found on dest only - skipping")
	}
	return isDir(o)
}

// Match is called when object exists on both src and dst (whether equal or not)
func (dts *dirTimesSync) Match(ctx context.Context, o2, o1 fs.DirEntry) (recurse bool) {
	if !isDir(o1) {
		return false
	}

	d := o2.(fs.Directory)
	ch := accounting.Stats(ctx).NewCheckingTransfer(o1, "checking times")
	defer func() {
		ch.Done(ctx, nil)
	}()
	marchErrLock.Lock()
	defer marchErrLock.Unlock()
	if timeDiffers(ctx, o1.ModTime(ctx), o2.ModTime(ctx), f1, f2, o1.Remote()) {
		// filters
		if (ci.UpdateOlder || preferNewer) && o2.ModTime(ctx).After(o1.ModTime(ctx)) {
			fs.Debugf(o1, "Destination is newer than source, skipping")
			return true
		} else if preferOlder && o2.ModTime(ctx).Before(o1.ModTime(ctx)) {
			fs.Debugf(o1, "Destination is older than source, skipping")
			return true
		}
		// source
		if !fi.ModTimeFrom.IsZero() && o1.ModTime(ctx).Before(fi.ModTimeFrom) {
			fs.Debugf(o1, "Source %v is older than %v, skipping", o1.ModTime(ctx), fi.ModTimeFrom)
			return true
		}
		if !fi.ModTimeTo.IsZero() && o1.ModTime(ctx).After(fi.ModTimeTo) {
			fs.Debugf(o1, "Source %v is younger than %v, skipping", o1.ModTime(ctx), fi.ModTimeTo)
			return true
		}
		// dest
		if !destModTimeFrom.IsZero() && o2.ModTime(ctx).Before(destModTimeFrom) {
			fs.Debugf(o2, "Dest %v is older than %v, skipping", o2.ModTime(ctx), destModTimeFrom)
			return true
		}
		if !destModTimeTo.IsZero() && o2.ModTime(ctx).After(destModTimeTo) {
			fs.Debugf(o2, "Dest %v is younger than %v, skipping", o2.ModTime(ctx), destModTimeTo)
			return true
		}

		tr := accounting.Stats(ctx).NewTransfer(o1)
		defer func() {
			tr.Done(ctx, nil)
		}()
		msg := fmt.Sprintf(Color(terminal.GreenFg, "set modtime to %s (was: %s)"), Color(terminal.MagentaFg, o1.ModTime(ctx).String()), Color(terminal.MagentaFg, o2.ModTime(ctx).String()))
		if !operations.SkipDestructive(ctx, d, msg) {
			err = touchDir(ctx, o1.ModTime(ctx), d)
			if err != nil {
				fs.Errorf(d, Color(terminal.RedFg, "error setting modtime: %v"), err)
				if firstErr == nil {
					firstErr = err
				}
			} else {
				fs.Infof(d, msg)
			}
		}
	}
	return true
}

func isDir(e fs.DirEntry) bool {
	_, ok := e.(fs.Directory)
	return ok
}

// returns true if the times are definitely different (by more than the modify window).
// returns false if equal, within modify window, or if either is unknown.
// considers precision per-Fs.
func timeDiffers(ctx context.Context, a, b time.Time, fsA, fsB fs.Info, name string) bool {
	modifyWindow := fs.GetModifyWindow(ctx, fsA, fsB)
	if modifyWindow == fs.ModTimeNotSupported {
		return false
	}
	if a.IsZero() || b.IsZero() {
		fs.Logf(name, "Fs supports modtime, but modtime is missing")
		return false
	}
	dt := b.Sub(a)
	if dt < modifyWindow && dt > -modifyWindow {
		fs.Debugf(name, "modification time the same (differ by %s, within tolerance %s)", dt, modifyWindow)
		return false
	}

	fs.Debugf(name, "Modification times differ by %s: %v, %v", dt, a, b)
	return true
}

// Color handles terminal colors
func Color(style string, s string) string {
	terminal.Start()
	return style + s + terminal.Reset
}
