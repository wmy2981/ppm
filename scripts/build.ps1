$version = (Get-Content "$PSScriptRoot\..\VERSION" -Raw).Trim()
go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$PSScriptRoot\..\ppm.exe" ./cmd/ppm
