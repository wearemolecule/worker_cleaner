// Package main is the main entrypoint to the worker cleaner
package main

import (
	"fmt"
	"io/ioutil"
	"log"
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

	log.Print("Polling worker list every 1min")
	for {

		runningPods := getLivingWorkers(kubeClient, namespace)
		redisWorkers := getWorkersFromRedis(redisClient)

		deadWorkers := getDeadWorkers(runningPods, redisWorkers)

		for _, dead := range deadWorkers {
			removeDeadWorker(redisClient, dead)
		}

		time.Sleep(1 * time.Minute)
	}
}

func removeDeadWorker(c *redis.Client, worker string) {
	c.SRem(workersKey, worker)
	// TODO: Check this key for any open jobs
	c.Del(fmt.Sprintf("%s:%s", workerKey, worker))
	c.Del(fmt.Sprintf("%s:%s:started", workerKey, worker))
	c.Del(fmt.Sprintf("%s:%s:shutdown", workerKey, worker))

	// delete stats
	c.Del(fmt.Sprintf("%s:%s", statProcessedKey, worker))
	c.Del(fmt.Sprintf("%s:%s", statFailedKey, worker))
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
		log.Fatal("Failed to connect to Redis")
	}

	return workers
}

func getRedisClient(useSentinel bool) *redis.Client {
	if useSentinel {
		return redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    "mymaster",
			SentinelAddrs: []string{""},
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
