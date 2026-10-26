package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func runInstall() {
	bin, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: find binary: %v\n", err)
		os.Exit(1)
	}
	bin, _ = filepath.EvalSymlinks(bin)

	// Install binary to ~/.local/bin
	localBin := os.Getenv("HOME") + "/.local/bin"
	if err := os.MkdirAll(localBin, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "install: mkdir %s: %v\n", localBin, err)
		os.Exit(1)
	}
	dst := localBin + "/watcher"
	if err := copyFile(bin, dst); err != nil {
		fmt.Fprintf(os.Stderr, "install: copy %s: %v\n", dst, err)
		os.Exit(1)
	}
	fmt.Println("installed", dst)

	// Create state and runtime directories
	os.MkdirAll(defaultRuntimeDir(), 0755)
	os.MkdirAll(defaultStateDir(), 0755)

	// Install service definition
	switch runtime.GOOS {
	case "linux":
		installSystemd(dst)
	case "darwin":
		installLaunchd(dst)
	default:
		fmt.Fprintf(os.Stderr, "install: unsupported platform %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func installSystemd(bin string) {
	unit := fmt.Sprintf(`[Unit]
Description=agent-status — AI session state watcher
Documentation=https://github.com/jvrsantacruz/agent-status
After=network.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5
Environment=AGENT_STATUS_STATE_DIR=%s
Environment=AGENT_STATUS_RUNTIME_DIR=%s

[Install]
WantedBy=default.target
`, bin, defaultStateDir(), defaultRuntimeDir())

	usrDir := os.Getenv("XDG_CONFIG_HOME")
	if usrDir == "" {
		usrDir = os.Getenv("HOME") + "/.config"
	}
	unitDir := usrDir + "/systemd/user"
	os.MkdirAll(unitDir, 0755)

	unitPath := unitDir + "/agent-status.service"
	os.WriteFile(unitPath, []byte(unit), 0644)
	fmt.Println("wrote", unitPath)

	exec.Command("systemctl", "--user", "daemon-reload").Run()
	exec.Command("systemctl", "--user", "enable", "agent-status").Run()
	exec.Command("systemctl", "--user", "start", "agent-status").Run()
	fmt.Println("agent-status: started (systemd --user)")
}

func installLaunchd(bin string) {
	home := os.Getenv("HOME")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.jvr.agent-status</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>AGENT_STATUS_STATE_DIR</key>
		<string>%s</string>
		<key>AGENT_STATUS_RUNTIME_DIR</key>
		<string>%s</string>
	</dict>
	<key>KeepAlive</key>
	<true/>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s/Library/Logs/agent-status.log</string>
	<key>StandardErrorPath</key>
	<string>%s/Library/Logs/agent-status.log</string>
</dict>
</plist>
`, bin, defaultStateDir(), defaultRuntimeDir(), home, home)

	launchDir := home + "/Library/LaunchAgents"
	os.MkdirAll(launchDir, 0755)

	plistPath := launchDir + "/com.jvr.agent-status.plist"
	os.WriteFile(plistPath, []byte(plist), 0644)
	fmt.Println("wrote", plistPath)

	exec.Command("launchctl", "load", plistPath).Run()
	fmt.Println("agent-status: started (launchd)")
}
