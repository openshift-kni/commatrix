package endpointslices

import (
	"context"
	"errors"
	"strings"
	"testing"

	commatrixclient "github.com/openshift-kni/commatrix/pkg/client"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type TestCase struct {
	desc      string
	podName   string
	nodeName  string
	ownerRefs []metav1.OwnerReference
	expected  string
}

func TestExtractPodName(t *testing.T) {
	tests := []TestCase{
		{
			desc:     "with-no-owner-reference",
			podName:  "kube-rbac-proxy",
			expected: "kube-rbac-proxy",
		},
		{
			desc:     "with-owner-reference-kind-node",
			nodeName: "worker-node",
			podName:  "kube-rbac-proxy-worker-node",
			ownerRefs: []metav1.OwnerReference{
				{
					Kind: "Node",
				},
			},
			expected: "kube-rbac-proxy",
		},
		{
			desc: "with-owner-reference-kind-replicaset",
			ownerRefs: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "kube-rbac-proxy-7b7df454c7",
				},
			},
			expected: "kube-rbac-proxy",
		},
		{
			desc: "with-owner-reference-kind-daemonset",
			ownerRefs: []metav1.OwnerReference{
				{
					Kind: "DaemonSet",
					Name: "kube-rbac-proxy",
				},
			},
			expected: "kube-rbac-proxy",
		},
		{
			desc: "with-owner-reference-kind-statefulset",
			ownerRefs: []metav1.OwnerReference{
				{
					Kind: "StatefulSet",
					Name: "kube-rbac-proxy",
				},
			},
			expected: "kube-rbac-proxy",
		},
	}
	for _, test := range tests {
		p := defineTestPod(&test)
		res, err := extractControllerName(p)
		if err != nil {
			t.Fatalf("test %s failed. got error: %s", test.desc, err)
		}
		if res != test.expected {
			t.Fatalf("test %s failed. expected %v got %v", test.desc, test.expected, res)
		}
	}
}

func defineTestPod(t *TestCase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: t.podName, OwnerReferences: t.ownerRefs},
		Spec:       corev1.PodSpec{NodeName: t.nodeName},
	}
}

func TestLoadExposedEndpointSlicesInfoBatchesByNamespace(t *testing.T) {
	services := []*corev1.Service{
		testService("service-a", "team-a", map[string]string{"app": "a"}),
		testService("service-b", "team-a", map[string]string{"app": "b"}),
		testService("service-c", "team-b", map[string]string{"app": "c"}),
	}
	objects := []rtclient.Object{
		services[0], services[1], services[2],
		testPod("pod-a", "team-a", "a", 8080),
		testPod("pod-b", "team-a", "b", 8081),
		testPod("pod-c", "team-b", "c", 8082),
		testEndpointSlice("slice-a-1", "team-a", "service-a", 8080),
		testEndpointSlice("slice-a-2", "team-a", "service-a", 8080),
		testEndpointSlice("slice-b", "team-a", "service-b", 8081),
		testEndpointSlice("slice-c", "team-b", "service-c", 8082),
	}
	listCalls := map[string]int{}
	listNamespaces := map[string][]string{}
	listInterceptor := interceptor.Funcs{
		List: func(ctx context.Context, c rtclient.WithWatch, list rtclient.ObjectList, opts ...rtclient.ListOption) error {
			listOptions := &rtclient.ListOptions{}
			listOptions.ApplyOptions(opts)
			resource := listResource(list)
			listCalls[resource]++
			if resource != "services" {
				listNamespaces[resource] = append(listNamespaces[resource], listOptions.Namespace)
			}
			return c.List(ctx, list, opts...)
		},
	}
	exporter := newTestExporter(t, listInterceptor, objects...)

	if err := exporter.LoadExposedEndpointSlicesInfo(); err != nil {
		t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v", err)
	}

	if listCalls["services"] != 1 || listCalls["pods"] != 2 || listCalls["endpointslices"] != 2 {
		t.Errorf("unexpected list call counts: got %v, want 1 service, 2 pod and 2 EndpointSlice lists", listCalls)
	}
	for resource, gotNamespaces := range listNamespaces {
		gotNamespaceSet := map[string]bool{}
		for _, namespace := range gotNamespaces {
			gotNamespaceSet[namespace] = true
		}
		if len(gotNamespaceSet) != 2 || !gotNamespaceSet["team-a"] || !gotNamespaceSet["team-b"] {
			t.Errorf("%s list namespaces = %v, want [team-a team-b]", resource, gotNamespaces)
		}
	}
	if len(exporter.sliceInfo) != 4 {
		t.Fatalf("got %d EndpointSlice infos, want 4", len(exporter.sliceInfo))
	}

	gotSlices := map[string]string{}
	for _, info := range exporter.sliceInfo {
		gotSlices[info.EndpointSlice.Name] = info.Service.Name
	}
	for sliceName, serviceName := range map[string]string{
		"slice-a-1": "service-a",
		"slice-a-2": "service-a",
		"slice-b":   "service-b",
		"slice-c":   "service-c",
	} {
		if gotSlices[sliceName] != serviceName {
			t.Errorf("slice %q associated with service %q, want %q", sliceName, gotSlices[sliceName], serviceName)
		}
	}
}

