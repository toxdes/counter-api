# Build Instructions

## Development Build

```bash
go build -o counter-api
./counter-api --version
# Output: Counter API v dev
```

## Production Build

To include version information in the binary:

```bash
VERSION=$(cat version.txt)
go build -ldflags="-X 'main.Version=${VERSION}'" -o counter-api
./counter-api --version
# Output: Counter API v1.0.3
```

## Docker Build

```dockerfile
COPY version.txt .
RUN VERSION=$(cat version.txt) && \
    go build -ldflags="-X 'main.Version=${VERSION}'" -o counter-api
```
