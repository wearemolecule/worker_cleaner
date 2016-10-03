// Package main is the main entrypoint to the worker cleaner
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/wearemolecule/kubeclient"
	"gopkg.in/redis.v4"
)

const (
	workerKey        = "resque:worker"
	workersKey       = "resque:workers"
	statProcessedKey = "resque:stat:processed"
	statFailedKey    = "resque:stat:failed"
)

func init() {
	flag.Parse()
}

func main() {
	glog.Info("Kubernetes-Resque Worker Cleanup Service")

	namespace := os.Getenv("NAMESPACE")
	kubeClient, err := kubeclient.GetKubeClientFromEnv()
	if err != nil {
		glog.Fatalf("Couldn't connect to kubernetes: %s", err)
	}
	redisClient := getRedisClient(true)

	glog.Info("Polling worker list every 5 min")
	for {
		runningPods := getLivingWorkers(kubeClient, namespace)
		glog.Infof("Got running pods %s", runningPods)
		redisWorkers := getWorkersFromRedis(redisClient)
		glog.Infof("Got redis workers %s", redisWorkers)

		deadWorkers := getDeadWorkers(runningPods, redisWorkers)
		glog.Infof("Removing dead workers %s", deadWorkers)

		for _, dead := range deadWorkers {
			removeDeadWorker(redisClient, dead)
		}

		time.Sleep(5 * time.Minute)
	}
}

type resqueJob struct {
	Queue   string `json:"queue"`
	Payload json.RawMessage
}

func (j *resqueJob) QueueKey() string {
	return fmt.Sprintf("resque:queue:%s", j.Queue)
}

func newResqueJob(data []byte) (resqueJob, error) {
	var j resqueJob
	err := json.Unmarshal(data, &j)
	return j, err
}

func requeueStuckJob(jobBytes []byte, c *redis.Client) error {
	var err error
	job, err := newResqueJob(jobBytes)
	if err != nil {
		glog.Warning("Could not deserialize job")
		return err
	}
	glog.Infof("Job found on queue %s: %s", job.Queue, job.Payload)
	c.SAdd("resque:queues", job.Queue)
	json, err := job.Payload.MarshalJSON()
	if err != nil {
		glog.Warning("Could not serialize job payload")
		return err
	}
	err = c.RPush(job.QueueKey(), string(json[:])).Err()
	if err != nil {
		glog.Warningf("Failed to insert job: %s", err)
		return err
	}

	return err
}

func removeDeadWorker(c *redis.Client, worker string) {
	bytes, err := c.Get(fmt.Sprintf("%s:%s", workerKey, worker)).Bytes()
	if err != nil {
		// if error is redis: nil we just ignore
		if err != redis.Nil {
			glog.Warningf("Could not fetch job for obj: %s", err)
			return
		}
	} else {
		err = requeueStuckJob(bytes, c)
		if err != nil {
			glog.Warningf("Failed to requeue job: %s", err)
			return
		}
		// Else continue to cleaning up the worker
	}

	c.Pipelined(func(pipe *redis.Pipeline) error {
		pipe.SRem(workersKey, worker)
		pipe.Del(fmt.Sprintf("%s:%s", workerKey, worker))
		pipe.Del(fmt.Sprintf("%s:%s:started", workerKey, worker))
		pipe.Del(fmt.Sprintf("%s:%s:shutdown", workerKey, worker))

		// delete stats
		pipe.Del(fmt.Sprintf("%s:%s", statProcessedKey, worker))
		pipe.Del(fmt.Sprintf("%s:%s", statFailedKey, worker))

		return nil
	})
}

func getDeadWorkers(running []string, listedWorkers []string) []string {
	var diff []string

	for _, worker := range listedWorkers {
		workerName := strings.SplitN(worker, ":", 2)[0]
		found := false
		for _, pod := range running {
			if workerName == pod {
				found = true
				break
			}
		}
		if !found {
			diff = append(diff, worker)
		}
	}

	return diff
}

func getLivingWorkers(c *kubeclient.Client, namespace string) []string {
	ctx := context.TODO()
	pods, err := c.PodList(ctx, namespace, "role=worker,app=vapor")
	if err != nil {
		glog.Fatal("Failed to get pods", err)
	}

	podNames := make([]string, len(pods))
	for i, pod := range pods {
		podNames[i] = pod.Name
	}
	return podNames
}

func getWorkersFromRedis(c *redis.Client) []string {
	workers, err := c.SMembers("resque:workers").Result()
	if err != nil {
		glog.Fatal("Failed to connect to Redis")
	}

	return workers
}

func getRedisClient(useSentinel bool) *redis.Client {
	if useSentinel {
		addr := fmt.Sprintf("%s:%s", os.Getenv("REDIS_SENTINEL_SERVICE_HOST"), os.Getenv("REDIS_SENTINEL_SERVICE_PORT"))
		return redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    "mymaster",
			SentinelAddrs: []string{addr},
		})
	} else {
		return redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
	}
}
