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
	apiToken  = kingpin.Flag("token", "API token").Required().OverrideDefaultFromEnvar("BUILDKITE_API_TOKEN").String()
	buildSlug = kingpin.Flag("slug", "Build slug(organization-slug/pipeline-slug/build-number)").Required().String()
	debug     = kingpin.Flag("debug", "Enable debugging").Bool()
	jobCount  = kingpin.Flag("count", "Number of jobs").Default("120").Int()
)

type jobsResponse struct {
	Build struct {
		Jobs jobs
	}
}

type pipelineResponse struct {
	Pipeline struct {
		Builds struct {
			Edges []struct {
				build `json:"Node"`
			}
		}
	}
}

type jobs struct {
	Edges []struct {
		Job struct {
			UUID string
		} `json:"Node"`
	}
}

type build struct {
	ID   string
	Jobs jobs
}

const buildQuery = `
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
`

const pipelineQuery = `
  query ($slug: ID!, $jobCount: Int, $state: [BuildStates!]) {
		pipeline(slug: $slug) {
			builds(first: 1, state: $state) {
        edges {
          node {
						id
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
      }
    }
  }
`

func main() {
	kingpin.Parse()

	var build *build
	var err error

	if strings.Count(*buildSlug, "/") > 1 {
		if build, err = fetchBuildByID(buildQuery); err != nil {
			log.Fatalf("graphql failed: %s", err)
		}
		build.ID = strings.Split(*buildSlug, "/")[2]
	} else {
		if build, err = fetchLatestBuild(pipelineQuery); err != nil {
			log.Fatalf("graphql failed: %s", err)
		}
	}

	config, err := buildkite.NewTokenConfig(*apiToken, *debug)
	if err != nil {
		log.Fatalf("client config failed: %s", err)
	}

	buildkiteClient := buildkite.NewClient(config.Client())
	jobArgs := strings.Split(*buildSlug, "/")
	logger := log.New(os.Stdout, "", 0)
	var wg sync.WaitGroup

	for _, edge := range build.Jobs.Edges {
		wg.Add(1)

		go func(uuid string) {
			defer wg.Done()
			jobLog, _, err := buildkiteClient.Jobs.GetJobLog(jobArgs[0], jobArgs[1], build.ID, uuid)
			if err != nil {
				errmsg := fmt.Sprintf("Error: %v\n", err)
				logger.Print(errmsg)
			} else {
				logger.Print(*jobLog.Content)
			}
		}(edge.Job.UUID)
	}
	wg.Wait()
}

func fetchBuildByID(query string) (*build, error) {
	var res jobsResponse
	var build build

	client := buildClient()
	req := buildRequest(query)
	ctx := context.Background()

	if err := client.Run(ctx, req, &res); err != nil {
		return nil, err
	}

	build.Jobs = res.Build.Jobs
	return &build, nil
}

func fetchLatestBuild(query string) (*build, error) {
	var res pipelineResponse

	client := buildClient()
	req := buildRequest(query)
	ctx := context.Background()

	req.Var("state", []string{"PASSED", "FAILED"})
	if err := client.Run(ctx, req, &res); err != nil {
		return nil, err
	}

	return &res.Pipeline.Builds.Edges[0].build, nil
}

func buildClient() *graphql.Client {
	graphqlClient := graphql.NewClient("https://graphql.buildkite.com/v1")
	if *debug {
		graphqlClient.Log = func(s string) { log.Println(s) }
	}

	return graphqlClient
}

func buildRequest(query string) *graphql.Request {
	req := graphql.NewRequest(query)

	auth := fmt.Sprintf("Bearer %s", *apiToken)
	req.Header.Set("Authorization", auth)
	req.Var("slug", *buildSlug)
	req.Var("jobCount", *jobCount)

	return req
}
