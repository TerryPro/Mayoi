cask "mayoi" do
  arch arm: "arm64", intel: "amd64"

  version "0.14.0"
  sha256 arm:   "71ac20825b19c19a63fb7b52c04ac9697622a4a6bc0293a2f3cedcc930893799",
         intel: "6fb303af880f9951b7241a785b977fbba2495809a0a7e40c4fa6d9f106e682f4"

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
