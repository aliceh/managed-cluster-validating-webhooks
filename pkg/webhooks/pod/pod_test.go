package pod

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openshift/managed-cluster-validating-webhooks/pkg/testutils"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func createRawPodJSON(tolerations []corev1.Toleration, testid, namespace string) (string, error) {

	str := `{
				"metadata": {
					"name": "%s",
					"namespace": "%s",
					"uid": "%s"
					},
					"%s"
					"users": null
			}`

	partial, err := json.Marshal(tolerations)
	return fmt.Sprintf(str, testid, namespace, string(partial)), err
}

type podTestSuites struct {
	testID          string
	targetPod       string
	namespace       string
	username        string
	operation       v1beta1.Operation
	userGroups      []string
	tolerations     []corev1.Toleration
	shouldBeAllowed bool
}

func runPodTests(t *testing.T, tests []podTestSuites) {
	gvk := metav1.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}
	gvr := metav1.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	for _, test := range tests {
		rawObjString, err := createRawPodJSON(test.tolerations, test.testID, test.namespace)
		if err != nil {
			t.Fatalf("Couldn't create a JSON fragment %s", err.Error)
		}

		obj := runtime.RawExtension{
			Raw: []byte(rawObjString),
		}
		hook := NewWebhook()
		httprequest, err := testutils.CreateHTTPRequest(hook.GetURI(),
			test.testID, gvk, gvr, test.operation, test.username, test.userGroups, test.operation, &obj)
		if err != nil {
			t.Fatalf("Expected no error, got %s", err.Error())
		}

		response, err := testutils.SendHTTPRequest(httprequest, hook)
		if err != nil {
			t.Fatalf("Expected no error, got %s", err.Error())
		}
		if response.UID == "" {
			t.Fatalf("No tracking UID associated with the response.")
		}

		if response.Allowed != test.shouldBeAllowed {
			t.Fatalf("Mismatch: %s (groups=%s) %s %s the %s pod. Test's expectation is that the user %s", test.username, test.userGroups, testutils.CanCanNot(response.Allowed), testutils.CanCanNot(test.shouldBeAllowed))
		}
	}
}

func Test(t *testing.T) {
	tests := []podTestSuites{
		{
			testID:     "dedicated-admin-cant-tolerate-in-protected",
			namespace:  "kube-system",
			username:   "dedicated-admin",
			userGroups: []string{"system:authenticated", "dedicated-admin"},
			tolerations: []corev1.Toleration{
				{
					Key:      "toleration key name",
					Operator: corev1.TolerationOpEqual,
					Value:    "toleration key value",
					Effect:   corev1.TaintEffectNoExecute,
				},
				{
					Key:      "toleration key name2",
					Operator: corev1.TolerationOpEqual,
					Value:    "toleration key value2",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			operation:       v1beta1.Create,
			shouldBeAllowed: false,
		},
	}
	runPodTests(t, tests)

}
