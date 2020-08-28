package pod

import (
	"fmt"
	"testing"

	"github.com/openshift/managed-cluster-validating-webhooks/pkg/testutils"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Raw JSON for a Namespace, used as runtime.RawExtension, and represented here
// because sometimes we need it for OldObject as well as Object.
const testPodRaw string = `{
  "metadata": {
    "name": "%s",
    "uid": "%s",
    "creationTimestamp": "2020-05-10T07:51:00Z"
  },
  "users": null
}`

type podTestSuites struct {
	testID          string
	targetPod       string
	username        string
	userGroups      []string
	oldObject       *runtime.RawExtension
	operation       v1beta1.Operation
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
		rawObjString := fmt.Sprintf(testPodRaw, test.targetPod, test.testID)
		obj := runtime.RawExtension{
			Raw: []byte(rawObjString),
		}
		hook := NewWebhook()
		httprequest, err := testutils.CreateHTTPRequest(hook.GetURI(),
			test.testID,
			gvk, gvr, test.operation, test.username, test.userGroups, &obj, test.oldObject)
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
			t.Fatalf("Mismatch: %s (groups=%s) %s %s the %s pod. Test's expectation is that the user %s", test.username, test.userGroups, testutils.CanCanNot(response.Allowed), string(test.operation), test.targetNamespace, testutils.CanCanNot(test.shouldBeAllowed))
		}
	}
}
