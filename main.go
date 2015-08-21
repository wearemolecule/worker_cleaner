// Package main is the main entrypoint to the worker cleaner
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/golang/glog"
	"gopkg.in/redis.v3"
)

const (
	workerKey        = "resque:worker"
	workersKey       = "resque:workers"
	statProcessedKey = "resque:stat:processed"
	statFailedKey    = "resque:stat:failed"
)

func main() {
	glog.Info("Kubernetes-Resque Worker Cleanup Service")

	namespace := os.Getenv("NAMESPACE")
	kubeClient := getKubernetesClient()
	redisClient := getRedisClient(true)

	glog.Info("Polling worker list every 5 min")
	for {

		runningPods := getLivingWorkers(kubeClient, namespace)
		redisWorkers := getWorkersFromRedis(redisClient)

		deadWorkers := getDeadWorkers(runningPods, redisWorkers)

		for _, dead := range deadWorkers {
			removeDeadWorker(redisClient, dead)
		}

		time.Sleep(5 * time.Minute)
	}
}

type resqueJob struct {
	Queue   string `json:queue`
	Payload json.RawMessage
}

func (j *resqueJob) QueueKey() string {
	return fmt.Sprintf("resque:queue:%s", j.Queue)
}

func (j *resqueJob) PayloadAsJson() ([]byte, error) {
	return json.Marshal(j.Payload)
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
	json, err := job.PayloadAsJson()
	if err != nil {
		glog.Warning("Could not serialize job payload")
		return err
	}
	rowsInserted, err := c.RPush(job.QueueKey(), string(json)).Result()
	if err != nil {
		glog.Warningf("Failed to insert job: %s", err)
		return err
	}
	if rowsInserted != 1 {
		err = errors.New("Failed to insert job")
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
			glog.Warning("Failed to requeue job")
		}
		return
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

func getLivingWorkers(c *client.Client, namespace string) []string {
	selector := "role=worker,app=vapor"
	labels, err := labels.Parse(selector)
	if err != nil {
		glog.Fatalf("Failed to parse selector %q: %v", selector, err)
	}

	pods, err := c.Pods(namespace).List(labels, fields.Everything())
	if err != nil {
		glog.Fatal("Failed to get pods")
	}

	podItems := pods.Items
	podNames := make([]string, len(podItems))
	for i, pod := range podItems {
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

func getKubernetesClient() *client.Client {
	kubernetesService := os.Getenv("KUBERNETES_SERVICE_HOST")
	if kubernetesService == "" {
		glog.Fatalf("Please specify the Kubernetes server")
	}
	apiServer := fmt.Sprintf("https://%s:%s", kubernetesService, os.Getenv("KUBERNETES_SERVICE_PORT"))

	token, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		glog.Fatalf("No service account token found")
	}

	config := client.Config{
		Host:        apiServer,
		BearerToken: string(token),
		Insecure:    true,
	}

	c, err := client.New(&config)
	if err != nil {
		glog.Fatalf("Failed to make client: %v", err)
	}
	return c
}
