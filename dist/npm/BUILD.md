# Building npm packages

Follows the esbuild pattern: one main package with optionalDependencies
pointing to scoped platform packages. npm only installs the matching one.

## Step 1: Cross-compile Go binaries

```bash
mkdir -p dist/npm/platforms/{darwin-arm64,darwin-x64,linux-arm64,linux-x64,win32-x64}

GOOS=darwin  GOARCH=arm64 go build -o dist/npm/platforms/darwin-arm64/adit   ./cmd/adit
GOOS=darwin  GOARCH=amd64 go build -o dist/npm/platforms/darwin-x64/adit     ./cmd/adit
GOOS=linux   GOARCH=arm64 go build -o dist/npm/platforms/linux-arm64/adit    ./cmd/adit
GOOS=linux   GOARCH=amd64 go build -o dist/npm/platforms/linux-x64/adit      ./cmd/adit
GOOS=windows GOARCH=amd64 go build -o dist/npm/platforms/win32-x64/adit.exe  ./cmd/adit
```

## Step 2: Create platform packages

For each platform directory, create a package.json from the template:

```bash
for plat in darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-x64; do
  os_name=$(echo $plat | cut -d- -f1)
  arch=$(echo $plat | cut -d- -f2)
  cat > dist/npm/platforms/$plat/package.json << EOF
{
  "name": "@adit-code/$plat",
  "version": "0.1.0",
  "description": "adit-code binary for $plat",
  "license": "MIT",
  "os": ["$os_name"],
  "cpu": ["$arch"]
}
EOF
done
```

## Step 3: Publish (when ready)

```bash
# Publish platform packages first
for plat in darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-x64; do
  cd dist/npm/platforms/$plat
  npm publish --access public
  cd ../../../..
done

# Then publish the main package
cd dist/npm
npm publish --access public
```

## How it works

1. User runs `npm install adit-code`
2. npm resolves optionalDependencies, installs only the matching platform package
3. `postinstall` script (`install.js`) copies the binary from the platform package to `bin/adit`
4. `npx adit` or project scripts can now run the binary
