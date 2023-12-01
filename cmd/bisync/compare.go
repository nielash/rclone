package bisync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/lib/terminal"
)

// CompareOpt describes the Compare options in force
type CompareOpt = struct {
	Modtime    bool
	Size       bool
	Checksum   bool
	HashType1  hash.Type
	HashType2  hash.Type
	NoSlowHash bool
}

func (b *bisyncRun) setCompareDefaults(ctx context.Context) error {
	ci := fs.GetConfig(ctx)

	// defaults
	b.opt.Compare.Size = true
	b.opt.Compare.Modtime = true
	b.opt.Compare.Checksum = false

	if ci.SizeOnly {
		b.opt.Compare.Size = true
		b.opt.Compare.Modtime = false
		b.opt.Compare.Checksum = false
	} else if ci.CheckSum && !b.opt.IgnoreListingChecksum {
		b.opt.Compare.Size = true
		b.opt.Compare.Modtime = false
		b.opt.Compare.Checksum = true
	}

	if ci.IgnoreSize {
		b.opt.Compare.Size = false
	}

	err = b.setFromCompareFlag()
	if err != nil {
		return err
	}

	if b.opt.Compare.Checksum && !b.opt.IgnoreListingChecksum {
		b.setHashType()
	}

	// Checks and Warnings
	if b.opt.Compare.Modtime && (b.fs1.Precision() == fs.ModTimeNotSupported || b.fs2.Precision() == fs.ModTimeNotSupported) {
		fs.Logf(nil, Color(terminal.YellowFg, "WARNING: Modtime compare was requested but at least one remote does not support it. It is recommended to use --checksum or --size-only instead."))
	}
	if b.opt.Compare.Checksum && (b.opt.Compare.HashType1 == hash.None || b.opt.Compare.HashType2 == hash.None) {
		fs.Logf(nil, Color(terminal.YellowFg, "WARNING: Checksum compare was requested but at least one remote does not support checksums. Path1 (%s): %s, Path2 (%s): %s"), b.fs1.String(), b.opt.Compare.HashType1.String(), b.fs2.String(), b.opt.Compare.HashType2.String())
	}
	if b.opt.Compare.Checksum && !ci.CheckSum {
		fs.Logf(nil, Color(terminal.YellowFg, "WARNING: Checksums will be compared for deltas but not during sync as --checksum is not set."))
	}
	if (ci.CheckSum || b.opt.Compare.Checksum) && b.opt.IgnoreListingChecksum {
		fs.Logf(nil, Color(terminal.YellowFg, "WARNING: Ignoring checksum for deltas as --ignore-listing-checksum is set"))
		// note: --checksum will still affect the internal sync calls
	}
	if b.opt.Compare.Modtime && ci.CheckSum {
		fs.Logf(nil, Color(terminal.YellowFg, "WARNING: Modtimes will be compared for deltas but not during sync as --checksum is set."))
	}
	if !b.opt.Compare.Size && !b.opt.Compare.Modtime && !b.opt.Compare.Checksum {
		return errors.New(Color(terminal.RedFg, "must set a Compare method. (size, modtime, and checksum can't all be false.)"))
	}
	prettyprint(b.opt.Compare, "Bisyncing with Comparison Settings", fs.LogLevelDebug)
	return nil
}

// returns true if the sizes are definitely different.
// returns false if equal, or if either is unknown.
func sizeDiffers(a, b int64) bool {
	if a < 0 || b < 0 {
		return false
	}
	return a != b
}

// returns true if the hashes are definitely different.
// returns false if equal, or if either is unknown.
func hashDiffers(a, b string, ht1, ht2 hash.Type, size1, size2 int64) bool {
	if a == "" || b == "" {
		if ht1 != hash.None && ht2 != hash.None && !(size1 <= 0 || size2 <= 0) {
			fs.Logf(nil, Color(terminal.YellowFg, "WARNING: hash unexpectedly blank despite Fs support (%s, %s) (you may need to --resync!)"), a, b)
		}
		return false
	}
	if ht1 != ht2 {
		fs.Infof(nil, Color(terminal.YellowFg, "WARNING: Can't compare hashes of different types (%s, %s)"), ht1.String(), ht2.String())
		return false
	}
	return a != b
}