func TestLoadExposedEndpointSlicesInfoSkipsServicesWithoutUsableEndpoints(t *testing.T) {
	validService := testService("service-a", "team-a", map[string]string{"app": "a"})
	endpointSlice := testEndpointSlice("slice-a", "team-a", "service-a", 8080)
	unlabeledEndpointSlice := testEndpointSlice("slice-a", "team-a", "service-a", 8080)
	unlabeledEndpointSlice.Labels = nil
	podNetworkPod := testPod("pod-a", "team-a", "a", 8080)
	podNetworkPod.Spec.HostNetwork = false
	localhostPod := testPod("pod-a", "team-a", "a", 8080)
	localhostPod.Spec.Containers[0].Ports[0].HostIP = "127.0.0.1"
	tests := []struct {
		name              string
		objects           []rtclient.Object
		wantEndpointLists int
		wantPodLists      int
	}{
		{
			name: "service without selector",
			objects: []rtclient.Object{
				testService("service-a", "team-a", nil),
				endpointSlice,
			},
		},
		{
			name: "service without EndpointSlices",
			objects: []rtclient.Object{
				validService,
				testPod("pod-a", "team-a", "a", 8080),
			},
			wantEndpointLists: 1,
			wantPodLists:      1,
		},
		{
			name: "EndpointSlice without service-name label",
			objects: []rtclient.Object{
				validService,
				unlabeledEndpointSlice,
				testPod("pod-a", "team-a", "a", 8080),
			},
			wantEndpointLists: 1,
			wantPodLists:      1,
		},
		{
			name: "non-hostNetwork Pod without hostPort",
			objects: []rtclient.Object{
				validService,
				endpointSlice,
				podNetworkPod,
			},
			wantEndpointLists: 1,
			wantPodLists:      1,
		},
		{
			name: "hostNetwork Pod with only localhost-bound ports",
			objects: []rtclient.Object{
				validService,
				endpointSlice,
				localhostPod,
			},
			wantEndpointLists: 1,
			wantPodLists:      1,
		},
		{
			name: "service without matching Pods",
			objects: []rtclient.Object{
				validService,
				endpointSlice,
				testPod("pod-other", "team-a", "other", 8080),
			},
			wantEndpointLists: 1,
			wantPodLists:      1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listCalls := map[string]int{}
			listInterceptor := interceptor.Funcs{
				List: func(ctx context.Context, c rtclient.WithWatch, list rtclient.ObjectList, opts ...rtclient.ListOption) error {
					listCalls[listResource(list)]++
					return c.List(ctx, list, opts...)
				},
			}
			exporter := newTestExporter(t, listInterceptor, tt.objects...)
			if err := exporter.LoadExposedEndpointSlicesInfo(); err != nil {
				t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v", err)
			}
			if got := listCalls["endpointslices"]; got != tt.wantEndpointLists {
				t.Errorf("EndpointSlice list calls = %d, want %d", got, tt.wantEndpointLists)
			}
			if got := listCalls["pods"]; got != tt.wantPodLists {
				t.Errorf("Pod list calls = %d, want %d", got, tt.wantPodLists)
			}
			if len(exporter.sliceInfo) != 0 {
				t.Errorf("got %d EndpointSlice infos, want none", len(exporter.sliceInfo))
			}
		})
	}
}

