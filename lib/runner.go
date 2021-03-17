package lib

import (
	"fmt"
	"runtime"
)

// PrintVersion prints the specified app version to STDOUT
func PrintVersion(appName string, version string, gitref string) {
	if gitref != "" {
		fmt.Printf("%v v%v git:%v %v\n", appName, version, gitref, runtime.Version())
	} else {
		fmt.Printf("%v v%v %v\n", appName, version, runtime.Version())
	}
}
