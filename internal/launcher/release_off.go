//go:build !archive && !musl

package launcher

// release is false in a development build (go run, go build): LibGodot and graphics/ lie in the project.
const release = false

// environment is deployment.environment.name in logs.
const environment = "development"
