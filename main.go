package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"regexp"

	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/nufyoot/datadog-sync/dd"
	"github.com/nufyoot/datadog-sync/util"
)

var (
	modeStr   = flag.String("mode", "pull", "Mode: push or pull")
	formatStr = flag.String("format", "yaml", "File format: yaml or json")
	filter    = flag.String("only", "", "Regex to restrict synced monitors to those whose name match")
	dryRun    = flag.Bool("dry-run", false, "Dry run: output which changes would be performed")
	verbose   = flag.Bool("verbose", false, "Verbose: print complete monitors during synchronization")
	withIds   = flag.Bool("ids", false, "Include monitor IDs in dump")
)

type mode int

const (
	pull mode = iota
	push
)

func filteredMonitors(from []dd.Monitor, filter string) ([]dd.Monitor, error) {
	if filter == "" {
		return from, nil
	}
	reg, err := regexp.Compile(filter)
	if err != nil {
		return nil, err
	}
	var res []dd.Monitor
	for _, m := range from {
		if reg.MatchString(m.Name) {
			res = append(res, m)
		}
	}
	return res, nil
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)

	var action mode
	var format util.Format

	flag.Parse()

	switch *modeStr {
	case "pull":
		action = pull
	case "push":
		action = push
	default:
		logrus.Fatalf("unsupported mode %v", *modeStr)
	}

	switch *formatStr {
	case "json":
		format = util.JSON
	case "yaml":
		format = util.YAML
	default:
		logrus.Fatalf("unsupported format %v", *formatStr)
	}

	if *dd.APIKey == "" {
		var ok bool
		if *dd.APIKey, ok = os.LookupEnv("DATADOG_API_KEY"); !ok {
			logrus.Fatal("no API key provided")
		}
	}

	if *dd.AppKey == "" {
		var ok bool
		if *dd.AppKey, ok = os.LookupEnv("DATADOG_APP_KEY"); !ok {
			logrus.Fatal("no application key provided")
		}
	}

	client := &http.Client{}

	remote, err := dd.GetMonitors(client)
	if err != nil {
		logrus.WithError(err).Fatal("could not pull monitors")
	}

	remote, err = filteredMonitors(remote, *filter)
	if err != nil {
		logrus.WithError(err).Fatal("could not filter remote monitors")
	}

	switch action {
	case pull:
		var repr string
		if !*withIds {
			for i := range remote {
				remote[i].ID = nil
			}
		}
		repr, err = util.Marshal(remote, format)
		if err != nil {
			logrus.WithError(err).Fatal("could not serialize monitors")
		}
		fmt.Println(repr)
	case push:
		var local []dd.Monitor
		repr, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			logrus.WithError(err).Fatal("could not read from standard input")
		}

		if err = util.Unmarshal(repr, &local, format); err != nil {
			logrus.WithError(err).Fatal("could not deserialize monitors")
		}

		local, err = filteredMonitors(local, *filter)
		if err != nil {
			logrus.WithError(err).Fatal("could not filter local monitors")
		}

		if err = dd.SyncMonitors(local, remote, client, *dryRun, *verbose); err != nil {
			logrus.WithError(err).Fatal("could not sync monitors: %v")
		}
	}
}