func TestLoadExposedEndpointSlicesInfoFiltersNonHostNetworkPorts(t *testing.T) {
	service := testService("service-a", "team-a", map[string]string{"app": "a"})
	port := int32(8080)
	localhostPort := int32(9090)
	noHostPort := int32(7070)
	protocol := corev1.ProtocolTCP
	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "slice-a",
			Namespace: "team-a",
			Labels:    map[string]string{discoveryv1.LabelServiceName: "service-a"},
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: &port, Protocol: &protocol},
			{Port: &localhostPort, Protocol: &protocol},
			{Port: &noHostPort, Protocol: &protocol},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "team-a", Labels: map[string]string{"app": "a"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "app",
			Ports: []corev1.ContainerPort{
				{ContainerPort: port, HostPort: 18080},
				{ContainerPort: localhostPort, HostPort: 19090, HostIP: "127.0.0.1"},
				{ContainerPort: noHostPort},
			},
		}}},
	}
	exporter := newTestExporter(t, interceptor.Funcs{}, service, endpointSlice, pod)

	if err := exporter.LoadExposedEndpointSlicesInfo(); err != nil {
		t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v", err)
	}
	if len(exporter.sliceInfo) != 1 {
		t.Fatalf("got %d EndpointSlice infos, want 1", len(exporter.sliceInfo))
	}
	filteredPorts := exporter.sliceInfo[0].EndpointSlice.Ports
	if len(filteredPorts) != 1 {
		t.Fatalf("got %d ports after filtering, want 1: %+v", len(filteredPorts), filteredPorts)
	}
	if got := *filteredPorts[0].Port; got != port {
		t.Errorf("retained port = %d, want hostPort-backed non-localhost port %d", got, port)
	}
}

func TestLoadExposedEndpointSlicesInfoKeepsUsableSlicesWhenSiblingFiltered(t *testing.T) {
	service := testService("service-a", "team-a", map[string]string{"app": "a"})
	pod := testPod("pod-a", "team-a", "a", 8080)
	pod.Spec.Containers[0].Ports = append(pod.Spec.Containers[0].Ports,
		corev1.ContainerPort{ContainerPort: 9090, HostIP: "127.0.0.1"})
	exporter := newTestExporter(t, interceptor.Funcs{},
		service,
		pod,
		testEndpointSlice("slice-localhost", "team-a", "service-a", 9090),
		testEndpointSlice("slice-usable", "team-a", "service-a", 8080),
	)

	if err := exporter.LoadExposedEndpointSlicesInfo(); err != nil {
		t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v", err)
	}
	if len(exporter.sliceInfo) != 1 {
		t.Fatalf("got %d EndpointSlice infos, want 1", len(exporter.sliceInfo))
	}
	if got := exporter.sliceInfo[0].EndpointSlice.Name; got != "slice-usable" {
		t.Errorf("kept EndpointSlice %q, want slice-usable", got)
	}
}

