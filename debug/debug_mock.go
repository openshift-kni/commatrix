package debug

import (
	"time"

	"github.com/openshift-kni/commatrix/client"
	"github.com/stretchr/testify/mock"
)

// MockDebugPod is a mock type for the DebugPodInterface// MockDebugPod is a mock type for the DebugPodInterface
type MockDebugPod struct {
	mock.Mock
}

// ExecWithRetry mocks the ExecWithRetry method
func (m *MockDebugPod) ExecWithRetry(command string, interval time.Duration, duration time.Duration) ([]byte, error) {
	args := m.Called(command, interval, duration)
	return args.Get(0).([]byte), args.Error(1)
}

// Clean mocks the Clean method
func (m *MockDebugPod) Clean() error {
	args := m.Called()
	return args.Error(0)
}

// GetNodeName mocks the GetNodeName method
func (m *MockDebugPod) GetNodeName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockDebugPod) New(cs *client.ClientSet, node string, namespace string, image string) (DebugPodInterface, error) {
	args := m.Called(cs, node, namespace, image)
	return args.Get(0).(DebugPodInterface), args.Error(1)
}
