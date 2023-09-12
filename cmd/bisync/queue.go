package bisync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/rclone/rclone/cmd/bisync/bilib"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/fs/sync"
)

// TODO: handle dirs, ..path1/2 renames, deletes, --resync

type Results struct {
	Src        string
	Dst        string
	Side       string
	Name       string
	Size       int64
	Modtime    time.Time
	ModtimeStr string
	Hash       string
	Flags      string
	Sigil      rune
	SigilStr   string
	Err        error
}

var logger = operations.NewLoggerOpt()

func FsPathIfAny(x fs.ObjectInfo) string {
	_, ok := x.(fs.Object)
	if x != nil && ok {
		return bilib.FsPath(x.Fs())
	}
	return ""
}

func WriteResults(ctx context.Context, sigil rune, src fs.ObjectInfo, dst fs.ObjectInfo, err error) {
	result := Results{
		SigilStr: operations.TranslateSigil(sigil),
		Sigil:    sigil,
		Src:      FsPathIfAny(src),
		Dst:      FsPathIfAny(dst),
		Err:      err,
	}

	fss := []fs.ObjectInfo{src, dst}
	for _, side := range fss {

		if side == nil {
			continue
		}

		result.Side = FsPathIfAny(side)
		result.Name = side.Remote()
		result.Flags = "-"
		result.Size = side.Size()
		result.Modtime = side.ModTime(ctx).In(time.UTC)
		result.ModtimeStr = side.ModTime(ctx).In(time.UTC).Format(timeFormat)
		result.Hash, _ = side.Hash(ctx, side.Fs().Hashes().GetOne())
		// TODO: respect --ignore-checksum and --ignore-listing-checksum

		fs.Debugf(nil, "writing result: %v", result)
		json.NewEncoder(logger.JSON).Encode(result)
	}
}

func ReadResults(results io.Reader) []Results {
	dec := json.NewDecoder(results)
	var slice []Results
	for {
		var r Results
		if err := dec.Decode(&r); err == io.EOF {
			break
		}
		fs.Debugf(nil, "result: %v", r)
		slice = append(slice, r)
	}
	// fs.Debugf(nil, "Got results: %v", slice)
	return slice
}

func (b *bisyncRun) fastCopy(ctx context.Context, fsrc, fdst fs.Fs, files bilib.Names, queueName string) error {
	if err := b.saveQueue(files, queueName); err != nil {
		return err
	}

	ctxCopy, filterCopy := filter.AddConfig(b.opt.setDryRun(ctx))
	for _, file := range files.ToList() {
		if err := filterCopy.AddFile(file); err != nil {
			return err
		}
	}

	logger.LoggerFn = WriteResults
	ctxCopyLogger := operations.NewSyncLogger(ctxCopy, logger)
	err := sync.CopyDir(ctxCopyLogger, fdst, fsrc, b.opt.CreateEmptySrcDirs)
	fs.Debugf(nil, "logger is: %v", logger)

	getResults := ReadResults(logger.JSON)
	fs.Debugf(nil, "Got %v results for %v", len(getResults), queueName) // TODO: use the results!

	// Example of using results:
	lineFormat := "%s %8d %s %s %s %q\n"
	for _, result := range getResults {
		fs.Debugf(nil, lineFormat, result.Flags, result.Size, result.Hash, "", result.ModtimeStr, result.Name)
	}

	return err
}

func (b *bisyncRun) fastDelete(ctx context.Context, f fs.Fs, files bilib.Names, queueName string) error {
	if err := b.saveQueue(files, queueName); err != nil {
		return err
	}

	transfers := fs.GetConfig(ctx).Transfers

	ctxRun, filterDelete := filter.AddConfig(b.opt.setDryRun(ctx))

	for _, file := range files.ToList() {
		if err := filterDelete.AddFile(file); err != nil {
			return err
		}
	}

	objChan := make(fs.ObjectsChan, transfers)
	errChan := make(chan error, 1)
	go func() {
		errChan <- operations.DeleteFiles(ctxRun, objChan)
	}()
	err := operations.ListFn(ctxRun, f, func(obj fs.Object) {
		remote := obj.Remote()
		if files.Has(remote) {
			objChan <- obj
		}
	})
	close(objChan)
	opErr := <-errChan
	if err == nil {
		err = opErr
	}
	return err
}

// operation should be "make" or "remove"
func (b *bisyncRun) syncEmptyDirs(ctx context.Context, dst fs.Fs, candidates bilib.Names, dirsList *fileList, operation string) {
	if b.opt.CreateEmptySrcDirs && (!b.opt.Resync || operation == "make") {

		candidatesList := candidates.ToList()
		if operation == "remove" {
			// reverse the sort order to ensure we remove subdirs before parent dirs
			sort.Sort(sort.Reverse(sort.StringSlice(candidatesList)))
		}

		for _, s := range candidatesList {
			var direrr error
			if dirsList.has(s) { //make sure it's a dir, not a file
				if operation == "remove" {
					//note: we need to use Rmdirs instead of Rmdir because directories will fail to delete if they have other empty dirs inside of them.
					direrr = operations.Rmdirs(ctx, dst, s, false)
				} else if operation == "make" {
					direrr = operations.Mkdir(ctx, dst, s)
				} else {
					direrr = fmt.Errorf("invalid operation. Expected 'make' or 'remove', received '%q'", operation)
				}

				if direrr != nil {
					fs.Debugf(nil, "Error syncing directory: %v", direrr)
				}
			}
		}
	}
}

func (b *bisyncRun) saveQueue(files bilib.Names, jobName string) error {
	if !b.opt.SaveQueues {
		return nil
	}
	queueFile := fmt.Sprintf("%s.%s.que", b.basePath, jobName)
	return files.Save(queueFile)
}
