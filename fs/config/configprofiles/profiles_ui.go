// Package profiles handles presets for config
package profiles

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/spf13/pflag"
)

// Register with Fs
func init() {
	fsi := &fs.RegInfo{
		Name:        "profile",
		Description: "Preset of config options",
		NewFs:       nope,
		Config:      profileConfig,
		Options: []fs.Option{{
			Name:     parentKey,
			Help:     "Comma-separated list of parent profiles that this (child) profile should inherit. (priority lowest to highest, parents all lower than child)",
			Required: false,
			Advanced: true,
		}},
		Hide: false, // TODO? (or would that defeat the purpose?)
	}
	fs.Register(fsi)
}

// Options defines the configuration for this "backend"
type Options struct {
	InheritProfiles fs.CommaSepList `config:"parent_profiles"`
}

func profileConfig(ctx context.Context, name string, m configmap.Mapper, configIn fs.ConfigIn) (*fs.ConfigOut, error) {
	switch configIn.State {
	case "", "menu":
		return fs.ConfigChooseExclusive("main_menu", "choice", "What would you like to do?", 8, func(i int) (itemValue string, itemHelp string) {
			switch i + 1 {
			case 1:
				return "default", "Set a default profile"
			case 2:
				return "parents", "Set parent profiles (inherit settings from other profiles)"
			case 3:
				return "show", "Show the options currently set for this profile"
			case 4:
				return "set", "Set an option by name"
			case 5:
				return "delete", "Delete an option by name"
			case 6:
				return "global", "List possible global options"
			case 7:
				return "backend", "List possible backend and global options"
			case 8:
				return "q", "Quit config"
			default:
				return "TODO!", "TODO!"
			}
		})
	case "main_menu":
		switch configIn.Result {
		case "default":
			return fs.ConfigInput("default_profile_name", "default profile name", "Name of the default profile")
		case "parents":
			if name == "default_profile" {
				return fs.ConfigResult("main_menu", "default")
			}
			return fs.ConfigInput("parents_names", "parent profile names", "Enter the names of the parent profile(s), separated by commas")
		case "show":
			return nil, nil
		case "set":
			if name == "default_profile" {
				return fs.ConfigResult("main_menu", "default")
			}
			return fs.ConfigInput("key_name", "option name", "Config name for the option")
		case "delete":
			if name == "default_profile" {
				return fs.ConfigResult("main_menu", "default")
			}
			keys := config.LoadedData().GetKeyList(name)
			keys = slices.DeleteFunc[[]string, string](keys, func(s string) bool { return s == "type" }) // disallow deleting "type"
			return fs.ConfigChooseExclusive("delete_key", "option to delete", "Delete which option?", len(keys)+1, func(i int) (itemValue string, itemHelp string) {
				if i >= len(keys) {
					return "cancel", "(nevermind - do not delete)"
				}
				val, _ := config.LoadedData().GetValue(name, keys[i])
				return keys[i], fmt.Sprintf("%s = %s", keys[i], val)
			})
		case "global":
			listGlobalFlags()
			return fs.ConfigGoto("menu")
		case "backend":
			listBackendFlags()
			return fs.ConfigGoto("menu")
		case "quit", "q", "exit":
			return nil, nil
		default:
			return fs.ConfigGoto("menu")
		}
	case "do_another":
		if configIn.Result == "true" {
			return fs.ConfigInput("key_name", "option name", "Config name for the option")
		}
		return nil, nil
	case "key_name":
		convertedName, err := validateKeyName(configIn.Result)
		if err != nil {
			return fs.ConfigError("try_again", fmt.Sprintf("Invalid characters detected. Try %s instead of %s.", convertedName, configIn.Result))
		}
		return fs.ConfigInputOptional("val_"+configIn.Result, configIn.Result, "Value for option "+configIn.Result)
	case "parents_names":
		err := SetParents(name, strings.Split(configIn.Result, ","))
		if err != nil {
			return fs.ConfigError("", err.Error())
		}
		return fs.ConfigGoto("menu")
	case "try_again":
		return fs.ConfigResult("do_another", "true")
	case "default_profile_name":
		err := SetDefaultProfile(configIn.Result)
		if err != nil {
			return fs.ConfigError("", err.Error())
		}
		return fs.ConfigGoto("menu")
	case "delete_key":
		if configIn.Result != "cancel" {
			DeleteProfileKey(name, configIn.Result)
		}
		return fs.ConfigGoto("menu")
	default:
		if strings.HasPrefix(configIn.State, "val_") {
			key := strings.TrimPrefix(configIn.State, "val_")
			val := configIn.Result
			m.Set(key, val)
		}
		return fs.ConfigConfirm("do_another", false, "ask_do_another", "Would you like to add/edit another option?")
	}
}