func TestLoadExposedEndpointSlicesInfoListErrors(t *testing.T) {
	service := testService("service-a", "team-a", map[string]string{"app": "a"})
	endpointSlice := testEndpointSlice("slice-a", "team-a", "service-a", 8080)
	tests := []struct {
		name          string
		failedList    string
		wantErrorText string
		objects       []rtclient.Object
	}{
		{
			name:          "service list failure",
			failedList:    "services",
			wantErrorText: "failed to list services",
		},
		{
			name:          "EndpointSlice list failure",
			failedList:    "endpointslices",
			wantErrorText: `failed to list endpoint slices in namespace "team-a"`,
			objects:       []rtclient.Object{service},
		},
		{
			name:          "Pod list failure",
			failedList:    "pods",
			wantErrorText: `failed to list pods in namespace "team-a"`,
			objects:       []rtclient.Object{service, endpointSlice},
		},
	}
	listErr := errors.New("injected list failure")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listInterceptor := interceptor.Funcs{
				List: func(ctx context.Context, c rtclient.WithWatch, list rtclient.ObjectList, opts ...rtclient.ListOption) error {
					if listResource(list) == tt.failedList {
						return listErr
					}
					return c.List(ctx, list, opts...)
				},
			}
			exporter := newTestExporter(t, listInterceptor, tt.objects...)

			err := exporter.LoadExposedEndpointSlicesInfo()
			if !errors.Is(err, listErr) {
				t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v, want wrapped list error", err)
			}
			if !strings.Contains(err.Error(), tt.wantErrorText) {
				t.Errorf("error %q does not contain %q", err, tt.wantErrorText)
			}
		})
	}
}

func TestLoadExposedEndpointSlicesInfoPaginatesEndpointSlices(t *testing.T) {
	service := testService("service-a", "team-a", map[string]string{"app": "a"})
	objects := []rtclient.Object{service}
	endpointSlicePage := 0
	podPage := 0
	listInterceptor := interceptor.Funcs{
		List: func(ctx context.Context, c rtclient.WithWatch, list rtclient.ObjectList, opts ...rtclient.ListOption) error {
			listOptions := &rtclient.ListOptions{}
			listOptions.ApplyOptions(opts)
			switch typedList := list.(type) {
			case *discoveryv1.EndpointSliceList:
				if listOptions.Limit != listPageSize {
					t.Errorf("EndpointSlice list limit = %d, want %d", listOptions.Limit, listPageSize)
				}
				endpointSlicePage++
				if endpointSlicePage == 1 {
					if listOptions.Continue != "" {
						t.Errorf("first EndpointSlice page continue token = %q, want empty", listOptions.Continue)
					}
					typedList.Items = []discoveryv1.EndpointSlice{*testEndpointSlice("slice-a-1", "team-a", "service-a", 8080)}
					typedList.Continue = "next-endpointslice-page-2"
					return nil
				}
				if endpointSlicePage == 2 {
					if listOptions.Continue != "next-endpointslice-page-2" {
						t.Errorf("second EndpointSlice page continue token = %q, want next-endpointslice-page-2", listOptions.Continue)
					}
					typedList.Continue = "next-endpointslice-page-3"
					return nil
				}
				if listOptions.Continue != "next-endpointslice-page-3" {
					t.Errorf("third EndpointSlice page continue token = %q, want next-endpointslice-page-3", listOptions.Continue)
				}
				typedList.Items = []discoveryv1.EndpointSlice{*testEndpointSlice("slice-a-2", "team-a", "service-a", 8080)}
				return nil
			case *corev1.PodList:
				if listOptions.Limit != listPageSize {
					t.Errorf("Pod list limit = %d, want %d", listOptions.Limit, listPageSize)
				}
				podPage++
				if podPage == 1 {
					typedList.Items = []corev1.Pod{*testPod("unmatched-pod", "team-a", "other", 8080)}
					typedList.Continue = "next-pod-page-2"
					return nil
				}
				if podPage == 2 {
					if listOptions.Continue != "next-pod-page-2" {
						t.Errorf("second Pod page continue token = %q, want next-pod-page-2", listOptions.Continue)
					}
					typedList.Continue = "next-pod-page-3"
					return nil
				}
				if listOptions.Continue != "next-pod-page-3" {
					t.Errorf("third Pod page continue token = %q, want next-pod-page-3", listOptions.Continue)
				}
				typedList.Items = []corev1.Pod{*testPod("pod-a", "team-a", "a", 8080)}
				return nil
			default:
				return c.List(ctx, list, opts...)
			}
		},
	}
	exporter := newTestExporter(t, listInterceptor, objects...)

	if err := exporter.LoadExposedEndpointSlicesInfo(); err != nil {
		t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v", err)
	}
	if endpointSlicePage != 3 {
		t.Errorf("got %d EndpointSlice pages, want 3", endpointSlicePage)
	}
	if podPage != 3 {
		t.Errorf("got %d Pod pages, want 3", podPage)
	}
	if len(exporter.sliceInfo) != 2 {
		t.Errorf("got %d EndpointSlice infos, want 2", len(exporter.sliceInfo))
	}
}

