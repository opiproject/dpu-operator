/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"testing"

	configv1 "github.com/openshift/dpu-operator/api/v1"
	"github.com/openshift/dpu-operator/internal/images"
	"github.com/openshift/dpu-operator/pkgs/vars"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func nvidiaAdapterScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(configv1.AddToScheme(s))
	utilruntime.Must(appsv1.AddToScheme(s))
	return s
}

func TestEnsureNvidiaAdapterSkipsWhenVendorProviderAvailable(t *testing.T) {
	s := nvidiaAdapterScheme(t)
	vp := &configv1.VendorProvider{
		ObjectMeta: metav1.ObjectMeta{Name: configv1.VendorNvidia},
		Spec:       configv1.VendorProviderSpec{Vendor: configv1.VendorNvidia},
		Status: configv1.VendorProviderStatus{
			Conditions: []metav1.Condition{{
				Type:   configv1.VendorProviderConditionAvailable,
				Status: metav1.ConditionTrue,
				Reason: "Running",
			}},
		},
	}
	cfg := &configv1.DpuOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{Name: vars.DpuOperatorConfigName, Namespace: vars.Namespace},
		Spec:       configv1.DpuOperatorConfigSpec{Vendor: configv1.VendorNvidia},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(vp).WithObjects(vp, cfg).Build()
	r := NewDpuOperatorConfigReconciler(cl, images.NewDummyManager())

	if err := r.ensureNvidiaAdapter(context.Background(), cfg); err != nil {
		t.Fatalf("ensureNvidiaAdapter: %v", err)
	}

	var deploys appsv1.DeploymentList
	if err := cl.List(context.Background(), &deploys, client.InNamespace(vars.Namespace)); err != nil {
		t.Fatal(err)
	}
	if len(deploys.Items) != 0 {
		t.Fatalf("expected skip (no in-ns Deployment); got %d", len(deploys.Items))
	}
}

func TestReconcileVendorProviderSetsAvailable(t *testing.T) {
	s := nvidiaAdapterScheme(t)
	vp := &configv1.VendorProvider{
		ObjectMeta: metav1.ObjectMeta{Name: configv1.VendorNvidia},
		Spec:       configv1.VendorProviderSpec{Vendor: configv1.VendorNvidia},
		Status: configv1.VendorProviderStatus{
			Conditions: []metav1.Condition{{
				Type:   configv1.VendorProviderConditionAvailable,
				Status: metav1.ConditionTrue,
				Reason: "Running",
			}},
		},
	}
	cfg := &configv1.DpuOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{Name: vars.DpuOperatorConfigName, Namespace: vars.Namespace},
		Spec:       configv1.DpuOperatorConfigSpec{Vendor: configv1.VendorNvidia},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(vp, cfg).WithObjects(vp, cfg).Build()
	r := NewDpuOperatorConfigReconciler(cl, images.NewDummyManager())

	if err := r.reconcileVendorProvider(context.Background(), cfg); err != nil {
		t.Fatalf("reconcileVendorProvider: %v", err)
	}
	cond := metaFind(cfg, configv1.VendorProviderAvailableCondition)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("VendorProviderAvailable = %v, want True", cond)
	}
}

func TestReconcileVendorProviderMissing(t *testing.T) {
	s := nvidiaAdapterScheme(t)
	cfg := &configv1.DpuOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{Name: vars.DpuOperatorConfigName, Namespace: vars.Namespace},
		Spec:       configv1.DpuOperatorConfigSpec{Vendor: configv1.VendorNvidia},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(cfg).WithObjects(cfg).Build()
	r := NewDpuOperatorConfigReconciler(cl, images.NewDummyManager())

	err := r.reconcileVendorProvider(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when VendorProvider is missing")
	}
	cond := metaFind(cfg, configv1.VendorProviderAvailableCondition)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "ProviderMissing" {
		t.Fatalf("condition = %v, want False/ProviderMissing", cond)
	}
}

func metaFind(cfg *configv1.DpuOperatorConfig, typ string) *metav1.Condition {
	for i := range cfg.Status.Conditions {
		if cfg.Status.Conditions[i].Type == typ {
			return &cfg.Status.Conditions[i]
		}
	}
	return nil
}
