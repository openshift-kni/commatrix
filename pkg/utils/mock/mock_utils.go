// Code generated by MockGen. DO NOT EDIT.
// Source: utils.go

// Package mock_utils is a generated GoMock package.
package mock_utils

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1 "k8s.io/api/core/v1"
)

// MockUtilsInterface is a mock of UtilsInterface interface.
type MockUtilsInterface struct {
	ctrl     *gomock.Controller
	recorder *MockUtilsInterfaceMockRecorder
}

// MockUtilsInterfaceMockRecorder is the mock recorder for MockUtilsInterface.
type MockUtilsInterfaceMockRecorder struct {
	mock *MockUtilsInterface
}

// NewMockUtilsInterface creates a new mock instance.
func NewMockUtilsInterface(ctrl *gomock.Controller) *MockUtilsInterface {
	mock := &MockUtilsInterface{ctrl: ctrl}
	mock.recorder = &MockUtilsInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUtilsInterface) EXPECT() *MockUtilsInterfaceMockRecorder {
	return m.recorder
}

// CreateNamespace mocks base method.
func (m *MockUtilsInterface) CreateNamespace(namespace string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateNamespace", namespace)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateNamespace indicates an expected call of CreateNamespace.
func (mr *MockUtilsInterfaceMockRecorder) CreateNamespace(namespace interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateNamespace", reflect.TypeOf((*MockUtilsInterface)(nil).CreateNamespace), namespace)
}

// CreatePodOnNode mocks base method.
func (m *MockUtilsInterface) CreatePodOnNode(nodeName, namespace, image string) (*v1.Pod, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePodOnNode", nodeName, namespace, image)
	ret0, _ := ret[0].(*v1.Pod)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreatePodOnNode indicates an expected call of CreatePodOnNode.
func (mr *MockUtilsInterfaceMockRecorder) CreatePodOnNode(nodeName, namespace, image interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePodOnNode", reflect.TypeOf((*MockUtilsInterface)(nil).CreatePodOnNode), nodeName, namespace, image)
}

// DeleteNamespace mocks base method.
func (m *MockUtilsInterface) DeleteNamespace(namespace string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteNamespace", namespace)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteNamespace indicates an expected call of DeleteNamespace.
func (mr *MockUtilsInterfaceMockRecorder) DeleteNamespace(namespace interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteNamespace", reflect.TypeOf((*MockUtilsInterface)(nil).DeleteNamespace), namespace)
}

// DeletePod mocks base method.
func (m *MockUtilsInterface) DeletePod(pod *v1.Pod) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeletePod", pod)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeletePod indicates an expected call of DeletePod.
func (mr *MockUtilsInterfaceMockRecorder) DeletePod(pod interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeletePod", reflect.TypeOf((*MockUtilsInterface)(nil).DeletePod), pod)
}

// RunCommandOnPod mocks base method.
func (m *MockUtilsInterface) RunCommandOnPod(pod *v1.Pod, command []string) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RunCommandOnPod", pod, command)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RunCommandOnPod indicates an expected call of RunCommandOnPod.
func (mr *MockUtilsInterfaceMockRecorder) RunCommandOnPod(pod, command interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunCommandOnPod", reflect.TypeOf((*MockUtilsInterface)(nil).RunCommandOnPod), pod, command)
}

// WriteFile mocks base method.
func (m *MockUtilsInterface) WriteFile(path string, data []byte) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteFile", path, data)
	ret0, _ := ret[0].(error)
	return ret0
}

// WriteFile indicates an expected call of WriteFile.
func (mr *MockUtilsInterfaceMockRecorder) WriteFile(path, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WriteFile", reflect.TypeOf((*MockUtilsInterface)(nil).WriteFile), path, data)
}
