package bisync

import (
	"bytes"
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

type Results struct {
	Name       string
	Size       int64
	Modtime    time.Time
	ModtimeStr string
	Hash       string
	Flags      string
	Action     string
	Err        error
}

func getResults(results io.Reader) []Results {
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
	fs.Debugf(nil, "Got results: %v", slice)
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

	results := new(bytes.Buffer)
	syncErr := sync.BisyncDir(ctxCopy, fdst, fsrc, b.opt.CreateEmptySrcDirs, results)
	getResults := getResults(results)
	fs.Debugf(nil, "Got %v results for %v", len(getResults), queueName) // TODO: use the results!

	// Example of using results:
	lineFormat := "%s %8d %s %s %s %q\n"
	for _, result := range getResults {
		fs.Debugf(nil, lineFormat, result.Flags, result.Size, result.Hash, "", result.ModtimeStr, result.Name)
	}

	return syncErr
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

	results := new(bytes.Buffer)
	objChan := make(fs.ObjectsChan, transfers)
	errChan := make(chan error, 1)
	go func() {
		errChan <- operations.DeleteFiles(ctxRun, objChan, results)
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
	getResults := getResults(results)
	fs.Debugf(nil, "Got %v results for %v", len(getResults), queueName) // TODO: use the results!
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

		results := new(bytes.Buffer)

		for _, s := range candidatesList {
			var direrr error
			if dirsList.has(s) { //make sure it's a dir, not a file
				if operation == "remove" {
					//note: we need to use Rmdirs instead of Rmdir because directories will fail to delete if they have other empty dirs inside of them.
					direrr = operations.Rmdirs(ctx, dst, s, false, results)
				} else if operation == "make" {
					direrr = operations.Mkdir(ctx, dst, s, results)
				} else {
					direrr = fmt.Errorf("invalid operation. Expected 'make' or 'remove', received '%q'", operation)
				}

				if direrr != nil {
					fs.Debugf(nil, "Error syncing directory: %v", direrr)
				}
			}
		}

		getResults := getResults(results)
		fs.Debugf(nil, "Got %v results for %v empty dirs: %v", len(getResults), operation, dst.Root()) // TODO: use the results!
	}
}

func (b *bisyncRun) saveQueue(files bilib.Names, jobName string) error {
	if !b.opt.SaveQueues {
		return nil
	}
	queueFile := fmt.Sprintf("%s.%s.que", b.basePath, jobName)
	return files.Save(queueFile)
}
