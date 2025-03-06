// Copyright 2021 Ingvar Mattsson
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/vatine/promgraph/pkg/rulegraph"
)

// Return the files, with glob expansions if needed
func files(in []string) []string {
	var rv []string

	for _, p := range in {
		match, err := filepath.Glob(p)
		if err != nil {
			log.WithFields(log.Fields{
				"error":   err,
				"pattern": p,
			}).Error("Failed to expand glob.")
			continue
		}
		rv = append(rv, match...)
	}

	return rv
}

// Return a sink for the graph, and a bool indicating if it needs to
// be closed.
func output(designator string) io.Writer {
	if designator == "-" {
		return os.Stdout
	}

	f, err := os.Open(designator)
	if err != nil {
		log.WithFields(log.Fields{
			"error":    err,
			"filename": designator,
		}).Fatal("Failed to open output file.")
	}

	return f
}

func main() {
	out := flag.String("output", "-", "Output file (use '-' for stdout).")
	flag.Parse()

	filenames := files(flag.Args())
	sink := output(*out)

	if closer, ok := sink.(io.WriteCloser); ok {
		defer closer.Close()
	}

	rules, err := rulegraph.LoadRuleFiles(filenames...)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to parse rules, aborting.")
	}
	graph := rulegraph.BuildRuleDiagram(rules)
	rulegraph.EmitGraph(graph, sink)
}
