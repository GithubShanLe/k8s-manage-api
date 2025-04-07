package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s-manage-api/handlers"
	"k8s-manage-api/k8s"
	"net/http"
	"strconv"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListPodRequest struct {
	PodName   string `json:"podName"`
	NameSpace string `json:"namespace"`
}

type ListPodResponse struct {
	handlers.ErrorResponse
	Pods []Pod `json:"pods"`
}

type Pod struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Images     []string          `json:"images"`
	Cpu        string            `json:"cpu"`
	Mem        string            `json:"mem"`
	Restart    int32             `json:"restart"`
	CreateTime string            `json:"createTime"`
	Yaml       string            `json:"yaml"`
	Status     string            `json:"status"`
	ContainersState string            `json:"containersState"`
}

func ListPod(w http.ResponseWriter, r *http.Request) {
	clientset := k8s.GetClient()
	var resp ListPodResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req ListPodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}

	listOptions := metav1.ListOptions{}
	if req.PodName != "" {
		listOptions.FieldSelector = fmt.Sprintf("metadata.name=%s", req.PodName)
	}
	pods, err := clientset.CoreV1().Pods(req.NameSpace).List(context.Background(), listOptions)
	if err != nil {
		resp.ErrorCode = "500"
		resp.ErrorMessage = fmt.Sprintf("获取svc列表失败: %v", err)
		return
	}
	for _, pod := range pods.Items {
		pod.APIVersion = "v1"
		pod.Kind = "Pod"
		pod.ManagedFields = nil
		// 转换为 YAML
		yamlData,err:=json.Marshal(pod)
		if err != nil {
			resp.ErrorCode = "500"
			resp.ErrorMessage = fmt.Sprintf("转换YAML失败: %v", err)
			return
		}
		cr, _, restart := Statuses(pod.Status.ContainerStatuses)

		resp.Pods = append(resp.Pods, Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    pod.Labels,
			Images: func() []string {
				var images []string
				for _, container := range pod.Spec.Containers {
					images = append(images, container.Image)
				}
				return images
			}(),
			CreateTime: pod.CreationTimestamp.Format("2006-01-02 15:04:05"),
			Cpu: func() string {
				var (
					requestCpu int64
					limitCpu   int64
				)

				for _, container := range pod.Spec.Containers {
					requestCpu += container.Resources.Requests.Cpu().MilliValue()
					limitCpu += container.Resources.Limits.Cpu().MilliValue()
				}
				return fmt.Sprintf("%d/%d", requestCpu, limitCpu)
			}(),
			Mem: func() string {
				var (
					limitMem   int64
					requestMem int64
				)
				for _, container := range pod.Spec.Containers {
					limitMem += container.Resources.Limits.Memory().Value() / 1024 / 1024
					requestMem += container.Resources.Requests.Memory().Value() / 1024 / 1024
				}
				return fmt.Sprintf("%d/%d", requestMem, limitMem)
			}(),
			Restart: int32(restart),
			ContainersState: func() string {
				return 		strconv.Itoa(cr) + "/" + strconv.Itoa(len(pod.Spec.Containers))
			}(),
			Status: Phase(&pod),
			Yaml: string(yamlData),
		})
	}
	return
}

type GetPodMetricRequest struct {
	PodName   string `json:"PodName"`
	NameSpace string `json:"namespace"`
}

type GetPodMetricsponse struct {
	handlers.ErrorResponse
	Metric Metric `json:"metric"`
}
type Metric struct {
	CpuLimitRate      string `json:"cpuLimitRate"`
	CpuRequestRate    string `json:"cpuRequestRate"`
	MemoryLimitRate   string `json:"memoryLimitRate"`
	MemoryReuqestRate string `json:"memoryReuqestRate"`
}

func GetPodMetric(w http.ResponseWriter, r *http.Request) {

	var resp GetPodMetricsponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	// 解析请求参数
	var req GetPodMetricRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}
	clientset := k8s.GetClient()
	pod, err := clientset.CoreV1().Pods(req.NameSpace).Get(context.TODO(), req.PodName, metav1.GetOptions{})
	if err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("get pod err: %s", err.Error())
		return
	}
	// 获取cpu的请求值
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit int64
	for _, containers := range pod.Spec.Containers {
		cpuRequest += containers.Resources.Requests.Cpu().MilliValue()
		cpuLimit += containers.Resources.Limits.Cpu().MilliValue()
		memoryRequest += containers.Resources.Requests.Memory().MilliValue()
		memoryLimit += containers.Resources.Limits.Memory().MilliValue()
	}

	getOptions := metav1.GetOptions{}
	metricClientset := k8s.GetMetricClient()
	result, err := metricClientset.MetricsV1beta1().PodMetricses(req.NameSpace).Get(context.TODO(), req.PodName, getOptions)
	if err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("get podMetricses err: %s", err.Error())
		return
	}
	var (
		memoryLimitRate, memoryReuqestRate string
		cpuLimitRate, cpuRequestRate       string
		cpu, mem                           int64
	)

	for _, container := range result.Containers {
		cpu += container.Usage.Cpu().MilliValue()
		mem += container.Usage.Memory().MilliValue()
	}
	// 处理分母为0的情况，返回N/A
	if cpuLimit == 0 {
		cpuLimitRate = "N/A"
	} else {
		cpuLimitRate = fmt.Sprintf("%.1f", (float64(cpu)/float64(cpuLimit))*100)
	}
	fmt.Println(cpuLimit, cpuRequest, memoryRequest, memoryLimit, cpu, mem)
	if cpuRequest == 0 {
		cpuRequestRate = "N/A"
	} else {
		cpuRequestRate = fmt.Sprintf("%.1f", (float64(cpu)/float64(cpuRequest))*100)
	}

	if memoryLimit == 0 {
		memoryLimitRate = "N/A"
	} else {
		memoryLimitRate = fmt.Sprintf("%.1f", (float64(mem)/float64(memoryLimit))*100)
	}

	if memoryRequest == 0 {
		memoryReuqestRate = "N/A"
	} else {
		memoryReuqestRate = fmt.Sprintf("%.1f", (float64(mem)/float64(memoryRequest))*100)
	}
	resp.Metric = Metric{
		CpuLimitRate:      cpuLimitRate,
		CpuRequestRate:    cpuRequestRate,
		MemoryLimitRate:   memoryLimitRate,
		MemoryReuqestRate: memoryReuqestRate,
	}
	return
}