func TestLoadExposedEndpointSlicesInfoPaginationErrors(t *testing.T) {
	service := testService("service-a", "team-a", map[string]string{"app": "a"})
	endpointSlice := testEndpointSlice("slice-a", "team-a", "service-a", 8080)
	tests := []struct {
		name          string
		failedList    string
		wantErrorText string
		objects       []rtclient.Object
	}{
		{
			name:          "EndpointSlice failure after continuation",
			failedList:    "endpointslices",
			wantErrorText: `failed to list endpoint slices in namespace "team-a"`,
			objects:       []rtclient.Object{service},
		},
		{
			name:          "Pod failure after continuation",
			failedList:    "pods",
			wantErrorText: `failed to list pods in namespace "team-a"`,
			objects:       []rtclient.Object{service, endpointSlice},
		},
	}
	listErr := errors.New("injected page failure")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageCalls := 0
			listInterceptor := interceptor.Funcs{
				List: func(ctx context.Context, c rtclient.WithWatch, list rtclient.ObjectList, opts ...rtclient.ListOption) error {
					if listResource(list) != tt.failedList {
						return c.List(ctx, list, opts...)
					}
					pageCalls++
					if pageCalls == 1 {
						list.SetContinue("next-page")
						return nil
					}
					listOptions := &rtclient.ListOptions{}
					listOptions.ApplyOptions(opts)
					if listOptions.Continue != "next-page" {
						t.Errorf("second page continue token = %q, want next-page", listOptions.Continue)
					}
					return listErr
				},
			}
			exporter := newTestExporter(t, listInterceptor, tt.objects...)

			err := exporter.LoadExposedEndpointSlicesInfo()
			if !errors.Is(err, listErr) {
				t.Fatalf("LoadExposedEndpointSlicesInfo() error = %v, want wrapped page error", err)
			}
			if !strings.Contains(err.Error(), tt.wantErrorText) {
				t.Errorf("error %q does not contain %q", err, tt.wantErrorText)
			}
			if pageCalls != 2 {
				t.Errorf("got %d pages for failed resource, want 2", pageCalls)
			}
		})
	}
}

func listResource(list rtclient.ObjectList) string {
	switch list.(type) {
	case *corev1.ServiceList:
		return "services"
	case *corev1.PodList:
		return "pods"
	case *discoveryv1.EndpointSliceList:
		return "endpointslices"
	}
	return ""
}

func newTestExporter(t *testing.T, listInterceptor interceptor.Funcs, objects ...rtclient.Object) *EndpointSlicesExporter {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding core API types to scheme: %v", err)
	}
	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding discovery API types to scheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithInterceptorFuncs(listInterceptor).Build()
	return &EndpointSlicesExporter{ClientSet: &commatrixclient.ClientSet{Client: fakeClient}}
}

func testService(name, namespace string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.ServiceSpec{Selector: selector},
	}
}

func testPod(name, namespace, app string, port int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app": app}},
		Spec: corev1.PodSpec{
			HostNetwork: true,
			Containers:  []corev1.Container{{Name: "app", Ports: []corev1.ContainerPort{{ContainerPort: port}}}},
		},
	}
}

func testEndpointSlice(name, namespace, serviceName string, port int32) *discoveryv1.EndpointSlice {
	protocol := corev1.ProtocolTCP
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{discoveryv1.LabelServiceName: serviceName},
		},
		Ports: []discoveryv1.EndpointPort{{Port: &port, Protocol: &protocol}},
	}
}
