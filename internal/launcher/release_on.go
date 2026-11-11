//go:build archive || musl

package launcher

// release is true in a release build: gd build sets the archive tag (macOS, Windows) or musl (Linux)
// and embeds the engine and the scene.
const release = true

// environment is deployment.environment.name in logs.
const environment = "production"
