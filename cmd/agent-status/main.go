// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

// Command agent-status shows in the tmux window list what each AI coding
// agent is doing: working, waiting for you, or failed.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

var clock = time.Now

// version is set at release time with -ldflags "-X main.version=v1.2.3".
// Without it, the version Go stamps from the git tag is used.
var version = ""

const usage = `usage: agent-status <command> [arguments]

commands:
  daemon                          read events, keep state, write state.json
  emit [flags] <state> [message]  send one event: working, question, idle, error, end;
                                  --claude-hook inside a Claude Code hook
  render <pane>                   print the tmux marker for a pane id, such as %3
  doctor                          check the installation end to end
  version                         print version, commit and platform

'agent-status emit -h' lists the emit flags. Documentation:
https://github.com/jvrsantacruz/agent-status
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "daemon":
		return runDaemon()
	case "emit":
		return runEmit(args[1:], stdin, stderr)
	case "render":
		return runRender(args[1:], stdout)
	case "doctor":
		return runDoctor(stdout)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, versionLine())
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "agent-status: unknown command %q\n\n%s", args[0], usage)
	return 2
}

// buildVersion returns the release version, or what Go stamped from git.
func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "devel"
}

// versionLine is what a bug report needs: version, commit, commit date, toolchain, platform.
func versionLine() string {
	// go install builds carry no VCS data; their pseudo-version already
	// holds the commit and its date.
	rev, at := "", ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value[:min(len(s.Value), 12)]
			case "vcs.time":
				at = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					rev += "+dirty"
				}
			}
		}
	}
	build := ""
	if rev != "" {
		build = fmt.Sprintf(" (%s, %s)", rev, at)
	}
	return fmt.Sprintf("agent-status %s%s %s %s/%s",
		buildVersion(), build, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
