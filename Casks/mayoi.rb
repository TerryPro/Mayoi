cask "mayoi" do
  arch arm: "arm64", intel: "amd64"

  version "0.14.2"
  sha256 arm:   "6071819f905de0b8d9e0e0fd7b85bca2d71a07e231f39b1ad1c516a0adf62713",
         intel: "4467ead07f6f80657a5e782782205942697bf73cd0bef0515ddf43e23d67b8f4"

  url "https://github.com/sorrycc/Mayoi/releases/download/v#{version}/mayoi-#{version}-#{arch}.dmg"
  name "Mayoi"
  desc "Small, fast code editor"
  homepage "https://github.com/sorrycc/Mayoi"

  auto_updates true
  depends_on macos: :ventura

  app "Mayoi.app"
  command_wrapper "mayoi", content: <<~SH
    #!/bin/sh
    # Managed by Homebrew; use brew uninstall --cask mayoi to remove it.
    app=#{(appdir/"Mayoi.app/Contents/MacOS/Mayoi").to_s.shellescape}
    [ -x "$app" ] || { echo "mayoi: Mayoi.app is not found" >&2; exit 1; }
    MAYOI_CLI=1 exec "$app" "$@"
  SH
end
