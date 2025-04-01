// Package convmv provides the convmv command.
package convmv

import (
	"context"
	"errors"
	"strings"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/fs/sync"
	"github.com/rclone/rclone/lib/transform"
	"github.com/spf13/cobra"
)

// Globals
var (
	deleteEmptySrcDirs = false
	createEmptySrcDirs = false
)

func init() {
	cmd.Root.AddCommand(commandDefinition)
	cmdFlags := commandDefinition.Flags()
	flags.BoolVarP(cmdFlags, &deleteEmptySrcDirs, "delete-empty-src-dirs", "", deleteEmptySrcDirs, "Delete empty source dirs after move", "")
	flags.BoolVarP(cmdFlags, &createEmptySrcDirs, "create-empty-src-dirs", "", createEmptySrcDirs, "Create empty source dirs on destination after move", "")
}

var commandDefinition = &cobra.Command{
	Use:   "convmv dest:path --name-transform XXX",
	Short: `Convert file and directory names in place.`,
	// Warning! "|" will be replaced by backticks below
	Long: strings.ReplaceAll(`DOCS TODO!
`, "|", "`"),
	Annotations: map[string]string{
		"versionIntroduced": "v1.71",
		"groups":            "Filter,Listing,Important,Copy",
	},
	Run: func(command *cobra.Command, args []string) {
		cmd.CheckArgs(1, 1, command, args)
		fdst, srcFileName := cmd.NewFsFile(args[0])
		cmd.Run(false, true, command, func() error {
			if !transform.Transforming() {
				return errors.New("--name-transform must be set")
			}
			if srcFileName == "" {
				return sync.Transform(context.Background(), fdst, deleteEmptySrcDirs, createEmptySrcDirs)
			}
			return operations.TransformFile(context.Background(), fdst, srcFileName)
		})
	},
}
