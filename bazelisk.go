// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main is the entry point for Bazelisk.
package main

import (
	"log"
	"os"

	"github.com/bazelbuild/bazelisk/core"
	"github.com/bazelbuild/bazelisk/repositories"
)

func main() {
	gcs := &repositories.GCSRepo{}
	config := core.MakeDefaultConfig()
	gitHub := repositories.CreateGitHubRepo(config.Get("BAZELISK_GITHUB_TOKEN"))
	// Fetch LTS releases & candidates, rolling releases and Bazel-at-commits from GCS, forks from GitHub.
	repos := core.CreateRepositories(gcs, gitHub, gcs, gcs, true)

	exitCode, err := core.RunBazeliskWithArgsFuncAndConfig(func(string) []string { return os.Args[1:] }, repos, config)
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(exitCode)
}
