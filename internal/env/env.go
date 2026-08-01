package env

import "os"

func Verbose() bool {
	return os.Getenv("PKGFORT_VERBOSE") == "1"
}

func SkipRequested() bool {
	return os.Getenv("PKGFORT_SKIP") == "1"
}
