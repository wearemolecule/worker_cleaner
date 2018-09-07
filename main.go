// Package main is the main entrypoint to the worker cleaner
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/wearemolecule/worker_cleaner/kubernetes"
	"gopkg.in/redis.v4"
)

const (
	resqueWorkerKey      = "resque:worker"
	resqueWorkersKey     = "resque:workers"
	resqueQueueKey       = "resque:queue"
	resqueQueuesKey      = "resque:queues"
	resqueFailedQueueKey = "resque:failed"
	statProcessedKey     = "resque:stat:processed"
	statFailedKey        = "resque:stat:failed"
)

var (
	blacklistedResqueQueues []string
	namespace               string
	labelSelector           string
	kubeConfig              string
)

func init() {
	flag.Parse()
	blacklistedResqueQueues = strings.Split(os.Getenv("BLACKLISTED_QUEUES"), ",")
	namespace = os.Getenv("NAMESPACE")
	labelSelector = os.Getenv("LABEL_SELECTOR")
	kubeConfig = os.Getenv("KUBE_CONFIG")
}

func main() {
	glog.V(0).Info("Kubernetes-Resque Worker Cleanup Service")

	kubeClient, err := kubernetes.NewClient(kubeConfig)
	if err != nil {
		glog.Fatal(errors.Wrap(err, "Unable to create kubernetes client").Error())
	}
	redisClient := getRedisClient()

	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for _ = range ticker.C {
		glog.V(1).Infof("starting cleanup at %v...", time.Now())

		kubernetesWorkers, err := getWorkersFromKubernetes(kubeClient)
		if err != nil {
			glog.Warning(errors.Wrap(err, "Unable to get workers from kubernetes").Error())
			continue
		}

		redisWorkers, err := getWorkersFromRedis(redisClient)
		if err != nil {
			glog.Warning(errors.Wrap(err, "Unable to get workers from redis").Error())
			continue
		}

		deadRedisWorkers := redisWorkersNotInKubernetes(redisWorkers, kubernetesWorkers)

		glog.V(2).Infof("Found %d kubernetes workers", len(kubernetesWorkers))
		glog.V(2).Info(kubernetesWorkers)

		glog.V(2).Infof("Found %d redis workers", len(redisWorkers))
		glog.V(2).Info(redisWorkers)

		glog.V(2).Infof("Found %d dead redis workers", len(deadRedisWorkers))
		glog.V(2).Info(deadRedisWorkers)

		for _, redisWorker := range deadRedisWorkers {
			if err := removeDeadRedisWorker(redisClient, redisWorker); err != nil {
				glog.Warning(errors.Wrap(err, fmt.Sprintf("Unable to delete %s", redisWorker)).Error())
			}
		}

		failedResqueJobs, err := getDirtyExitFailedJobsFromRedis(redisClient)
		if err != nil {
			glog.Warning(errors.Wrap(err, "Unable to get failed jobs from redis").Error())
			continue
		}

		glog.V(2).Infof("Found %d failed resque jobs", len(failedResqueJobs))
		glog.V(2).Info(failedResqueJobs)

		for _, failedResqueJob := range failedResqueJobs {
			if err := removeFailedResqueJob(redisClient, failedResqueJob); err != nil {
				glog.Warning(errors.Wrap(err, "Unable to retry failed resque job").Error())
			}
		}

		glog.V(1).Infof("finished cleanup at %v...", time.Now())
	}
}

func getWorkersFromKubernetes(kubeClient *kubernetes.Client) ([]string, error) {
	podList, err := kubeClient.ListPods(namespace, labelSelector)
	if err != nil {
		return []string{}, errors.Wrap(err, "Unable to list pods")
	}

	var podNames []string
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.Name)
	}

	return podNames, nil
}

func getWorkersFromRedis(redisClient *redis.Client) ([]redisWorker, error) {
	workers, err := redisClient.SMembers(resqueWorkersKey).Result()
	if err != nil {
		return []redisWorker{}, errors.Wrap(err, "Failed to get resque workers")
	}

	var redisWorkers []redisWorker
OUTER:
	for _, worker := range workers {
		workerName := strings.SplitN(worker, ":", 2)[0]
		queueName := strings.SplitN(worker, ":", 2)[1]

		for _, blacklistedQueue := range blacklistedResqueQueues {
			if strings.Contains(blacklistedQueue, queueName) || strings.Contains(queueName, blacklistedQueue) {
				continue OUTER
			}
		}

		redisWorkers = append(redisWorkers, redisWorker{workerName, worker})
	}

	return redisWorkers, nil
}

type redisWorker struct {
	name string
	info string
}

