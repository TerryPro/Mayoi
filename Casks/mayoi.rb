cask "mayoi" do
  arch arm: "arm64", intel: "amd64"

  version "0.14.1"
  sha256 arm:   "54709897cc5fa5a4db9fe1b7bc32c0de6ff4009635eaf0a23e766cf87fb414cf",
         intel: "c3eff992d8cfc2110860178db89f2992ab5e078a74617bfd3bf1992cb54d4caa"

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
