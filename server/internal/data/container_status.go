package data

import (
	"vgpu/internal/biz"

	corev1 "k8s.io/api/core/v1"
)

// Waiting reasons are an open string field. Only known failures belong in the
// error filter; an unfamiliar runtime reason must remain visible as waiting.
func isContainerWaitingError(reason string) bool {
	switch reason {
	case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull",
		"InvalidImageName", "ErrImageNeverPull", "ImageInspectError",
		"CreateContainerConfigError", "CreateContainerError", "RunContainerError",
		"PreCreateHookError", "PreStartHookError", "PostStartHookError":
		return true
	default:
		return false
	}
}

func containerWillRestart(policy corev1.RestartPolicy, exitCode int32) bool {
	switch policy {
	case corev1.RestartPolicyNever:
		return false
	case corev1.RestartPolicyOnFailure:
		return exitCode != 0
	default:
		// The API defaults an omitted Pod restart policy to Always.
		return true
	}
}

// classifyContainerStatus describes one regular container. Inventory membership
// and allocation accounting still use the existing Pod terminal-phase boundary.
func classifyContainerStatus(pod *corev1.Pod, observed *corev1.ContainerStatus) (string, *biz.ContainerStatusDetail) {
	detail := &biz.ContainerStatusDetail{
		PodPhase:   string(pod.Status.Phase),
		PodReason:  pod.Status.Reason,
		PodMessage: pod.Status.Message,
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			detail.PodReady = string(condition.Status)
			detail.PodReadyReason = condition.Reason
			detail.PodReadyMessage = condition.Message
			break
		}
	}
	if observed != nil {
		ready := observed.Ready
		detail.Ready = &ready
		detail.RestartCount = observed.RestartCount
		if previous := observed.LastTerminationState.Terminated; previous != nil {
			exitCode := previous.ExitCode
			detail.LastTerminationReason = previous.Reason
			detail.LastTerminationMessage = previous.Message
			detail.LastExitCode = &exitCode
		}
		switch {
		case observed.State.Running != nil:
			detail.ContainerState = "Running"
		case observed.State.Terminated != nil:
			terminated := observed.State.Terminated
			exitCode := terminated.ExitCode
			detail.ContainerState = "Terminated"
			detail.Reason = terminated.Reason
			detail.Message = terminated.Message
			detail.ExitCode = &exitCode
		case observed.State.Waiting != nil:
			detail.ContainerState = "Waiting"
			detail.Reason = observed.State.Waiting.Reason
			detail.Message = observed.State.Waiting.Message
		default:
			// ContainerState defaults to Waiting when no member is specified.
			detail.ContainerState = "Waiting"
		}
	}

	// Node loss can leave stale Running/Ready data in containerStatuses. Do not
	// present that observation as healthy, or imply confirmed graceful shutdown.
	if pod.Status.Phase == corev1.PodUnknown || pod.Status.Reason == "NodeLost" ||
		(detail.PodReady == string(corev1.ConditionUnknown) && detail.PodReadyReason == "NodeStatusUnknown") ||
		detail.Reason == "ContainerStatusUnknown" {
		return biz.ContainerStatusUnknown, detail
	}
	if pod.DeletionTimestamp != nil {
		return biz.ContainerStatusTerminating, detail
	}
	if observed == nil {
		if pod.Status.Phase == corev1.PodPending {
			return biz.ContainerStatusWaiting, detail
		}
		return biz.ContainerStatusUnknown, detail
	}
	switch detail.ContainerState {
	case "Running":
		if observed.Ready {
			return biz.ContainerStatusSuccess, detail
		}
		return biz.ContainerStatusNotReady, detail
	case "Waiting":
		if isContainerWaitingError(detail.Reason) {
			return biz.ContainerStatusError, detail
		}
		return biz.ContainerStatusWaiting, detail
	case "Terminated":
		detail.RestartPending = containerWillRestart(pod.Spec.RestartPolicy, *detail.ExitCode)
		if detail.RestartPending {
			if *detail.ExitCode == 0 {
				return biz.ContainerStatusWaiting, detail
			}
			return biz.ContainerStatusError, detail
		}
		if *detail.ExitCode == 0 {
			return biz.ContainerStatusClosed, detail
		}
		return biz.ContainerStatusFailed, detail
	default:
		return biz.ContainerStatusUnknown, detail
	}
}
