package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDeadWorkers(t *testing.T) {
	runningPods := []string{"vapor-worker-v1-abcd", "vapor-worker-v1-dcba"}
	redisWorkers := []string{"vapor-worker-v1-abcd:*", "vapor-worker-v1-dcba:*", "vapor-worker-v1-abbb:*"}

	dead := getDeadWorkers(runningPods, redisWorkers)
	expectedDead := []string{"vapor-worker-v1-abbb:*"}

	assert.Equal(t, expectedDead, dead, "they should be equal")
}
