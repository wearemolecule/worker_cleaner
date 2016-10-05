![Molecule Software](https://avatars1.githubusercontent.com/u/2736908?v=3&s=100 "Molecule Software")
# Kubernetes Worker Cleaner

A kubernetes utility to clean up stale worker entries from a resque queue system.

## Why We Made This

We love (and use!) [kubernetes](http://kubernetes.io/) and [resque](https://github.com/resque/resque) but sometimes resque doesn't clean up after itself correctly. We built this utility to remove lingering resque jobs when their kubernetes counterparts have already finished.

## To Use

The worker cleaner requires a kubernetes cluster with a configured resque queue system. If you have those set up, great keep reading! If not, there are [guides](http://kubernetes.io/docs/getting-started-guides/) available to walk you through setting up a local or hosted kubernetes solution ([minikube](http://kubernetes.io/docs/getting-started-guides/minikube/) is a really simple way to get started) and the resque docs are a good place to learn more about resque. 

Once your cluster is configured you can simply run this utility inside one of your namespaces. Our setup looks like this:
- We have a single "Worker" replication controller
- We use [redis sentinels](http://redis.io/topics/sentinel) inside of our kubernetes cluster
- Each resque job is assigned to a pod
- Each resque job is listed in the redis `resque:workers` key
- Each resque job can be got by looking at the `resque:worker:#{worker-name}` redis key

## Getting things Running

1. Clone this repo.
1. Run `glide install -v -s` (more info on [glide](https://github.com/Masterminds/glide)).
1. Run `make gtest` to run tests.
1. Set environment variables (all are optional and depends on your setup):
    * `CERTS_PATH` should be location of your kubernetes credentials.
    * `KUBERNETES_SERVICE_HOST` should be the host name of your kubernetes cluster.
    * `KUBERNETES_SERVICE_PORT` should be the port of your kubernetes cluster.
    * `POD_ROLE` should be the role label your kuberenetes workers are listed under (we use "worker").
    * `POD_APP` should be the app label your kuberenetes workers are listed under (we use our company name).
    * `NAMESPACE` should be the namespace of your kubernetes workers.
    * `KUBE` should "true" if using redis sentinels.
    * `REDIS_SENTINEL_SERVICE_HOST` should be the host of your redis sentinel service.
    * `REDIS_SENTINEL_SERVICE_PORT` should be the port of your redis sentinel service.
    * `BLACKLISTED_QUEUES` should be set if you have resque workers working on queues that you don't want to clean up.
1. Run `make run` to start the utility.

## Gotchas

* The worker cleaner uses [glog](https://github.com/golang/glog) to handle levelled logging. By default the logging is set to "basically everything" (check out the Makefile) and can be adjusted.

## Copyright and License

Copyright © 2016 Molecule Software, Inc. All Rights Reserved.

Licensed under the MIT License (the "License"). You may not use this work except in compliance with the License. You may obtain a copy of the License in the LICENSE file.
