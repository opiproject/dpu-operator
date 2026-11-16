package sfcreconciler

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/dpu-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// recordingClient serves one ServiceFunctionChain and records created pods.
type recordingClient struct {
	sfc  *configv1.ServiceFunctionChain
	pods []*corev1.Pod
}

func (c *recordingClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	switch dst := obj.(type) {
	case *configv1.ServiceFunctionChain:
		if c.sfc == nil || c.sfc.Name != key.Name {
			return apierrors.NewNotFound(schema.GroupResource{Resource: "servicefunctionchains"}, key.Name)
		}
		c.sfc.DeepCopyInto(dst)
		return nil
	default:
		return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, key.Name)
	}
}

func (c *recordingClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return nil
}

func (c *recordingClient) Create(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return apierrors.NewBadRequest("expected a pod")
	}
	c.pods = append(c.pods, pod.DeepCopy())
	return nil
}

func (c *recordingClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return nil
}
func (c *recordingClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return nil
}
func (c *recordingClient) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	return nil
}
func (c *recordingClient) DeleteAllOf(context.Context, client.Object, ...client.DeleteAllOfOption) error {
	return nil
}
func (c *recordingClient) Status() client.SubResourceWriter            { return nil }
func (c *recordingClient) SubResource(string) client.SubResourceClient { return nil }
func (c *recordingClient) Scheme() *runtime.Scheme                     { return nil }
func (c *recordingClient) RESTMapper() meta.RESTMapper                 { return nil }
func (c *recordingClient) GroupVersionKindFor(runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (c *recordingClient) IsObjectNamespaced(runtime.Object) (bool, error) { return true, nil }

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := configv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func reconcileSFC(t *testing.T, sfc *configv1.ServiceFunctionChain) []*corev1.Pod {
	t.Helper()
	rec := &recordingClient{sfc: sfc}
	r := &SfcReconciler{Client: rec, Scheme: testScheme(t), nodeName: "node-a"}
	ctx := logr.NewContext(context.Background(), logr.Discard())
	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sfc)}); err != nil {
		t.Fatal(err)
	}
	return rec.pods
}

func TestChartNetworkFunctionCreatesNoPod(t *testing.T) {
	pods := reconcileSFC(t, &configv1.ServiceFunctionChain{
		ObjectMeta: metav1.ObjectMeta{Name: "chain", Namespace: "openshift-dpu-operator"},
		Spec: configv1.ServiceFunctionChainSpec{NetworkFunctions: []configv1.NetworkFunction{{
			Name:  "firewall",
			Chart: &configv1.HelmChartSource{Repository: "oci://example.com/charts", Name: "firewall", Version: "1.0.0"},
		}}},
	})
	if len(pods) != 0 {
		t.Fatalf("chart network function created %d pods", len(pods))
	}
}

func TestImageNetworkFunctionCreatesPod(t *testing.T) {
	pods := reconcileSFC(t, &configv1.ServiceFunctionChain{
		ObjectMeta: metav1.ObjectMeta{Name: "chain", Namespace: "openshift-dpu-operator"},
		Spec: configv1.ServiceFunctionChainSpec{NetworkFunctions: []configv1.NetworkFunction{
			{Name: "from-chart", Chart: &configv1.HelmChartSource{Repository: "oci://example.com/charts", Name: "fw", Version: "1.0.0"}},
			{Name: "from-image", Image: "example.com/nf:1"},
		}},
	})
	if len(pods) != 1 {
		t.Fatalf("created %d pods, want the image network function only", len(pods))
	}
	if pods[0].Name != "from-image" || pods[0].Spec.Containers[0].Image != "example.com/nf:1" {
		t.Fatalf("pod = %s image %s", pods[0].Name, pods[0].Spec.Containers[0].Image)
	}
}
