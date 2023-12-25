// Package profiles handles presets for config
package profiles_test

import (
	"log"
	"os"
	"regexp"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/accounting"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/config/configflags"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/filter/filterflags"
	fslog "github.com/rclone/rclone/fs/log"
	"github.com/rclone/rclone/fs/log/logflags"
	"github.com/rclone/rclone/fs/rc/rcflags"
	"github.com/rclone/rclone/fs/rc/rcserver"
	"github.com/rclone/rclone/fstest"
	"github.com/rclone/rclone/lib/terminal"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	Root         = cmd.Root
	backendFlags map[string]struct{}
	flagsRe      *regexp.Regexp
)

// Some times used in the tests
var (
	t1 = fstest.Time("2001-02-03T04:05:06.499999999Z")
	// t2 = fstest.Time("2011-12-25T12:59:59.123456789Z")
	// t3 = fstest.Time("2011-12-30T12:59:59.000000000Z")
)

/*
These are all just helpers for the tests.
Most of this is from cmd/cmd.go, to mock what rclone would normally do before executing a command.
It's important in our case, as it affects the availability of flags, and their values.
We can't call it directly as some of the functions are not exported, and we need to be able to reset the state between tests.
*/

// setupRootCommand sets default usage, help, and error handling for
// the root command.
//
// Helpful example: https://github.com/moby/moby/blob/master/cli/cobra.go
func setupRootCommand(rootCmd *cobra.Command) {
	ci := fs.GetConfig(ctx)
	// Add global flags
	configflags.AddFlags(ci, pflag.CommandLine)
	filterflags.AddFlags(pflag.CommandLine)
	rcflags.AddFlags(pflag.CommandLine)
	logflags.AddFlags(pflag.CommandLine)

	// Root.Run = runRoot
	// Root.Flags().BoolVarP(&version, "version", "V", false, "Print the version number")
	// Root.PersistentPreRunE = profiles.HandleProfiles

	cobra.AddTemplateFunc("showGlobalFlags", func(cmd *cobra.Command) bool {
		return cmd.CalledAs() == "flags" || cmd.Annotations["groups"] != ""
	})
	cobra.AddTemplateFunc("showCommands", func(cmd *cobra.Command) bool {
		return cmd.CalledAs() != "flags"
	})
	cobra.AddTemplateFunc("showLocalFlags", func(cmd *cobra.Command) bool {
		// Don't show local flags (which are the global ones on the root) on "rclone" and
		// "rclone help" (which shows the global help)
		return cmd.CalledAs() != "rclone" && cmd.CalledAs() != ""
	})
	cobra.AddTemplateFunc("flagGroups", func(cmd *cobra.Command) []*flags.Group {
		// Add the backend flags and check all flags
		backendGroup := flags.All.NewGroup("Backend", "Backend only flags. These can be set in the config file also.")
		allRegistered := flags.All.AllRegistered()
		cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
			if _, ok := backendFlags[flag.Name]; ok {
				backendGroup.Add(flag)
			} else if _, ok := allRegistered[flag]; ok {
				// flag is in a group already
			} else {
				fs.Errorf(nil, "Flag --%s is unknown", flag.Name)
			}
		})
		groups := flags.All.Filter(flagsRe).Include(cmd.Annotations["groups"])
		return groups.Groups
	})
	/* 	rootCmd.SetUsageTemplate(usageTemplate)
	   	// rootCmd.SetHelpTemplate(helpTemplate)
	   	// rootCmd.SetFlagErrorFunc(FlagErrorFunc)
	   	rootCmd.SetHelpCommand(helpCommand)
	   	// rootCmd.PersistentFlags().BoolP("help", "h", false, "Print usage")
	   	// rootCmd.PersistentFlags().MarkShorthandDeprecated("help", "please use --help")

	   	rootCmd.AddCommand(helpCommand)
	   	helpCommand.AddCommand(helpFlags)
	   	helpCommand.AddCommand(helpBackends)
	   	helpCommand.AddCommand(helpBackend) */

	cobra.OnInitialize(initConfig)

}

// initConfig is run by cobra after initialising the flags
func initConfig() {
	ctx := ctx
	ci := fs.GetConfig(ctx)

	// Start the logger
	fslog.InitLogging()

	// Finish parsing any command line flags
	configflags.SetFlags(ci)

	// Load the config
	configfile.Install()

	// Start accounting
	accounting.Start(ctx)

	// Configure console
	if ci.NoConsole {
		// Hide the console window
		terminal.HideConsole()
	} else {
		// Enable color support on stdout if possible.
		// This enables virtual terminal processing on Windows 10,
		// adding native support for ANSI/VT100 escape sequences.
		terminal.EnableColorsStdout()
	}

	// Load filters
	err := filterflags.Reload(ctx)
	if err != nil {
		log.Fatalf("Failed to load filters: %v", err)
	}

	// Write the args for debug purposes
	fs.Debugf("rclone", "Version %q starting with parameters %q", fs.Version, os.Args)

	// Inform user about systemd log support now that we have a logger
	if fslog.Opt.LogSystemdSupport {
		fs.Debugf("rclone", "systemd logging support activated")
	}

	// Start the remote control server if configured
	_, err = rcserver.Start(ctx, &rcflags.Opt)
	if err != nil {
		log.Fatalf("Failed to start remote control: %v", err)
	}

	/* // Setup CPU profiling if desired
	if *cpuProfile != "" {
		fs.Infof(nil, "Creating CPU profile %q\n", *cpuProfile)
		f, err := os.Create(*cpuProfile)
		if err != nil {
			err = fs.CountError(err)
			log.Fatal(err)
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			err = fs.CountError(err)
			log.Fatal(err)
		}
		atexit.Register(func() {
			pprof.StopCPUProfile()
		})
	}

	// Setup memory profiling if desired
	if *memProfile != "" {
		atexit.Register(func() {
			fs.Infof(nil, "Saving Memory profile %q\n", *memProfile)
			f, err := os.Create(*memProfile)
			if err != nil {
				err = fs.CountError(err)
				log.Fatal(err)
			}
			err = pprof.WriteHeapProfile(f)
			if err != nil {
				err = fs.CountError(err)
				log.Fatal(err)
			}
			err = f.Close()
			if err != nil {
				err = fs.CountError(err)
				log.Fatal(err)
			}
		})
	} */

	/* if m, _ := regexp.MatchString("^(bits|bytes)$", *dataRateUnit); !m {
		fs.Errorf(nil, "Invalid unit passed to --stats-unit. Defaulting to bytes.")
		ci.DataRateUnit = "bytes"
	} else {
		ci.DataRateUnit = *dataRateUnit
	} */
	ci.DataRateUnit = "bytes"
}