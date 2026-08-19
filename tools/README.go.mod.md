# tools/go.mod

This file declares `tools/` as its own Go module. It has three lines of substance.

## Module name

`github.com/ashwalk33r/minecraft-fabric-commandsspy/tools`

The import path for this module. It follows the repo's GitHub URL plus the `tools/` subdirectory. Go uses it to identify the module; nothing needs to be published for local builds to work.

## Go version

`go 1.23`

The minimum Go toolchain allowed to build this module. It is pinned to 1.23 deliberately: Go 1.22.4 produces broken macOS (darwin) binaries that are missing the `LC_UUID` Mach-O load command. Do not lower this.

## Dependencies

None. The module uses only the Go standard library, so there is no `require` block and no `go.sum` file.

## Why tools/ has its own module

The repository is a Java (Fabric/Gradle) Minecraft mod. The Go code in `tools/` is helper tooling, not part of the mod. A separate module keeps the Go toolchain scoped to this directory: `go build` and `go test` work here without pretending the whole repo is a Go project, and Go dependencies (if any ever appear) stay isolated from the Java build.
