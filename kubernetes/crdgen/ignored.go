package main

import "github.com/Clever/go-utils/stringset"

var ignoredFields = map[string]stringset.StringSet{
	"UserSpecV2": stringset.New("LocalAuth", "Expires", "CreatedBy", "Status"),
}
