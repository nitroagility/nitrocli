<p align="center">
  <img src="images/nitrocli.png" alt="NitroCLI" width="200">
</p>

<h1 align="center">Nitro<i>CLI</i></h1>

<p align="center">
  <strong>The official command line interface for <a href="https://www.nitroagility.com" target="_blank">NitroAgility</a></strong>
</p>

<p align="center">
  <a href="https://github.com/nitroagility/nitrocli/releases"><img src="https://img.shields.io/github/v/release/nitroagility/nitrocli?style=flat-square&color=00c8ff" alt="Release"></a>
  <a href="https://github.com/nitroagility/nitrocli/blob/main/LICENSE"><img src="https://img.shields.io/github/license/nitroagility/nitrocli?style=flat-square&color=00c8ff" alt="License"></a>
  <a href="https://github.com/nitroagility/nitrocli"><img src="https://img.shields.io/github/stars/nitroagility/nitrocli?style=flat-square&color=00c8ff" alt="Stars"></a>
</p>

```sh
$ nitro
Fast. Minimal. Powerful.
Manage your software and operations from the terminal.
```

---

## Installation

### Homebrew (macOS & Linux)

```bash
brew install nitroagility/tap/nitro
```

### Debian / Ubuntu

```bash
VERSION="v0.0.0"
ARCH=$( [ "$(uname -m)" = "aarch64" ] && echo "arm64" || echo "x86_64" )
curl -fsSL -o nitrocli.deb "https://github.com/nitroagility/nitrocli/releases/download/${VERSION}/nitrocli_Linux_${ARCH}.deb"
sudo dpkg -i nitrocli.deb
```

### Fedora / RHEL / CentOS

```bash
VERSION="v0.0.0"
ARCH=$( [ "$(uname -m)" = "aarch64" ] && echo "arm64" || echo "x86_64" )
curl -fsSL -o nitrocli.rpm "https://github.com/nitroagility/nitrocli/releases/download/${VERSION}/nitrocli_Linux_${ARCH}.rpm"
sudo rpm -i nitrocli.rpm
```

### Alpine Linux

```bash
VERSION="v0.0.0"
ARCH=$( [ "$(uname -m)" = "aarch64" ] && echo "arm64" || echo "x86_64" )
curl -fsSL -o nitrocli.apk "https://github.com/nitroagility/nitrocli/releases/download/${VERSION}/nitrocli_Linux_${ARCH}.apk"
sudo apk add --allow-untrusted nitrocli.apk
```

### Binary (macOS & Linux)

```bash
VERSION="v0.0.0"
OS=$(uname -s)
ARCH=$( [ "$(uname -m)" = "aarch64" ] || [ "$(uname -m)" = "arm64" ] && echo "arm64" || echo "x86_64" )
curl -fsSL -o nitrocli.tar.gz "https://github.com/nitroagility/nitrocli/releases/download/${VERSION}/nitrocli_${OS}_${ARCH}.tar.gz"
tar -xzf nitrocli.tar.gz
sudo mv nitro /usr/local/bin/
```

### Windows (PowerShell)

```powershell
$VERSION = "v0.0.0"
$ARCH = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "x86_64" }
Invoke-WebRequest -Uri "https://github.com/nitroagility/nitrocli/releases/download/$VERSION/nitrocli_Windows_$ARCH.zip" -OutFile nitrocli.zip
Expand-Archive nitrocli.zip -DestinationPath "$env:LOCALAPPDATA\nitrocli"
$env:PATH += ";$env:LOCALAPPDATA\nitrocli"
[Environment]::SetEnvironmentVariable("PATH", $env:PATH, "User")
```

### Verify

```bash
nitro version
```

---

## License

NitroCLI is licensed under the [Apache License 2.0](LICENSE).

Copyright (c) [NitroAgility](https://www.nitroagility.com).
