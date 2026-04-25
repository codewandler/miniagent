package agent

import "github.com/codewandler/agentsdk/runnertest"

const TestServiceID = "test"
const TestModelID = "test-model"

func newFakeClient() *runnertest.Client {
	return runnertest.NewClient(runnertest.TextStream("test response", "msg_test"))
}