func getDirtyExitFailedJobsFromRedis(redisClient *redis.Client) ([]redisWorker, error) {
	failedJobsLength, err := redisClient.LLen(resqueFailedQueueKey).Result()
	if err != nil {
		return []redisWorker{}, errors.Wrap(err, "Failed to get failed resque jobs length")
	}

	failedJobs, err := redisClient.LRange(resqueFailedQueueKey, 0, failedJobsLength).Result()
	if err != nil {
		return []redisWorker{}, errors.Wrap(err, "Failed to get failed resque jobs")
	}

	var failedResqueJobs []redisWorker
	for _, failedJob := range failedJobs {
		var job failedResqueJob
		if err := json.Unmarshal([]byte(failedJob), &job); err != nil {
			continue
		}

		if regexp.MustCompile(`^Resque::.*DirtyExit`).MatchString(job.Exception) {
			failedResqueJobs = append(failedResqueJobs, redisWorker{failedJob, failedJob})
		}
	}

	return failedResqueJobs, nil
}

type failedResqueJob struct {
	Exception string `json:"exception"`
}

func redisWorkersNotInKubernetes(redisWorkers []redisWorker, kubernetesWorkers []string) []redisWorker {
	var deadRedisWorkers []redisWorker

OUTER:
	for _, redisWorker := range redisWorkers {
		for _, kubernetesWorker := range kubernetesWorkers {
			if kubernetesWorker == redisWorker.name {
				continue OUTER
			}
		}

		deadRedisWorkers = append(deadRedisWorkers, redisWorker)
	}

	return deadRedisWorkers
}

func removeDeadRedisWorker(redisClient *redis.Client, redisWorker redisWorker) error {
	bytes, err := redisClient.Get(fmt.Sprintf("%s:%s", resqueWorkerKey, redisWorker.info)).Bytes()
	if err != nil && err != redis.Nil {
		return errors.Wrap(err, fmt.Sprintf("Error getting %s from redis", redisWorker))
	}
	if err != nil && err == redis.Nil {
		// Redis key not present so the issue probably corrected itself
		return nil
	}

	if err := retryDeadWorker(redisClient, bytes); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to retry %s", redisWorker))
	}

	redisClient.Pipelined(func(pipe *redis.Pipeline) error {
		pipe.SRem(resqueWorkersKey, redisWorker.info)
		pipe.Del(fmt.Sprintf("%s:%s", resqueWorkerKey, redisWorker.info))
		pipe.Del(fmt.Sprintf("%s:%s:started", resqueWorkerKey, redisWorker.info))
		pipe.Del(fmt.Sprintf("%s:%s:shutdown", resqueWorkerKey, redisWorker.info))
		pipe.Del(fmt.Sprintf("%s:%s", statProcessedKey, redisWorker.info))
		pipe.Del(fmt.Sprintf("%s:%s", statFailedKey, redisWorker.info))
		return nil
	})

	return nil
}

func removeFailedResqueJob(redisClient *redis.Client, redisWorker redisWorker) error {
	if err := retryDeadWorker(redisClient, []byte(redisWorker.info)); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to retry %s", redisWorker.info))
	}

	redisClient.Pipelined(func(pipe *redis.Pipeline) error {
		pipe.LRem(resqueFailedQueueKey, 1, redisWorker.info)
		return nil
	})

	return nil
}

func retryDeadWorker(redisClient *redis.Client, workerData []byte) error {
	resqueJob, err := newResqueJob(workerData)
	if err != nil {
		return errors.Wrap(err, "Unable to deserialize job")
	}

	glog.V(3).Infof("Going to retry job on queue: %s, with payload: %s", resqueJob.Queue, resqueJob.Payload)

	if err := redisClient.SAdd(resqueQueuesKey, resqueJob.Queue).Err(); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to create resque queue %s", resqueJob.Queue))
	}

	resqueJobJSON, err := resqueJob.Payload.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "Could not serialize job payload")
	}

	if err = redisClient.RPush(fmt.Sprintf("%s:%s", resqueQueueKey, resqueJob.Queue), string(resqueJobJSON)).Err(); err != nil {
		return errors.Wrap(err, "Failed to insert job into resque queue")
	}

	return nil
}

func newResqueJob(data []byte) (resqueJob, error) {
	var j resqueJob
	err := json.Unmarshal(data, &j)
	return j, err
}

type resqueJob struct {
	Queue   string `json:"queue"`
	Payload json.RawMessage
}

func getRedisClient() *redis.Client {
	if kubeConfig == "" {
		addr := fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT"))
		return redis.NewClient(&redis.Options{
			Addr: addr,
		})
	} else {
		return redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
	}
}
