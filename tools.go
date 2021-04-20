// +build tools

package tools

// CLI tools used for the dev environment.
// --
//
// If you'd like to add a cli tool, simply add its import path below
// here, then add it to go.mod via `go get -u <pkg>`.
//
// To use a CLI package from below, instead of compiling the bin
// and using it locally (which you can do with GOBIN=$PWD/bin; go install <pkg-import>),
// it's easier to manage by just running the cli via `go run`.
//
// For instance, if you want to use the `rerun` CLI, previously you'd
// use it from your global system with `rerun <args>`. Now, you should run it
// via: `go run github.com/VojtechVitek/rerun/cmd/rerun <args>`.
//
// This is considered best practice in the Go space. For more info on this
// technique see https://gist.github.com/tschaub/66f5feb20ae1b5166e9fe928c5cba5e4

import (
	_ "github.com/goware/modvendor"
)
