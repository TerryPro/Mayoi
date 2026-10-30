cask "mayoi" do
  arch arm: "arm64", intel: "amd64"

  version "0.15.0"
  sha256 arm:   "66ef26e70e033531a7f0edc2b6966d3d11fb5e3c047f1dde86a33c0448ca3c07",
         intel: "0ed9566a588034bfbe8c36d3e1ea72792f53518147307fd6ddc3ee53c319e5de"

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
