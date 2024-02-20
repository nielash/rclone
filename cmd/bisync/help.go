package bisync

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func makeHelp(help string) string {
	replacer := strings.NewReplacer(
		"||", "`",
		"{MAXDELETE}", strconv.Itoa(DefaultMaxDelete),
		"{CHECKFILE}", DefaultCheckFilename,
		// "{WORKDIR}", DefaultWorkdir,
	)
	return replacer.Replace(help)
}

var shortHelp = `Perform bidirectional synchronization between two paths.`

// RcHelp returns the rc help
func RcHelp() string {
	return makeHelp(`This takes the following parameters:

- path1 (required) - (string) a remote directory string e.g. ||drive:path1||
- path2 (required) - (string) a remote directory string e.g. ||drive:path2||
- dryRun - (bool) dry-run mode
` + GenerateParams() + `


See [bisync command help](https://rclone.org/commands/rclone_bisync/)
and [full bisync description](https://rclone.org/bisync/)
for more information.`)
}

var longHelp = shortHelp + makeHelp(`

[Bisync](https://rclone.org/bisync/) provides a
bidirectional cloud sync solution in rclone.
It retains the Path1 and Path2 filesystem listings from the prior run.
On each successive run it will:
- list files on Path1 and Path2, and check for changes on each side.
  Changes include ||New||, ||Newer||, ||Older||, and ||Deleted|| files.
- Propagate changes on Path1 to Path2, and vice-versa.

Bisync is **in beta** and is considered an **advanced command**, so use with care.
Make sure you have read and understood the entire [manual](https://rclone.org/bisync)
(especially the [Limitations](https://rclone.org/bisync/#limitations) section) before using,
or data loss can result. Questions can be asked in the [Rclone Forum](https://forum.rclone.org/).

See [full bisync description](https://rclone.org/bisync/) for details.
`)

// example: "create-empty-src-dirs" -> "createEmptySrcDirs"
func toCamel(s string) string {
	split := strings.Split(s, "-")
	builder := strings.Builder{}
	for i, word := range split {
		if i == 0 { // first word always all lowercase
			builder.WriteString(strings.ToLower(word))
			continue
		}
		builder.WriteString(cases.Title(language.AmericanEnglish).String(word))
	}
	return builder.String()
}

// GenerateParams automatically generates the param list from commandDefinition.Flags
func GenerateParams() string {
	builder := strings.Builder{}
	fn := func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		builder.WriteString(fmt.Sprintf("- %s - (%s) %s  \n", toCamel(flag.Name), flag.Value.Type(), flag.Usage))
	}
	commandDefinition.Flags().VisitAll(fn)
	return builder.String()
}
