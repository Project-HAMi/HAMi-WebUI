package data

import (
	"context"
	"testing"

	"vgpu/internal/biz"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

func runningContainer(ready bool) *corev1.ContainerStatus {
	return &corev1.ContainerStatus{Name: "worker", Ready: ready, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
}

func waitingContainer(reason string) *corev1.ContainerStatus {
	return &corev1.ContainerStatus{Name: "worker", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}}
}

func terminatedContainer(code int32) *corev1.ContainerStatus {
	return &corev1.ContainerStatus{Name: "worker", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: code}}}
}

func TestClassifyContainerStatus(t *testing.T) {
	tests := []struct {
		name           string
		phase          corev1.PodPhase
		policy         corev1.RestartPolicy
		observed       *corev1.ContainerStatus
		modifyPod      func(*corev1.Pod)
		want           string
		restartPending bool
	}{
		{name: "running and ready", observed: runningContainer(true), want: biz.ContainerStatusSuccess},
		{name: "running without readiness", observed: runningContainer(false), want: biz.ContainerStatusNotReady},
		{name: "container state is not replaced by Pod phase", phase: corev1.PodPending, observed: runningContainer(true), want: biz.ContainerStatusSuccess},
		{name: "another container or readiness gate is not ready", observed: runningContainer(true), modifyPod: func(p *corev1.Pod) {
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ReadinessGatesNotReady"}}
		}, want: biz.ContainerStatusSuccess},
		{name: "creating container", observed: waitingContainer("ContainerCreating"), want: biz.ContainerStatusWaiting},
		{name: "initializing Pod", observed: waitingContainer("PodInitializing"), want: biz.ContainerStatusWaiting},
		{name: "crash loop", observed: waitingContainer("CrashLoopBackOff"), want: biz.ContainerStatusError},
		{name: "image pull backoff", observed: waitingContainer("ImagePullBackOff"), want: biz.ContainerStatusError},
		{name: "image pull failure", observed: waitingContainer("ErrImagePull"), want: biz.ContainerStatusError},
		{name: "container configuration failure", observed: waitingContainer("CreateContainerConfigError"), want: biz.ContainerStatusError},
		{name: "unfamiliar runtime reason is not guessed from words", observed: waitingContainer("RuntimeErrorRecoveryPending"), want: biz.ContainerStatusWaiting},
		{name: "empty ContainerState defaults to waiting", observed: &corev1.ContainerStatus{Name: "worker"}, want: biz.ContainerStatusWaiting},
		{name: "no status yet on assigned pending Pod", phase: corev1.PodPending, want: biz.ContainerStatusWaiting},
		{name: "no status on running Pod", want: biz.ContainerStatusUnknown},
		{name: "completed container with no restart", policy: corev1.RestartPolicyNever, observed: terminatedContainer(0), want: biz.ContainerStatusClosed},
		{name: "failed container with no restart", policy: corev1.RestartPolicyNever, observed: terminatedContainer(1), want: biz.ContainerStatusFailed},
		{name: "on failure leaves successful container complete", policy: corev1.RestartPolicyOnFailure, observed: terminatedContainer(0), want: biz.ContainerStatusClosed},
		{name: "on failure awaits restart after failure", policy: corev1.RestartPolicyOnFailure, observed: terminatedContainer(1), want: biz.ContainerStatusError, restartPending: true},
		{name: "always restarts successful execution", policy: corev1.RestartPolicyAlways, observed: terminatedContainer(0), want: biz.ContainerStatusWaiting, restartPending: true},
		{name: "always restarts failed execution", policy: corev1.RestartPolicyAlways, observed: terminatedContainer(1), want: biz.ContainerStatusError, restartPending: true},
		{name: "omitted restart policy defaults to always", observed: terminatedContainer(0), want: biz.ContainerStatusWaiting, restartPending: true},
		{name: "deleting ready container", observed: runningContainer(true), modifyPod: func(p *corev1.Pod) {
			now := metav1.Now()
			p.DeletionTimestamp = &now
		}, want: biz.ContainerStatusTerminating},
		{name: "unknown Pod phase overrides stale ready observation", phase: corev1.PodUnknown, observed: runningContainer(true), want: biz.ContainerStatusUnknown},
		{name: "lost node overrides deletion and stale ready", observed: runningContainer(true), modifyPod: func(p *corev1.Pod) {
			now := metav1.Now()
			p.DeletionTimestamp = &now
			p.Status.Reason = "NodeLost"
		}, want: biz.ContainerStatusUnknown},
		{name: "unreachable node readiness condition", observed: runningContainer(true), modifyPod: func(p *corev1.Pod) {
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionUnknown, Reason: "NodeStatusUnknown"}}
		}, want: biz.ContainerStatusUnknown},
		{name: "runtime could not find container", policy: corev1.RestartPolicyNever, observed: &corev1.ContainerStatus{State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{Reason: "ContainerStatusUnknown", ExitCode: 137},
		}}, want: biz.ContainerStatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase := tt.phase
			if phase == "" {
				phase = corev1.PodRunning
			}
			pod := &corev1.Pod{Spec: corev1.PodSpec{RestartPolicy: tt.policy}, Status: corev1.PodStatus{Phase: phase}}
			if tt.modifyPod != nil {
				tt.modifyPod(pod)
			}
			got, detail := classifyContainerStatus(pod, tt.observed)
			if got != tt.want || detail.RestartPending != tt.restartPending {
				t.Fatalf("classification = %q, restartPending=%v; want %q, %v", got, detail.RestartPending, tt.want, tt.restartPending)
			}
			if tt.observed == nil && detail.Ready != nil {
				t.Fatalf("missing observation must not invent readiness: %#v", detail)
			}
		})
	}
}

