// Resolves the correct platform-specific binary package.
// Follows the esbuild pattern: the main package declares
// optionalDependencies for each platform, npm installs only
// the matching one, and this script links it.

const os = require("os");
const path = require("path");
const fs = require("fs");

const PLATFORM_MAP = {
  darwin: {
    arm64: "@adit-code/darwin-arm64",
    x64: "@adit-code/darwin-x64",
  },
  linux: {
    arm64: "@adit-code/linux-arm64",
    x64: "@adit-code/linux-x64",
  },
  win32: {
    x64: "@adit-code/win32-x64",
  },
};

function main() {
  const platform = os.platform();
  const arch = os.arch();

  const platformPackages = PLATFORM_MAP[platform];
  if (!platformPackages) {
    console.error(`adit-code: unsupported platform: ${platform}`);
    process.exit(1);
  }

  const packageName = platformPackages[arch];
  if (!packageName) {
    console.error(`adit-code: unsupported architecture: ${platform}/${arch}`);
    process.exit(1);
  }

  // The platform package contains the binary at its root
  try {
    const packageDir = path.dirname(require.resolve(`${packageName}/package.json`));
    const ext = platform === "win32" ? ".exe" : "";
    const src = path.join(packageDir, `adit${ext}`);
    const dest = path.join(__dirname, "bin", `adit${ext}`);

    if (!fs.existsSync(src)) {
      console.error(`adit-code: binary not found in ${packageName}`);
      process.exit(1);
    }

    fs.mkdirSync(path.join(__dirname, "bin"), { recursive: true });
    fs.copyFileSync(src, dest);
    fs.chmodSync(dest, 0o755);
  } catch (e) {
    console.error(`adit-code: could not resolve ${packageName}: ${e.message}`);
    console.error("Install the Go binary directly: go install github.com/justinstimatze/adit-code/cmd/adit@latest");
    process.exit(1);
  }
}

main();
