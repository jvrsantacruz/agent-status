class AgentStatus < Formula
  desc "Track AI agent sessions (pi, Claude) in the tmux window list"
  homepage "https://github.com/jvrsantacruz/agent-status"
  license "MIT"
  version "0.1.0"

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/jvrsantacruz/agent-status/releases/download/v0.1.0/agent-status-v0.1.0-darwin-arm64.tar.gz"
      sha256 "CHANGE_ME_FIRST_RELEASE"
    else
      url "https://github.com/jvrsantacruz/agent-status/releases/download/v0.1.0/agent-status-v0.1.0-darwin-amd64.tar.gz"
      sha256 "CHANGE_ME_FIRST_RELEASE"
    end
  end

  def install
    bin.install "watcher"
  end

  service do
    run [opt_bin/"watcher"]
    keep_alive true
    run_at_load true
    log_path var/"log/agent-status.log"
    error_log_path var/"log/agent-status.log"
    environment_variables({
      AGENT_STATUS_STATE_DIR:    "#{Dir.home}/.local/state/agent-status",
      AGENT_STATUS_RUNTIME_DIR:  "#{Dir.home}/.local/state/agent-status",
    })
  end

  def caveats
    <<~EOS
      The watcher daemon runs in the background via launchd (brew services).
      To start it now and enable at login:

        brew services start agent-status

      The tmux plugin is a separate install. Add to ~/.tmux.conf:

        set -g @plugin 'jvrsantacruz/agent-status-tmux'

      State and logs:
        #{var}/log/agent-status.log
        #{Dir.home}/.local/state/agent-status/state.json
    EOS
  end

  test do
    assert_match "usage", shell_output("#{bin}/watcher emit 2>&1", 1)
    assert_match "usage", shell_output("#{bin}/watcher render 2>&1")
    assert_match "version", shell_output("#{bin}/watcher --version 2>&1", 1)
  end
end
