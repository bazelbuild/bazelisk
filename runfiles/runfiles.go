// Package runfiles offers functionality to read data dependencies of tests.
package runfiles

import (
	"io/ioutil"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

func ReadFile(name string) ([]byte, error) {
	path, err := bazel.Runfile(name)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}
