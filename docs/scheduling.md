# Scheduled Runs

Use the unified `bilge` command to schedule scans or cleanup during low-usage hours.

## cron (Linux)

Weekly at 3:00 AM every Sunday:

```cron
0 3 * * 0 /absolute/path/to/bilge clean --all --yes >> "$HOME/bilge-cleanup.log" 2>&1
```

Replace `/absolute/path/to/bilge` with the path returned by `command -v bilge`. The `--yes` flag skips the dev-cache confirmation prompt. Keep log redirection enabled for troubleshooting.

On macOS, prefer `launchd`; cron is supported but Apple recommends `launchd` for timed jobs.

## launchd (macOS)

Find the installed executable with `command -v bilge`, then create `~/Library/LaunchAgents/com.bilge.weekly.plist`. Replace `/absolute/path/to/bilge` and `YOUR_USER` with the values for your machine:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.bilge.weekly</string>

  <key>ProgramArguments</key>
  <array>
    <string>/absolute/path/to/bilge</string>
    <string>--clean</string>
    <string>--all</string>
    <string>--yes</string>
  </array>

  <key>StartCalendarInterval</key>
  <dict>
    <key>Weekday</key>
    <integer>0</integer>
    <key>Hour</key>
    <integer>3</integer>
    <key>Minute</key>
    <integer>0</integer>
  </dict>

  <key>StandardOutPath</key>
  <string>/Users/YOUR_USER/Library/Logs/bilge-cleanup.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/YOUR_USER/Library/Logs/bilge-cleanup.log</string>
</dict>
</plist>
```

Load and start:

```bash
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.bilge.weekly.plist
launchctl kickstart "gui/$(id -u)/com.bilge.weekly"
```

Unload:

```bash
launchctl bootout "gui/$(id -u)" ~/Library/LaunchAgents/com.bilge.weekly.plist