// chooses hash type, giving priority to types both sides have in common
func (b *bisyncRun) setHashType() {
	if b.opt.Compare.NoSlowHash && (b.fs1.Features().SlowHash || b.fs2.Features().SlowHash) {
		fs.Infof(nil, "Not checking for common hash as at least one slow hash detected.")
	} else {
		common := b.fs1.Hashes().Overlap(b.fs2.Hashes())
		if common.Count() > 0 && common.GetOne() != hash.None {
			ht := common.GetOne()
			b.opt.Compare.HashType1 = ht
			b.opt.Compare.HashType2 = ht
			return
		}
	}
	fs.Logf(b.fs2, "--checksum is in use but Path1 and Path2 have no hashes in common; falling back to modtime,size for sync. (Use --size-only to ignore modtime)")
	b.opt.Compare.Modtime = true
	b.opt.Compare.Size = true
	if b.opt.Compare.NoSlowHash && b.fs1.Features().SlowHash {
		fs.Infof(nil, Color(terminal.YellowFg, "Slow hash detected on Path1. Will ignore checksum due to --no-slow-hash"))
		b.opt.Compare.HashType1 = hash.None
	} else {
		b.opt.Compare.HashType1 = b.fs1.Hashes().GetOne()
		if b.opt.Compare.HashType1 != hash.None {
			fs.Logf(b.fs1, "will use %s for same-side diffs on Path1 only", b.opt.Compare.HashType1)
		}
	}
	if b.opt.Compare.NoSlowHash && b.fs2.Features().SlowHash {
		fs.Infof(nil, Color(terminal.YellowFg, "Slow hash detected on Path2. Will ignore checksum due to --no-slow-hash"))
		b.opt.Compare.HashType1 = hash.None
	} else {
		b.opt.Compare.HashType2 = b.fs2.Hashes().GetOne()
		if b.opt.Compare.HashType2 != hash.None {
			fs.Logf(b.fs2, "will use %s for same-side diffs on Path2 only", b.opt.Compare.HashType2)
		}
	}
}

// returns true if the times are definitely different (by more than the modify window).
// returns false if equal, within modify window, or if either is unknown.
// considers precision per-Fs.
func timeDiffers(ctx context.Context, a, b time.Time, fsA, fsB fs.Fs) bool {
	modifyWindow := fs.GetModifyWindow(ctx, fsA, fsB)
	if modifyWindow == fs.ModTimeNotSupported {
		return false
	}
	if a.IsZero() || b.IsZero() {
		fs.Logf(fsA, "Fs supports modtime, but modtime is missing")
		return false
	}
	dt := b.Sub(a)
	if dt < modifyWindow && dt > -modifyWindow {
		fs.Debugf(a, "modification time the same (differ by %s, within tolerance %s)", dt, modifyWindow)
		return false
	}

	fs.Debugf(a, "Modification times differ by %s: %v, %v", dt, a, b)
	return true
}

func (b *bisyncRun) setFromCompareFlag() error {
	if b.opt.CompareFlag == "" {
		return nil
	}
	opts := strings.Split(b.opt.CompareFlag, ",")
	for _, opt := range opts {
		switch strings.ToLower(strings.TrimSpace(opt)) {
		case "size":
			b.opt.Compare.Size = true
		case "modtime":
			b.opt.Compare.Modtime = true
		case "checksum":
			b.opt.Compare.Checksum = true
		default:
			return fmt.Errorf(Color(terminal.RedFg, "unknown compare option: %s (must be size, modtime, or checksum)"), opt)
		}
	}
	return nil
}
