module github.com/ashwalk33r/minecraft-fabric-commandsspy/tools

// >= 1.23 is required: Go 1.22.4 emits broken darwin binaries (missing LC_UUID).
// Full patch version: a bare "go 1.23" makes older host toolchains try to
// download the nonexistent "go1.23" artifact ("toolchain not available").
go 1.23.0
