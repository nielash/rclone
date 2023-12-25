// Package profiles handles presets for config
package profiles

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configflags"
	"github.com/rclone/rclone/fs/operations"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	ignoredFlags = []string{"use-profile", "save-profile", "profile-include-args", "profile-strict-flags", "flags-from", "type"}
)

const (
	argPrefix = "arg_"
	parentKey = "parent_profiles"
)

// ProfileOpt stores the settings for Profiles
type ProfileOpt struct {
	SaveProfile string          // name of profile to save
	UseProfile  fs.CommaSepList // name(s) of profiles to use
	StrictFlags bool            // If set, --use-profile will error if any flags are invalid for this command, instead of ignoring.
	IncludeArgs bool            // Include args (ex. the paths being synced) in addition to flags when saving/using a profile
}

// Profile represents a flag/arg configuration to save/use as a reusable preset
type Profile struct {
	Name    string            // the name of the profile
	Args    []string          // command args (ex. the paths being synced)
	Flags   map[string]string // any flags (command, config, backend...)
	Parents []string          // see p.GetParents() to get a slice of *Profiles instead
}

// NewProfile returns a new empty *Profile
func NewProfile(name string) *Profile {
	return &Profile{
		Name:    name,
		Args:    []string{},
		Flags:   map[string]string{},
		Parents: []string{},
	}
}

// ProfileOptFromCtx gets a new ProfileOpt from config settings
func ProfileOptFromCtx(ctx context.Context) ProfileOpt {
	ci := fs.GetConfig(ctx)
	return ProfileOpt{
		SaveProfile: ci.SaveProfile,
		UseProfile:  ci.UseProfile,
		StrictFlags: ci.ProfileStrictFlags,
		IncludeArgs: ci.ProfileIncludeArgs,
	}
}

// NewFromCommand sets the profile's Args and Flags from the cmd and args passed in
func (p *Profile) NewFromCommand(cmd *cobra.Command, args []string) {
	p.Args = args

	setProfileVal := func(flag *pflag.Flag) {
		fmt.Println(flag.Name, flag.Value.String())
		if slices.ContainsFunc[[]string, string](ignoredFlags, func(s string) bool { return s == flag.Name }) {
			return
		}
		if !flag.Changed {
			return
		}
		p.Flags[flag.Name] = flag.Value.String()
	}

	// note that for some reason, .Visit() does not visit the flags we add manually in the setprofile command with --flags-from,
	// but .VisitAll() with flag.Changed does visit them. So that's why we use .VisitAll() here.
	cmd.Flags().VisitAll(setProfileVal)
}

// GetProfile gets a profile from the config
func GetProfile(name string) (*Profile, error) {
	p := NewProfile(name)
	data := config.LoadedData()
	if !data.HasSection(p.Name) {
		return nil, fmt.Errorf("no section named %s found in config file", p.Name)
	}

	keys := data.GetKeyList(p.Name)
	for _, key := range keys {
		if slices.ContainsFunc[[]string, string](ignoredFlags, func(s string) bool { return optionToProfileKey(s) == key }) {
			continue
		}

		if key == parentKey {
			parentList, _ := config.LoadedData().GetValue(p.Name, key)
			p.Parents = strings.Split(parentList, ",")
			continue
		}

		if strings.HasPrefix(key, argPrefix) {
			i, err := argNum(key) // don't assume they're already in order!
			if err != nil {
				return nil, fmt.Errorf("error parsing arg %s: %v", key, err)
			}
			val, _ := config.LoadedData().GetValue(p.Name, key)
			if i > len(p.Args)-1 {
				p.Args = append(p.Args, "")
			}
			p.Args[i] = val
			continue
		}

		val, _ := config.LoadedData().GetValue(p.Name, key)
		p.Flags[profileKeyToFlag(key)] = val
	}
	return p, nil
}

