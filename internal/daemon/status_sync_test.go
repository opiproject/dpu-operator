/*
Copyright 2024.

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

package daemon

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/dpu-operator/api/v1"
	"github.com/openshift/dpu-operator/internal/daemon/plugin"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const vendorReadyCondition = "VendorReady"

func TestSyncSingleDpuCRKeepsForeignCondition(t *testing.T) {
	current := testDPU()
	current.Status.Conditions = []metav1.Condition{
		readyCondition(metav1.ConditionFalse, "NotReady", "down", 1),
		vendorReadyConditionObj(),
	}
	desired := current.DeepCopy()
	desired.Status.Conditions = []metav1.Condition{
		readyCondition(metav1.ConditionTrue, "Ready", "up", 2),
	}

	c := &recordingClient{current: current.DeepCopy()}
	updated, err := (&Daemon{client: c, log: logr.Discard()}).syncSingleDpuCR(desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("Ready change must update status")
	}
	if c.statusWrites != 1 {
		t.Fatalf("status writes = %d, want 1", c.statusWrites)
	}

	got := meta.FindStatusCondition(c.current.Status.Conditions, plugin.ReadyConditionType)
	if got == nil || got.Status != metav1.ConditionTrue || got.Reason != "Ready" || got.Message != "up" {
		t.Fatalf("Ready = %+v", got)
	}
	foreign := meta.FindStatusCondition(c.current.Status.Conditions, vendorReadyCondition)
	if foreign == nil || foreign.Status != metav1.ConditionTrue || foreign.Reason != vendorReadyCondition || foreign.Message != "card ready" {
		t.Fatalf("foreign condition = %+v", foreign)
	}
}

func TestSyncSingleDpuCRSkipsUnchangedReady(t *testing.T) {
	ready := readyCondition(metav1.ConditionTrue, "Ready", "up", 1)
	current := testDPU()
	current.Status.Conditions = []metav1.Condition{ready, vendorReadyConditionObj()}
	desired := current.DeepCopy()
	desired.Status.Conditions = []metav1.Condition{ready}

	c := &recordingClient{current: current.DeepCopy()}
	updated, err := (&Daemon{client: c, log: logr.Discard()}).syncSingleDpuCR(desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("unchanged Ready must not update status")
	}
	if c.statusWrites != 0 {
		t.Fatalf("status writes = %d, want 0", c.statusWrites)
	}
	foreign := meta.FindStatusCondition(c.current.Status.Conditions, vendorReadyCondition)
	if foreign == nil || foreign.Message != "card ready" {
		t.Fatalf("foreign condition = %+v", foreign)
	}
}

func testDPU() *configv1.DataProcessingUnit {
	return &configv1.DataProcessingUnit{
		ObjectMeta: metav1.ObjectMeta{Name: "dpu-1"},
		Spec: configv1.DataProcessingUnitSpec{
			DpuProductName: "test-dpu",
			NodeName:       "node-1",
		},
	}
}

func readyCondition(status metav1.ConditionStatus, reason, message string, generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               plugin.ReadyConditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Unix(1, 0),
	}
}

func vendorReadyConditionObj() metav1.Condition {
	return metav1.Condition{
		Type:               vendorReadyCondition,
		Status:             metav1.ConditionTrue,
		Reason:             vendorReadyCondition,
		Message:            "card ready",
		ObservedGeneration: 1,
		LastTransitionTime: metav1.Unix(1, 0),
	}
}

// recordingClient serves one DataProcessingUnit and counts status writes.
// syncSingleDpuCR only Get's the CR and updates its status on this path.
type recordingClient struct {
	client.Client
	current      *configv1.DataProcessingUnit
	statusWrites int
}

func (c *recordingClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	dpu, ok := obj.(*configv1.DataProcessingUnit)
	if !ok {
		return fmt.Errorf("get %T, want *DataProcessingUnit", obj)
	}
	if c.current == nil || c.current.Name != key.Name {
		return fmt.Errorf("dataprocessingunit %q not found", key.Name)
	}
	c.current.DeepCopyInto(dpu)
	return nil
}

func (c *recordingClient) Status() client.SubResourceWriter {
	return statusRecorder{c}
}

type statusRecorder struct{ c *recordingClient }

func (s statusRecorder) Update(_ context.Context, obj client.Object, _ ...client.SubResourceUpdateOption) error {
	dpu, ok := obj.(*configv1.DataProcessingUnit)
	if !ok {
		return fmt.Errorf("status update %T, want *DataProcessingUnit", obj)
	}
	s.c.statusWrites++
	s.c.current = dpu.DeepCopy()
	return nil
}

func (s statusRecorder) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return fmt.Errorf("unexpected status create")
}

func (s statusRecorder) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return fmt.Errorf("unexpected status patch")
}