func TestContainerStatusDetailsKeepEvidenceSeparateFromCurrentState(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase:      corev1.PodRunning,
		Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady", Message: "containers with unready status: [sidecar]"}},
	}}
	observed := runningContainer(true)
	observed.RestartCount = 2
	observed.LastTerminationState.Terminated = &corev1.ContainerStateTerminated{Reason: "OOMKilled", Message: "previous execution", ExitCode: 137}
	status, detail := classifyContainerStatus(pod, observed)
	if status != biz.ContainerStatusSuccess || detail.Reason != "" || detail.Message != "" {
		t.Fatalf("past failure must not replace current running state: %s, %#v", status, detail)
	}
	if detail.Ready == nil || !*detail.Ready || detail.PodReady != "False" || detail.PodReadyReason != "ContainersNotReady" {
		t.Fatalf("container and Pod readiness were conflated: %#v", detail)
	}
	if detail.RestartCount != 2 || detail.LastTerminationReason != "OOMKilled" || detail.LastExitCode == nil || *detail.LastExitCode != 137 {
		t.Fatalf("last termination evidence missing: %#v", detail)
	}

	observed = terminatedContainer(0)
	observed.State.Terminated.Reason = "Completed"
	observed.State.Terminated.Message = "completed all work"
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	_, detail = classifyContainerStatus(pod, observed)
	if detail.Ready == nil || *detail.Ready || detail.ExitCode == nil || *detail.ExitCode != 0 || detail.Reason != "Completed" || detail.Message != "completed all work" {
		t.Fatalf("false/zero or original termination evidence missing: %#v", detail)
	}
}

func TestPodInventoryKeepsContainerOutcomesUntilWholePodTerminates(t *testing.T) {
	const allocationKey = "hami.io/vgpu-devices-allocated"
	previous, existed := util.SupportDevices["NVIDIA"]
	util.SupportDevices["NVIDIA"] = allocationKey
	t.Cleanup(func() {
		if existed {
			util.SupportDevices["NVIDIA"] = previous
		} else {
			delete(util.SupportDevices, "NVIDIA")
		}
	})
	for _, terminalPhase := range []corev1.PodPhase{corev1.PodSucceeded, corev1.PodFailed} {
		t.Run(string(terminalPhase), func(t *testing.T) {
			repo := &podRepo{pods: map[k8stypes.UID]*biz.PodInfo{}, log: log.NewHelper(log.DefaultLogger)}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "training", UID: "pod-1", Annotations: map[string]string{
					util.AssignedNodeAnnotations: "node-1", allocationKey: "GPU-1,NVIDIA,1024,10:;GPU-1,NVIDIA,1024,10:;",
				}},
				Spec:   corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "worker"}, {Name: "sidecar"}}},
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			}
			repo.onAddPod(pod)
			containers, _ := repo.ListAll(context.Background())
			if len(containers) != 2 || containers[0].Status != biz.ContainerStatusWaiting {
				t.Fatalf("assigned Pending containers should be waiting, not empty status: %#v", containers)
			}
			for _, outcome := range []struct {
				exitCode int32
				status   string
			}{{0, biz.ContainerStatusClosed}, {1, biz.ContainerStatusFailed}} {
				updated := pod.DeepCopy()
				updated.Status.Phase = corev1.PodRunning
				worker := terminatedContainer(outcome.exitCode)
				sidecar := runningContainer(true)
				sidecar.Name = "sidecar"
				updated.Status.ContainerStatuses = []corev1.ContainerStatus{*sidecar, *worker}
				repo.onUpdatePod(pod, updated)
				containers, _ = repo.ListAll(context.Background())
				if len(containers) != 2 || containers[0].Name != "worker" || containers[0].Status != outcome.status || containers[1].Status != biz.ContainerStatusSuccess {
					t.Fatalf("container state must match names, with active Pod outcomes retained: %#v", containers)
				}
				count, cores, memory, _ := biz.ContainersStatisticsInfo(containers, "")
				if count != 2 || cores != 20 || memory != 2048 {
					t.Fatalf("state classification changed active allocation accounting: %d, %d, %d", count, cores, memory)
				}
				pod = updated
			}
			terminal := pod.DeepCopy()
			terminal.Status.Phase = terminalPhase
			workerExit := int32(0)
			if terminalPhase == corev1.PodFailed {
				workerExit = 1
			}
			worker := terminatedContainer(workerExit)
			sidecar := terminatedContainer(0)
			sidecar.Name = "sidecar"
			terminal.Status.ContainerStatuses = []corev1.ContainerStatus{*worker, *sidecar}
			repo.onUpdatePod(pod, terminal)
			containers, _ = repo.ListAll(context.Background())
			if len(containers) != 0 {
				t.Fatalf("terminal Pod must leave active inventory: %#v", containers)
			}
			count, cores, memory, _ := biz.ContainersStatisticsInfo(containers, "")
			if count != 0 || cores != 0 || memory != 0 {
				t.Fatal("terminal Pod allocations are still counted")
			}
			repo.onAddPod(terminal)
			if len(repo.pods) != 0 {
				t.Fatal("initial informer listing must not reintroduce terminal Pods")
			}
			unassigned := pod.DeepCopy()
			unassigned.UID = "unassigned"
			unassigned.Annotations = nil
			repo.onAddPod(unassigned)
			if len(repo.pods) != 0 {
				t.Fatal("status changes must not add unassigned Pods to inventory")
			}
		})
	}
}
