package bisync

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/cmd/bisync/bilib"
	"github.com/rclone/rclone/fs/accounting"
	"github.com/rclone/rclone/fs/config"
	fslog "github.com/rclone/rclone/fs/log"
	"github.com/rclone/rclone/fs/operations"
)

// strip ansi
const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

func strip(str string) string {
	return re.ReplaceAllString(str, "")
}

// GUIEvent is a row of data for the Bisync GUI
type GUIEvent struct {
	Session     string
	RunID       string
	Path1       string
	Path2       string
	Start       time.Time
	End         time.Time
	StartString string
	EndString   string
	Duration    time.Duration
	Icon        string
	Status      string
	Stats       string
	Details     string
	Filters     string
	Args        string
	Path1Lst    string
	Path2Lst    string
}

func (b *bisyncRun) gui(g GUIEvent) {

	if b.opt.DryRun || !b.opt.GUI {
		return
	}

	check := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}

	// set the filenames for HTML and data file
	// note: we create the files locally regardless, and move the HTML after if --gui-dir is non-local
	path := filepath.Join(b.workDir, "bisync_status")
	name := path + ".html"
	datafile := path + "_data.json"

	data := struct {
		Title      string
		Time       string
		Workdir    string
		Config     string
		Log        string
		RefreshInt int
		Rows       []GUIEvent
	}{}

	// get existing data from file, if any
	if bilib.FileExists(datafile) {
		rdf, err := os.Open(datafile)
		b.handleErr(datafile, "error reading data file", err, false, true)
		dec := json.NewDecoder(rdf)
		for {
			if err := dec.Decode(&data); err == io.EOF {
				break
			}
		}
		b.handleErr(datafile, "error closing file", rdf.Close(), false, true)
	}

	// processing specific to this run
	timeFormat := "Mon Jan 2, 2006 3:04:05 PM"
	timeToString := func(t time.Time) string {
		return t.Format(timeFormat) + "\n(" + time.Since(t).Round(time.Second).String() + " ago)"
	}
	// add the resync symbol
	if b.opt.Resync {
		g.Icon += "🔂"
	}

	// if we don't already have stats from statsInterval, grab them manually
	if g.Stats == "" {
		g.Stats = accounting.Stats(b.octx).String()
	}
	// set other run-specific variables
	g.Filters = b.opt.FiltersFile
	g.Args = strings.TrimSuffix(strings.TrimPrefix(fmt.Sprintf("%q", os.Args), "["), "]")
	g.Path1Lst = b.listing1
	g.Path2Lst = b.listing2
	g.Status = strip(g.Status)

	if !g.End.IsZero() {
		g.Duration = g.End.Sub(g.Start).Round(time.Second)
	} else {
		g.Duration = time.Since(g.Start).Round(time.Second)
	}

	// delete existing row with same RunID
	for i := range data.Rows {
		if data.Rows[i].RunID == g.RunID {
			data.Rows = append(data.Rows[:i], data.Rows[i+1:]...)
			break
		}
	}

	// sort
	data.Rows = append(data.Rows, g)
	sort.Slice(data.Rows, func(i, j int) bool {
		return data.Rows[i].Start.After(data.Rows[j].Start)
	})

	// prune list
	if len(data.Rows) > b.opt.GUIMaxRows {
		data.Rows = data.Rows[:b.opt.GUIMaxRows]
	}

	// recalculate all "since" times
	for i, row := range data.Rows {
		data.Rows[i].StartString = timeToString(row.Start)
		if !row.End.IsZero() {
			data.Rows[i].EndString = timeToString(row.End)
		}
	}
	// set global variables
	data.Title = g.Icon + " Bisync Status"
	data.Workdir = b.workDir
	data.Config = config.GetConfigPath()
	data.Log = fslog.Opt.File
	data.RefreshInt = int(b.opt.GUIRefreshInt.Round(time.Second).Seconds())
	data.Time = time.Now().Format(timeFormat)

	// save data file
	df, err := os.Create(datafile)
	b.handleErr(datafile, "error writing data file", err, false, true)
	b.handleErr(datafile, "error encoding JSON", json.NewEncoder(df).Encode(data), false, true)
	b.handleErr(datafile, "error closing file", df.Close(), false, true)

	// create/update HTML file from template
	f, err := os.Create(name)
	b.handleErr(name, "error writing file", err, false, true)
	t, err := template.New("webpage").Parse(tpl)
	check(err)
	err = t.Execute(f, data)
	check(err)
	b.handleErr(name, "error closing file", f.Close(), false, true)

	// move it to user-specified remote if necessary
	b.moveFile(name, b.GUIurl)
}

func (b *bisyncRun) moveFile(src, dst string) {
	if src == dst {
		return
	}
	args := []string{dst, ""}
	fsrc, remote := cmd.NewFsFile(src)
	fdst, dstFileName := cmd.NewFsDstFile(args)
	err = operations.MoveFile(b.octx, fdst, fsrc, dstFileName, remote)
	b.handleErr(dst, "error moving HTML file", err, false, true)
}

// StartStats prints the stats every statsInterval
//
// It returns a func which should be called to stop the stats.
func (b *bisyncRun) StartStats(g GUIEvent) func() {
	if b.opt.GUIStatsInt <= 0 || !b.opt.GUI {
		return func() {}
	}
	stopStats := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(b.opt.GUIStatsInt)
		for {
			select {
			case <-ticker.C:
				g.Stats = accounting.GlobalStats().String()
				b.gui(g)
			case <-stopStats:
				ticker.Stop()
				return
			}
		}
	}()
	return func() {
		close(stopStats)
		wg.Wait()
	}
}
