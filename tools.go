// +build tools

package talisman

// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/kisielk/errcheck" // tools dependency
	_ "golang.org/x/lint/golint"    // tools dependency
)