func nope(ctx context.Context, name string, root string, config configmap.Mapper) (fs.Fs, error) {
	return nil, errors.New("profiles are not real filesystems! To use a profile, add the --use-profile flag")
}

// ShowProfiles shows all profiles in the config file
func ShowProfiles() {
	profiles := GetProfileList()
	fmt.Printf("%-20s %s\n", "Name", "Type")
	fmt.Printf("%-20s %s\n", "====", "====")
	for _, profile := range profiles {
		fmt.Printf("%-20s %s\n", profile, "profile")
	}
}

// GetProfileList returns only remotes with type = profile
func GetProfileList() []string {
	remotes := config.LoadedData().GetSectionList()
	profiles := []string{}
	if len(remotes) == 0 {
		return profiles
	}
	sort.Strings(remotes)
	for _, remote := range remotes {
		remoteType := config.FileGet(remote, "type")
		if remoteType == "profile" {
			profiles = append(profiles, remote)
		}
	}
	return profiles
}

// EditProfiles edits the config file interactively
// TODO: most of the config.xRemote functions hard-code the word "remote" in the text displayed to the user,
// which could be confusing since "profiles" aren't really "remotes".
// Consider either duplicating those functions here (violating DRY) or editing them to take a variable.
// (Variable would set us up well for other kinds of stored states like bisync sessions https://github.com/rclone/rclone/issues/5678)
func EditProfiles(ctx context.Context) (err error) {
	for {
		haveProfiles := len(GetProfileList()) != 0
		what := []string{"eEdit existing profile", "nNew profile", "dDelete profile", "rRename profile", "cCopy profile", "sSet default profile", "qQuit config"}
		if haveProfiles {
			fmt.Printf("Current profiles:\n\n")
			ShowProfiles()
			fmt.Printf("\n")
		} else {
			fmt.Printf("No profiles found, make a new one?\n")
			// take 2nd item and last 2 items of menu list
			what = append(what[1:2], what[len(what)-2:]...)
		}
		switch i := config.Command(what); i {
		case 'e':
			newSection()
			name := ChooseProfile()
			newSection()
			fs := fs.MustFind("profile")
			err = config.EditRemote(ctx, fs, name)
			if err != nil {
				return err
			}
		case 'n':
			newSection()
			name := config.NewRemoteName()
			newSection()
			err = config.NewRemote(ctx, name)
			if err != nil {
				return err
			}
		case 'd':
			newSection()
			name := ChooseProfile()
			newSection()
			config.DeleteRemote(name)
		case 'r':
			newSection()
			name := ChooseProfile()
			newSection()
			config.RenameRemote(name)
		case 'c':
			newSection()
			name := ChooseProfile()
			newSection()
			config.CopyRemote(name)
		case 's': // 'd' was taken
			newSection()
			fs := fs.MustFind("profile")
			err = config.EditRemote(ctx, fs, "default_profile")
			if err != nil {
				return err
			}
		case 'q':
			return nil
		}
		newSection()
	}
}

// newSection prints an empty line to separate sections
func newSection() {
	fmt.Println()
}

// ChooseProfile chooses a profile name
func ChooseProfile() string {
	profiles := GetProfileList()
	sort.Strings(profiles)
	fmt.Println("Select profile.")
	return config.Choose("profile", "value", profiles, nil, "", true, false)
}

func listGlobalFlags() {
	all := flags.All.AllRegistered()
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, fmt.Sprintf("%-40s%-15s%-35s", optionToProfileKey(k.Name), k.Value.Type(), k.Usage))
	}
	sort.Strings(keys)
	for _, s := range keys {
		fmt.Println(s)
	}
}

func listBackendFlags() {
	keys := make([]string, 0, 10)
	list := func(f *pflag.Flag) {
		keys = append(keys, fmt.Sprintf("%-40s%-15s%-35s", optionToProfileKey(f.Name), f.Value.Type(), f.Usage))
	}
	pflag.CommandLine.VisitAll(list)
	sort.Strings(keys)
	for _, s := range keys {
		fmt.Println(s)
	}
}