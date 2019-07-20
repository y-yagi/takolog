package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/buildkite/go-buildkite/buildkite"
	"github.com/machinebox/graphql"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	apiToken  = kingpin.Flag("token", "API token").Required().String()
	buildSlug = kingpin.Flag("slug", "Build Slug").Required().String()
	debug     = kingpin.Flag("debug", "Enable debugging").Bool()
	jobCount  = kingpin.Flag("count", "Number of jobs").Default("30").Int()
)

type responseType struct {
	Build struct {
		Jobs struct {
			Edges []struct {
				Node struct {
					Uuid string
				}
			}
		}
	}
}

func main() {
	kingpin.Parse()

	graphqlClient := graphql.NewClient("https://graphql.buildkite.com/v1")

	req := graphql.NewRequest(`
		query ($slug: ID, $jobCount: Int) {
      build(slug: $slug) {
        jobs(first: $jobCount) {
          edges {
            node {
              ... on JobTypeCommand {
                uuid
              }
            }
          }
        }
      }
    }
	`)

	req.Var("slug", *buildSlug)
	req.Var("jobCount", *jobCount)
	auth := fmt.Sprintf("Bearer %s", *apiToken)
	req.Header.Set("Authorization", auth)
	ctx := context.Background()

	var responseData responseType
	if err := graphqlClient.Run(ctx, req, &responseData); err != nil {
		log.Fatalf("graphql failed: %s", err)
	}

	config, err := buildkite.NewTokenConfig(*apiToken, *debug)
	if err != nil {
		log.Fatalf("client config failed: %s", err)
	}

	buildkiteClient := buildkite.NewClient(config.Client())
	jobArgs := strings.Split(*buildSlug, "/")
	logger := log.New(os.Stdout, "", 0)
	var wg sync.WaitGroup

	for _, edge := range responseData.Build.Jobs.Edges {
		wg.Add(1)

		go func(uuid string) {
			defer wg.Done()
			jobLog, _, err := buildkiteClient.Jobs.GetJobLog(jobArgs[0], jobArgs[1], jobArgs[2], uuid)
			if err != nil {
				errmsg := fmt.Sprintf("Error: %v\n", err)
				logger.Print(errmsg)
			} else {
				logger.Print(*jobLog.Content)
			}
		}(edge.Node.Uuid)
	}
	wg.Wait()
}
