package bisync

import "github.com/rclone/rclone/fs"

// PushNotifyFlags describes the PushNotify options in force
type PushNotifyFlags = fs.Bits[pushNotifyChoices]

// PushNotifyFlags definitions
const (
	NotifyStart PushNotifyFlags = 1 << iota
	NotifyEnd
	NotifyError
)

type pushNotifyChoices struct{}

func (pushNotifyChoices) Choices() []fs.BitsChoicesInfo {
	return []fs.BitsChoicesInfo{
		// {Bit: uint64(0), Name: "OFF"}, // ""
		{Bit: uint64(NotifyStart), Name: "start"},
		{Bit: uint64(NotifyEnd), Name: "end"},
		{Bit: uint64(NotifyError), Name: "error"},
	}
}

func (pushNotifyChoices) Type() string {
	return "PushNotifyFlags"
}

// PushNotifyFlagsList is a list of possible values, for use in help
var PushNotifyFlagsList = NotifyStart.Help()