// GetProfiles gets a slice of profiles
func GetProfiles(names []string) ([]*Profile, error) {
	profiles := []*Profile{}
	for _, name := range names {
		p, err := GetProfile(name)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// UseProfiles gets one or more Profiles from opt.UseProfile and applies them to context (priority: lowest to highest)
func UseProfiles(ctx context.Context, cmd *cobra.Command, args []string, opt *ProfileOpt) error {
	for _, profileName := range opt.UseProfile {
		err := GetAndUseProfile(ctx, cmd, args, opt, profileName)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetAndUseProfile gets one Profile and uses it
func GetAndUseProfile(ctx context.Context, cmd *cobra.Command, args []string, opt *ProfileOpt, profileName string) error {
	profile, err := GetProfile(profileName)
	if err != nil {
		return err
	}

	err = profile.Use(ctx, cmd, args, opt)
	if err != nil {
		return err
	}
	return nil
}

// Save saves a profile to the config file
func (p *Profile) Save(ctx context.Context, opt *ProfileOpt) error {
	if p.Name == "default_profile" {
		return errors.New("profile name 'default_profile' is reserved and can only be set from config")
	}

	var err error
	p.Name, err = validateProfileName(p.Name)
	if err != nil {
		return err
	}

	if operations.SkipDestructive(ctx, p.Name, "save profile") {
		return nil
	}

	config.LoadedData().DeleteSection(p.Name) // clear it if already exists
	err = config.LoadedData().Save()
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}

	config.LoadedData().SetValue(p.Name, "type", "profile")
	for k, v := range p.Flags {
		config.LoadedData().SetValue(p.Name, optionToProfileKey(k), v)
	}

	if opt.IncludeArgs {
		p.saveArgs()
	}

	if len(p.Parents) > 0 {
		config.LoadedData().SetValue(p.Name, parentKey, strings.Join(p.Parents, ","))
	}

	err = config.LoadedData().Save()
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}

	fs.Debugf(p.Name, "saved profile: %v", SprintSection(p.Name))
	return nil
}

func (p *Profile) saveArgs() {
	for i, arg := range p.Args {
		config.LoadedData().SetValue(p.Name, argPrefix+fmt.Sprint(i+1), arg)
	}
}

// Use sets the current command args and flags from the profile struct
func (p *Profile) Use(ctx context.Context, cmd *cobra.Command, cobraArgs []string, opt *ProfileOpt) error {
	// set logging first
	ci := fs.GetConfig(ctx)
	if ci.LogLevel == fs.LogLevelNotice {
		_ = cmd.Flags().Lookup("verbose").Value.Set(config.FileGet(p.Name, "verbose"))
		configflags.SetFlags(ci)
	}
	fs.Debugf(p.Name, "loading profile: %v", SprintSection(p.Name))

	err := p.useParents(ctx, cmd, cobraArgs, opt)
	if err != nil {
		return err
	}

	err = p.useFlags(ctx, cmd, cobraArgs, opt)
	if err != nil {
		return err
	}

	if opt.IncludeArgs {
		p.useArgs(ctx, cmd, cobraArgs, opt)
	}

	// run configflags.SetFlags() again in case our changes changed the analysis
	configflags.SetFlags(ci)
	return nil

}

func (p *Profile) useFlags(ctx context.Context, cmd *cobra.Command, cobraArgs []string, opt *ProfileOpt) error {
	ci := fs.GetConfig(ctx)
	for key, val := range p.Flags {
		if slices.ContainsFunc[[]string, string](ignoredFlags, func(s string) bool { return optionToProfileKey(s) == key }) {
			continue
		}

		flag := cmd.Flags().Lookup(profileKeyToFlag(key))
		if flag == nil {
			if opt.StrictFlags {
				return fmt.Errorf("invalid flag: %s", profileKeyToFlag(key))
			}
		}

		if ci.DryRun && key == "dry_run" && val == "false" {
			// disallow overriding --dry-run if it was specifically set
			fs.Logf(nil, "for safety, profiles cannot change --dry-run from true to false. Ignoring.")
			continue
		}
		logChange(p.Name, key, flag.Value.String(), val)
		err := flag.Value.Set(val)
		if err != nil {
			return fmt.Errorf("%s: error setting val %s from profile: %v", flag.Name, val, err)
		}
	}
	return nil
}

// Note that args should probably not be used on parent profiles generally (children would overwrite parent), but I guess I can imagine a few use cases
// Note also that we don't CheckArgs() here as the command will do it later. Users should take care to not exceed MaxArgs.
func (p *Profile) useArgs(ctx context.Context, cmd *cobra.Command, cobraArgs []string, opt *ProfileOpt) {
	for i, pArg := range p.Args { // we DO assume that the p.Args are in the right order here.
		if pArg == "" {
			continue
		}
		if i > len(cobraArgs)-1 {
			cobraArgs = append(cobraArgs, "")
		}
		logChange(p.Name, fmt.Sprintf("Arg %d", i+1), cobraArgs[i], pArg)
		cobraArgs[i] = pArg
	}

	// rclone command [args...]
	os.Args = append([]string{os.Args[0], os.Args[1]}, cobraArgs...)
	os.Args = cleanArgs(os.Args)
}

// allows nested profiles (priority lowest to highest, parents all lower than child)
func (p *Profile) useParents(ctx context.Context, cmd *cobra.Command, args []string, opt *ProfileOpt) error {
	if len(p.Parents) == 0 {
		return nil
	}
	newopt := *opt
	newopt.UseProfile = p.Parents
	return UseProfiles(ctx, cmd, args, &newopt)
}

// GetParents returns the parent profiles of this profile
func (p *Profile) GetParents() ([]*Profile, error) {
	return GetProfiles(p.Parents)
}

// SetParents sets one or more parent profiles for a child profile
func SetParents(child string, parents []string) error {
	config.LoadedData().SetValue(child, parentKey, strings.Join(parents, ","))
	err := config.LoadedData().Save()
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}
	return nil
}

/*
TODO:
- should we be storing more in memory to cut down on config.LoadedData() calls?
- when profile/key names have illegal characters should we error or auto-convert them? currently we do some of both.
- more tests
- docs
- document handling of blank or default values
- UI could use some cleanup (and see the note there about the use of the word "remote")
- rc methods
- API for other commands (bisync) to save/use profiles
- better handle scenario where logging level passed in is higher than that of the profile we're getting/using...
...probably needs a defer to make sure debugs are seen (if requested) during profile setting, then set the real level (from profile) after
- maybe edit more of the helper functions to use the types and methods instead of modifying config directly
- the ProfileOpt logic maybe needs another look -- slightly confusing that we store the profile name both there and in the profile struct.
- search other TODOs
*/