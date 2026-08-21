//go:build darwin

package notify

import "context"

const notificationAppleScript = `on run argv
	set theTitle to item 1 of argv
	set theBody to item 2 of argv
	set theSound to item 3 of argv
	if theSound is "" then
		display notification theBody with title theTitle
	else
		display notification theBody with title theTitle sound name theSound
	end if
end run`

// AppleScriptNotifier delivers notifications through osascript.
type AppleScriptNotifier struct{}

// NewAppleScriptNotifier creates an osascript-backed notifier.
func NewAppleScriptNotifier() *AppleScriptNotifier {
	return &AppleScriptNotifier{}
}

// Notify invokes a fixed AppleScript with notification values as argv entries.
func (*AppleScriptNotifier) Notify(ctx context.Context, n Notification) error {
	soundName := n.SoundName
	if SuppressSoundForLevel(PriorityToLevel(n.Priority)) {
		soundName = ""
	}
	return runCommand(ctx, "osascript", "-e", notificationAppleScript, "--", n.Title, n.Body, soundName)
}

// Caps returns osascript's supported notification fields.
func (*AppleScriptNotifier) Caps() Caps {
	return Caps{Sound: true}
}
