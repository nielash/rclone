// Package setprofile provides the setprofile command.
package setprofile

import (
	"context"
	"errors"
	"os"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs"
	profiles "github.com/rclone/rclone/fs/config/configprofiles"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/spf13/cobra"
)

var (
	FlagsFrom fs.CommaSepList // FlagsFrom imports command-specific flags from these subcommands (note: will error if commands have conflicting flags.)
)

func init() {
	cmd.Root.AddCommand(commandDefinition)
	cmdFlags := commandDefinition.Flags()
	flags.FVarP(cmdFlags, &FlagsFrom, "flags-from", "", "If set, import command-specific flags from these subcommands (note: will error if commands have conflicting flags.)", "")

	commandDefinition.PersistentPreRunE = addFlagsFrom
	commandDefinition.FParseErrWhitelist.UnknownFlags = true // allow unknown flags until we do our custom parsing
}

func addFlagsFrom(setprofilecmd *cobra.Command, args []string) error {
	// prevent verbose getting double-counted (it's a CountVarP)
	verbose := setprofilecmd.Flags().Lookup("verbose").Value.String()
	defer func() {
		err := setprofilecmd.Flags().Lookup("verbose").Value.Set(verbose)
		if err != nil {
			fs.Errorf(nil, "error trying to set verbose to %s: %v", verbose, err)
		}
	}()

	if len(FlagsFrom) > 0 {
		for _, subcommand := range FlagsFrom {
			sc, _, err := cmd.Root.Find([]string{subcommand})
			if err != nil {
				return err
			}
			setprofilecmd.Flags().AddFlagSet(sc.Flags())
		}
	}
	// now reenable validation and reparse the flags
	commandDefinition.FParseErrWhitelist.UnknownFlags = false
	return setprofilecmd.ParseFlags(os.Args)
}

var commandDefinition = &cobra.Command{
	Use:   "setprofile [args...] [flags...] --save-profile PROFILENAME",
	Short: `Save a profile with the args and flags passed in.`,
	Long: `
This command does nothing except set a profile.

For more detailed profile config options, see:
	rclone config profile
`,
	Annotations: map[string]string{
		"groups": "Config",
	},
	Run: func(command *cobra.Command, args []string) {
		cmd.CheckArgs(0, 100, command, args)
		cmd.Run(false, false, command, func() error {
			ctx, ci := fs.AddConfig(context.Background())
			if ci.SaveProfile == "" {
				return errors.New("must use --save-profile PROFILENAME with this command")
			}
			return profiles.HandleProfiles(ctx, command, args)
		})
	},
}