func Statuses(ss []v1.ContainerStatus) (cr, ct, rc int) {
	for _, c := range ss {
		if c.State.Terminated != nil {
			ct++
		}
		if c.Ready {
			cr = cr + 1
		}
		rc += int(c.RestartCount)
	}

	return
}

// Phase reports the given pod phase.
func  Phase(po *v1.Pod) string {
	status := string(po.Status.Phase)
	if po.Status.Reason != "" {
		if po.DeletionTimestamp != nil && po.Status.Reason == "NodeLost" {
			return "Unknown"
		}
		status = po.Status.Reason
	}

	status, ok := initContainerPhase(po.Status, len(po.Spec.InitContainers), status)
	if ok {
		return status
	}

	status, ok = containerPhase(po.Status, status)
	if ok && status == Completed {
		status = Running
	}
	if po.DeletionTimestamp == nil {
		return status
	}

	return Terminating
}

func containerPhase(st v1.PodStatus, status string) (string, bool) {
	var running bool
	for i := len(st.ContainerStatuses) - 1; i >= 0; i-- {
		cs := st.ContainerStatuses[i]
		switch {
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
			status = cs.State.Waiting.Reason
		case cs.State.Terminated != nil && cs.State.Terminated.Reason != "":
			status = cs.State.Terminated.Reason
		case cs.State.Terminated != nil:
			if cs.State.Terminated.Signal != 0 {
				status = "Signal:" + strconv.Itoa(int(cs.State.Terminated.Signal))
			} else {
				status = "ExitCode:" + strconv.Itoa(int(cs.State.Terminated.ExitCode))
			}
		case cs.Ready && cs.State.Running != nil:
			running = true
		}
	}

	return status, running
}

func  initContainerPhase(st v1.PodStatus, initCount int, status string) (string, bool) {
	for i, cs := range st.InitContainerStatuses {
		s := checkContainerStatus(cs, i, initCount)
		if s == "" {
			continue
		}
		return s, true
	}

	return status, false
}

// ----------------------------------------------------------------------------
// Helpers..

func checkContainerStatus(cs v1.ContainerStatus, i, initCount int) string {
	switch {
	case cs.State.Terminated != nil:
		if cs.State.Terminated.ExitCode == 0 {
			return ""
		}
		if cs.State.Terminated.Reason != "" {
			return "Init:" + cs.State.Terminated.Reason
		}
		if cs.State.Terminated.Signal != 0 {
			return "Init:Signal:" + strconv.Itoa(int(cs.State.Terminated.Signal))
		}
		return "Init:ExitCode:" + strconv.Itoa(int(cs.State.Terminated.ExitCode))
	case cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing":
		return "Init:" + cs.State.Waiting.Reason
	default:
		return "Init:" + strconv.Itoa(i) + "/" + strconv.Itoa(initCount)
	}
}



const (
	// Terminating represents a pod terminating status.
	Terminating = "Terminating"

	// Running represents a pod running status.
	Running = "Running"

	// Initialized represents a pod initialized status.
	Initialized = "Initialized"

	// Completed represents a pod completed status.
	Completed = "Completed"

	// ContainerCreating represents a pod container status.
	ContainerCreating = "ContainerCreating"

	// PodInitializing represents a pod initializing status.
	PodInitializing = "PodInitializing"

	// Pending represents a pod pending status.
	Pending = "Pending"

	// Blank represents no value.
	Blank = ""
)


type DeletePodRequest struct {
	PodName   string `json:"podName"`
	NameSpace string `json:"namespace"`
}

type DeletePodResponse struct {
	handlers.ErrorResponse
}

func DeletePod(w http.ResponseWriter, r *http.Request) {

	var resp DeletePodResponse
	defer func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}()

	fmt.Println("delete pod")

	// 解析请求参数
	var req DeletePodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("解析请求失败: %v", err)
		return
	}
	fmt.Println("delete pod",req)
	
	clientset := k8s.GetClient()
	pod, err := clientset.CoreV1().Pods(req.NameSpace).Get(context.TODO(), req.PodName, metav1.GetOptions{})
	if err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("get pod err: %s", err.Error())
		return
	}
	err=clientset.CoreV1().Pods(req.NameSpace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
	if err != nil {
		resp.ErrorCode = "400"
		resp.ErrorMessage = fmt.Sprintf("get pod err: %s", err.Error())
		return
	}
	return
}