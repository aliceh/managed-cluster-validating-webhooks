package pod

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"

	responsehelper "github.com/openshift/managed-cluster-validating-webhooks/pkg/helpers"
	"github.com/openshift/managed-cluster-validating-webhooks/pkg/webhooks/utils"
	"k8s.io/api/admission/v1beta1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	admissionctl "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	WebhookName         string = "pod-validation"
	privilegedNamespace string = `(^kube$|^kube-.*|^openshift$|^openshift-.*|^default$|^redhat-.*)`	                               
	exceptionNamespace string = `(openshift-logging|openshift-operators)`
)

var (
	privilegedNamespaceRe = regexp.MustCompile(privilegedNamespace)
	exceptionNamespaceRe = regexp.MustCompile(exceptionNamespace)
	log                   = logf.Log.WithName(WebhookName)

	scope = admissionregv1.NamespacedScope
	rules = []admissionregv1.RuleWithOperations{
		{
			Operations: []admissionregv1.OperationType{admissionregv1.OperationAll},
			Rule: admissionregv1.Rule{
				APIGroups:   []string{"v1"},
				APIVersions: []string{"*"},
				Resources:   []string{"pods"},
				Scope:       &scope,
			},
		},
	}
)

type PodWebhook struct {
	mu sync.Mutex
	s  runtime.Scheme
}

// TimeoutSeconds implements Webhook interface
func (s *PodWebhook) TimeoutSeconds() int32 { return 1 }

// MatchPolicy implements Webhook interface
func (s *PodWebhook) MatchPolicy() admissionregv1.MatchPolicyType {
	return admissionregv1.Equivalent
}

// Name implements Webhook interface
func (s *PodWebhook) Name() string { return WebhookName }

// FailurePolicy implements Webhook interface
func (s *PodWebhook) FailurePolicy() admissionregv1.FailurePolicyType {
	return admissionregv1.Ignore
}

// Rules implements Webhook interface
func (s *PodWebhook) Rules() []admissionregv1.RuleWithOperations { return rules }

// GetURI implements Webhook interface
func (s *PodWebhook) GetURI() string { return "/" + WebhookName }

// SideEffects implements Webhook interface
func (s *PodWebhook) SideEffects() admissionregv1.SideEffectClass {
	return admissionregv1.SideEffectClassNone
}

// Validate implements Webhook interface
func (s *PodWebhook) Validate(req admissionctl.Request) bool {
	valid := true
	valid = valid && (req.UserInfo.Username != "")
	valid = valid && (req.Kind.Kind == "Pod")

	return valid
}

func (s *PodWebhook) renderPod(req admissionctl.Request) (*corev1.Pod, error) {
	decoder, err := admissionctl.NewDecoder(&s.s)
	if err != nil {
		return nil, err
	}
	pod := &corev1.Pod{}
	if len(req.OldObject.Raw) > 0 {
		err = decoder.DecodeRaw(req.OldObject, pod)
	} else {
		err = decoder.DecodeRaw(req.Object, pod)
	}
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func (s *PodWebhook) authorized(request admissionctl.Request) admissionctl.Response {
	var ret admissionctl.Response
	pod, err := s.renderPod(request)
	if err != nil {
		log.Error(err, "Couldn't render a Pod from the incoming request")
		return admissionctl.Errored(http.StatusBadRequest, err)
	}
// Pod will not be allowed to deploy on infra/master if it is outside of privilegedNamespace or in an exceptionNamespace
	if !privilegedNamespaceRe.Match([]byte(pod.ObjectMeta.GetNamespace())) || exceptionNamespaceRe.Match([]byte(pod.ObjectMeta.GetNamespace())){
		for _, toleration := range pod.Spec.Tolerations {
			if toleration.Key == "node-role.kubernetes.io/infra" && toleration.Effect == corev1.TaintEffectNoSchedule {
				ret = admissionctl.Denied("Not allowed to schedule a pod with NoSchedule taint on infra node")
				ret.UID = request.AdmissionRequest.UID
				return ret
			}
			if toleration.Key == "node-role.kubernetes.io/infra" && toleration.Effect == corev1.TaintEffectPreferNoSchedule {
				ret = admissionctl.Denied("Not allowed to schedule a pod with PreferNoSchedule taint on infra node")
				ret.UID = request.AdmissionRequest.UID
				return ret
			}
			if toleration.Key == "node-role.kubernetes.io/master" && toleration.Effect == corev1.TaintEffectNoSchedule {
				ret = admissionctl.Denied("Not allowed to schedule a pod with NoSchedule taint on master node")
				ret.UID = request.AdmissionRequest.UID
				return ret
			}
			if toleration.Key == "node-role.kubernetes.io/master" && toleration.Effect == corev1.TaintEffectPreferNoSchedule {
				ret = admissionctl.Denied("Not allowed to schedule a pod with PreferNoSchedule taint on master node")
				ret.UID = request.AdmissionRequest.UID
				return ret
			}

	} 
	
	// Hereafter, all requests are controlled by RBAC

	ret = admissionctl.Allowed("Allowed to create a Pod in a privileged Namespace because of RBAC")
	ret.UID = request.AdmissionRequest.UID
	return ret
}

// HandleRequest Decide if the incoming request is allowed
func (s *PodWebhook) HandleRequest(w http.ResponseWriter, r *http.Request) {

	s.mu.Lock()
	defer s.mu.Unlock()
	request, _, err := utils.ParseHTTPRequest(r)
	if err != nil {
		log.Error(err, "Error parsing HTTP Request Body")
		responsehelper.SendResponse(w, admissionctl.Errored(http.StatusBadRequest, err))
		return
	}
	// Is this a valid request?
	if !s.Validate(request) {
		responsehelper.SendResponse(w,
			admissionctl.Errored(http.StatusBadRequest,
				fmt.Errorf("Could not parse Namespace from request")))
		return
	}
	// should the request be authorized?

	responsehelper.SendResponse(w, s.authorized(request))
}

// NewWebhook creates a new webhook
func NewWebhook() *PodWebhook {
	scheme := runtime.NewScheme()
	v1beta1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)

	return &PodWebhook{
		s: *scheme,
	}
}
