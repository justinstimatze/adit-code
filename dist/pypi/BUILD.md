# Building PyPI wheels

The PyPI package is a thin wrapper that `os.execvp`s the Go binary.
Platform-tagged wheels contain the pre-built binary for each OS/arch.

## Option 1: go-to-wheel (recommended)

```bash
# Install go-to-wheel
pip install go-to-wheel

# Build wheels for all platforms
go-to-wheel . --name adit-code --version 0.1.0 --output dist/pypi/dist/
```

## Option 2: Manual wheel build

1. Cross-compile Go binaries:
```bash
GOOS=linux   GOARCH=amd64 go build -o dist/pypi/adit_code/bin/linux_amd64/adit   ./cmd/adit
GOOS=linux   GOARCH=arm64 go build -o dist/pypi/adit_code/bin/linux_arm64/adit   ./cmd/adit
GOOS=darwin  GOARCH=amd64 go build -o dist/pypi/adit_code/bin/darwin_amd64/adit  ./cmd/adit
GOOS=darwin  GOARCH=arm64 go build -o dist/pypi/adit_code/bin/darwin_arm64/adit  ./cmd/adit
GOOS=windows GOARCH=amd64 go build -o dist/pypi/adit_code/bin/windows_amd64/adit.exe ./cmd/adit
```

2. Build wheels:
```bash
cd dist/pypi
pip install build
python -m build
```

3. Upload (when ready):
```bash
pip install twine
twine upload dist/pypi/dist/*
```
