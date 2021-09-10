package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/buildkite/go-buildkite/v2/buildkite"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	apiToken  = kingpin.Flag("token", "API token").Required().OverrideDefaultFromEnvar("BUILDKITE_API_TOKEN").String()
	buildSlug = kingpin.Flag("slug", "Build slug(organization-slug/pipeline-slug/build-number)").Required().String()
	debug     = kingpin.Flag("debug", "Enable debugging").Bool()
	jobCount  = kingpin.Flag("count", "Number of jobs").Default("120").Int()
)

func main() {
	kingpin.Parse()

	if len(strings.Split(*buildSlug, "/")) != 3 {
		log.Fatalf("please specify build slug format as organization-slug/pipeline-slug/build-number")
	}

	var build *buildkite.Build
	var err error

	config, err := buildkite.NewTokenConfig(*apiToken, *debug)
	if err != nil {
		log.Fatalf("client config failed: %s", err)
	}
	buildkiteClient := buildkite.NewClient(config.Client())

	if build, err = fetchBuildByID(buildkiteClient); err != nil {
		log.Fatalf("graphql failed: %s", err)
	}

	jobArgs := strings.Split(*buildSlug, "/")
	logger := log.New(os.Stdout, "", 0)
	var wg sync.WaitGroup

	for _, job := range build.Jobs {
		wg.Add(1)

		go func(id string) {
			defer wg.Done()
			if len(id) != 0 {
				jobLog, _, err := buildkiteClient.Jobs.GetJobLog(jobArgs[0], jobArgs[1], *build.ID, id)
				if err != nil {
					errmsg := fmt.Sprintf("Error: %v, ID: %v, uuid: %v\n", err, build.ID, id)
					logger.Print(errmsg)
				} else {
					logger.Print(*jobLog.Content)
				}
			}
		}(*job.ID)
	}
	wg.Wait()
}

func fetchBuildByID(client *buildkite.Client) (*buildkite.Build, error) {
	slugs := strings.Split(*buildSlug, "/")
	build, _, err := client.Builds.Get(slugs[0], slugs[1], slugs[2], nil)
	if err != nil {
		return nil, err
	}

	return build, nil
}
