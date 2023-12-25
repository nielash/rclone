// Package profiles handles presets for config
package profiles

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/config/configflags"
	"github.com/rclone/rclone/fs/fspath"
	fslog "github.com/rclone/rclone/fs/log"
	"github.com/spf13/cobra"
)

// AddProfiles handles saving/loading of config profiles
// We do this before Root.Execute() because there's otherwise
// no good way to add args to an already-executing command
func AddProfiles(ctx context.Context, Root *cobra.Command) {
	if !slices.ContainsFunc[[]string, string](os.Args, func(E string) bool { return E == "--use-profile" || E == "--save-profile" }) && !hasDefaultProfile() {
		return
	}
	cmd, _, err := Root.Find(os.Args[1:])
	if err != nil {
		log.Fatalf("Fatal error: %v", err)
	}
	if cmd.Name() == "setprofile" {
		return
	}
	initConfig(ctx)
	err = cmd.Flags().Parse(os.Args[2:])
	if err != nil {
		log.Fatalf("Fatal error: %v", err)
	}

	err = HandleProfiles(ctx, cmd, cmd.Flags().Args())
	if err != nil {
		log.Fatalf("Fatal error: %v", err)
	}
}

// HandleProfiles handles --save-profile and --use-profile
func HandleProfiles(ctx context.Context, cmd *cobra.Command, args []string) error {
	opt := ProfileOptFromCtx(ctx)
	handleDefaultProfile(&opt)

	if len(opt.UseProfile) > 0 {
		err := UseProfiles(ctx, cmd, args, &opt)
		if err != nil {
			return err
		}
	}

	if opt.SaveProfile != "" {
		saveProfile := NewProfile(opt.SaveProfile)
		saveProfile.NewFromCommand(cmd, args)
		return saveProfile.Save(ctx, &opt)
	}
	return nil
}

func argNum(key string) (int, error) {
	s := strings.TrimPrefix(key, argPrefix)
	i, err := strconv.Atoi(s)
	if err != nil {
		return i, err
	}
	return i - 1, nil
}

func optionToProfileKey(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "_")
	return strings.ToLower(strings.ReplaceAll(name, "-", "_"))
}

// note that this is case-insensitive
func profileKeyToFlag(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func validateKeyName(name string) (convertedName string, err error) {
	convertedName = fspath.MakeConfigName(optionToProfileKey(name))
	if convertedName != name {
		return convertedName, errors.New("invalid characters detected")
	}
	return convertedName, nil
}

func validateProfileName(name string) (convertedName string, err error) {
	convertedName, err = validateKeyName(name)
	err2 := checkNameConflict(convertedName)
	if err2 != nil {
		return convertedName, err2
	}
	return convertedName, err
}

func checkNameConflict(name string) error {
	remoteType, found := config.LoadedData().GetValue(name, "type")
	if !found {
		return nil
	}
	if remoteType != "profile" {
		return fmt.Errorf("non-profile remote already exists with name %s (type: %s)", name, remoteType)
	}
	return nil
}

func logChange(useProfileName, key, oldval, newval string) {
	if oldval == newval {
		return
	}
	if oldval == "" {
		oldval = "[blank]"
	}
	fs.Infof(useProfileName, "changing %s from %s to %s", key, oldval, newval)
}

// SprintSection returns a string of this section from the config file
func SprintSection(name string) string {
	keys := config.LoadedData().GetKeyList(name)
	s := fmt.Sprintf("\n[%s]\n", name)
	for _, key := range keys {
		s += fmt.Sprintf("%s%s%s\n", key, " = ", config.FileGet(name, key))
	}
	return s
}

/*
	Special "default" profile with only one parameter:

[default_profile]
profile = nameofsomeprofile

If set, it will always be used unless overridden by --use-profile.
Intended use case is users who can't set any command line flags or env vars,
and need some way of setting --use-profile from the config file.
*/
func handleDefaultProfile(opt *ProfileOpt) {
	if len(opt.UseProfile) > 0 {
		return
	}

	val, found := config.LoadedData().GetValue("default_profile", "profile")
	if !found {
		return
	}

	opt.UseProfile = []string{val}
}

func hasDefaultProfile() bool {
	_, found := config.LoadedData().GetValue("default_profile", "profile")
	return found
}

// SetDefaultProfile sets the default profile in the config
func SetDefaultProfile(name string) error {
	config.LoadedData().SetValue("default_profile", "type", "profile")
	config.LoadedData().SetValue("default_profile", "profile", name)
	err := config.LoadedData().Save()
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}
	return nil
}

// DeleteProfileKey deletes a key (and its value) from a profile in the config
func DeleteProfileKey(profile, key string) {
	if key == "type" {
		fs.Errorf(profile, "deleting 'type' key is not allowed")
		return
	}
	config.LoadedData().DeleteKey(profile, key)
}

func cleanArgs(s []string) []string {
	var r []string
	// s = s[:cap(s)]
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func initConfig(ctx context.Context) {
	ci := fs.GetConfig(ctx)

	// Start the logger
	fslog.InitLogging()

	// Finish parsing any command line flags
	configflags.SetFlags(ci)

	// Load the config
	configfile.Install()
}
