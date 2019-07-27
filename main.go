package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/machinebox/graphql"
	"github.com/y-yagi/go-buildkite/buildkite"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	apiToken  = kingpin.Flag("token", "API token").Required().OverrideDefaultFromEnvar("BUILDKITE_API_TOKEN").String()
	buildSlug = kingpin.Flag("slug", "Build slug(organization-slug/pipeline-slug/build-number)").Required().String()
	debug     = kingpin.Flag("debug", "Enable debugging").Bool()
	jobCount  = kingpin.Flag("count", "Number of jobs").Default("30").Int()
)

type JobsResponse struct {
	Build struct {
		Jobs Jobs
	}
}

type PipelineResponse struct {
	Pipeline struct {
		Builds struct {
			Edges []struct {
				Build struct {
					ID   string
					Jobs Jobs
				} `json:"Node"`
			}
		}
	}
}

type Jobs struct {
	Edges []struct {
		Job struct {
			Uuid string
		} `json:"Node"`
	}
}

const BuildQuery = `
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

const PipelineQuery = `
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

	var query string
	var jobs Jobs
	var buildID string
	if strings.Count(*buildSlug, "/") > 1 {
		query = BuildQuery
	} else {
		query = PipelineQuery
	}

	graphqlClient := graphql.NewClient("https://graphql.buildkite.com/v1")
	if *debug {
		graphqlClient.Log = func(s string) { log.Println(s) }
	}
	req := graphql.NewRequest(query)

	auth := fmt.Sprintf("Bearer %s", *apiToken)
	req.Header.Set("Authorization", auth)
	req.Var("slug", *buildSlug)
	req.Var("jobCount", *jobCount)
	ctx := context.Background()

	if strings.Count(*buildSlug, "/") > 1 {
		var jobsResponse JobsResponse
		if err := graphqlClient.Run(ctx, req, &jobsResponse); err != nil {
			log.Fatalf("graphql failed: %s", err)
		}
		jobs = jobsResponse.Build.Jobs
		buildID = strings.Split(*buildSlug, "/")[2]
	} else {
		var pipelineResponse PipelineResponse
		req.Var("state", []string{"PASSED", "FAILED"})
		if err := graphqlClient.Run(ctx, req, &pipelineResponse); err != nil {
			log.Fatalf("graphql failed: %s", err)
		}
		jobs = pipelineResponse.Pipeline.Builds.Edges[0].Build.Jobs
		buildID = pipelineResponse.Pipeline.Builds.Edges[0].Build.ID
	}

	config, err := buildkite.NewTokenConfig(*apiToken, *debug)
	if err != nil {
		log.Fatalf("client config failed: %s", err)
	}

	buildkiteClient := buildkite.NewClient(config.Client())
	jobArgs := strings.Split(*buildSlug, "/")
	logger := log.New(os.Stdout, "", 0)
	var wg sync.WaitGroup

	for _, edge := range jobs.Edges {
		wg.Add(1)

		go func(uuid string) {
			defer wg.Done()
			jobLog, _, err := buildkiteClient.Jobs.GetJobLog(jobArgs[0], jobArgs[1], buildID, uuid)
			if err != nil {
				errmsg := fmt.Sprintf("Error: %v\n", err)
				logger.Print(errmsg)
			} else {
				logger.Print(*jobLog.Content)
			}
		}(edge.Job.Uuid)
	}
	wg.Wait()
}
