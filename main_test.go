package main

import "testing"

func TestRedisWorkersNotInKubernetes(t *testing.T) {
	testCases := []struct {
		kubernetesWorkers []string
		redisWorkers      []redisWorker
		expectedResult    []redisWorker
	}{
		// No dead redis workers should return an empty list
		{
			[]string{"vapor-worker-v1-abcd", "vapor-worker-v1-dcba"},
			[]redisWorker{
				{"vapor-worker-v1-abcd", "vapor-worker-v1-abcd:*"},
				{"vapor-worker-v1-dcba", "vapor-worker-v1-dcba:*"},
			},
			[]redisWorker{},
		},
		// A dead worker should return a list that includes the dead worker
		{
			[]string{"vapor-worker-v1-abcd", "vapor-worker-v1-dcba"},
			[]redisWorker{
				{"vapor-worker-v1-abcd", "vapor-worker-v1-abcd:*"},
				{"vapor-worker-v1-dcba", "vapor-worker-v1-dcba:*"},
				{"vapor-worker-v1-abbb", "vapor-worker-v1-abbb:*"},
			},
			[]redisWorker{{"vapor-worker-v1-abbb", "vapor-worker-v1-abbb:*"}},
		},
		// A dead kubernetes worker should return an empty list
		{
			[]string{"vapor-worker-v1-abcd", "vapor-worker-v1-dcba"},
			[]redisWorker{
				{"vapor-worker-v1-abcd", "vapor-worker-v1-abcd:*"},
			},
			[]redisWorker{},
		},
	}

	for _, testCase := range testCases {
		deadRedisWorkers := redisWorkersNotInKubernetes(testCase.redisWorkers, testCase.kubernetesWorkers)
		if !equal(deadRedisWorkers, testCase.expectedResult) {
			t.Error("Expected", testCase.expectedResult, "got", deadRedisWorkers)
		}
	}
}

func equal(s1, s2 []redisWorker) bool {
	if len(s1) != len(s2) {
		return false
	}

	for i, s := range s1 {
		if s2[i].name != s.name || s2[i].info != s.info {
			return false
		}
	}

	return true
}
