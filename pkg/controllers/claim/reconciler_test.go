/*
Copyright 2020 The Crossplane Authors.

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

package claim

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/claim"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/agent/pkg/resource"
)

var (
	errBoom = errors.New("boom")
	now     = metav1.Now()
	gvk     = schema.GroupVersionKind{}
)

func TestReconcile(t *testing.T) {
	type args struct {
		m      manager.Manager
		remote client.Client
		opts   []ReconcilerOption
	}
	type want struct {
		result reconcile.Result
		err    error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"LocalGetFailed": {
			reason: "An error should be returned if local claim cannot be retrieved",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
				err:    errors.Wrap(errBoom, localPrefix+errGetRequirement),
			},
		},
		"NotFound": {
			reason: "No error should be returned if local claim is gone",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
				},
			},
		},
		"RemoteGetFailed": {
			reason: "An error should be returned if remote claim cannot be retrieved",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetConditions(resource.AgentSyncError(errors.Wrap(errBoom, remotePrefix+errGetRequirement)))
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "An error should be returned if remote claim cannot be retrieved"
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						},
					},
				},
				remote: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"RemoteNotFoundAndDeleted": {
			reason: "No error should be returned if deletion is requested and the remote claim is gone",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							l := claim.New(claim.WithGroupVersionKind(gvk))
							l.SetDeletionTimestamp(&now)
							l.DeepCopyInto(obj.(*unstructured.Unstructured))
							return nil
						},
					},
				},
				remote: &test.MockClient{MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{RemoveFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return nil
					}}),
				},
			},
		},
		"RemoveFinalizerFailed": {
			reason: "Error during finalizer removal should be propagated",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							l := claim.New(claim.WithGroupVersionKind(gvk))
							l.SetDeletionTimestamp(&now)
							l.DeepCopyInto(obj.(*unstructured.Unstructured))
							return nil
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetDeletionTimestamp(&now)
							want.SetConditions(resource.AgentSyncError(errors.Wrap(errBoom, localPrefix+errRemoveFinalizer)))
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "Error during finalizer removal should be propagated"
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						},
					},
				},
				remote: &test.MockClient{
					MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, "")),
				},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{RemoveFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return errBoom
					}}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"RemoteFoundAndDeletionFailed": {
			reason: "The error should be returned if deletion call fails",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							l := claim.New(claim.WithGroupVersionKind(gvk))
							l.SetDeletionTimestamp(&now)
							l.DeepCopyInto(obj.(*unstructured.Unstructured))
							return nil
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetDeletionTimestamp(&now)
							want.SetConditions(resource.AgentSyncError(errors.Wrap(errBoom, remotePrefix+errDeleteClaim)))
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "The error should be returned if deletion call fails"
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						}},
				},
				remote: &test.MockClient{
					MockGet:    test.NewMockGetFn(nil),
					MockDelete: test.NewMockDeleteFn(errBoom),
				},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"RemoteFoundAndDeletionCalled": {
			reason: "No error should be returned when deletion is requested",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							l := claim.New(claim.WithGroupVersionKind(gvk))
							l.SetDeletionTimestamp(&now)
							l.DeepCopyInto(obj.(*unstructured.Unstructured))
							return nil
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetDeletionTimestamp(&now)
							want.SetConditions(resource.AgentSyncSuccess().WithMessage("Deletion is successfully requested"))
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "No error should be returned when deletion is requested"
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						}},
				},
				remote: &test.MockClient{
					MockGet:    test.NewMockGetFn(nil),
					MockDelete: test.NewMockDeleteFn(nil),
				},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: tinyWait},
			},
		},
		"AddFinalizerFailed": {
			reason: "An error should be returned if finalizer cannot be added",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetConditions(resource.AgentSyncError(errors.Wrap(errBoom, localPrefix+errAddFinalizer)))
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "An error should be returned if finalizer cannot be added"
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						},
					},
				},
				remote: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{AddFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return errBoom
					}}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"PropagatorFailed": {
			reason: "An error should be returned if propagator fails",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetConditions(resource.AgentSyncError(errors.Wrap(errBoom, errPush)))
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "An error should be returned if propagator fails"
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						},
					},
				},
				remote: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{AddFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return nil
					}}),
					WithPropagator(PropagateFn(func(_ context.Context, _, _ *claim.Unstructured) error {
						return errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"Successful": {
			reason: "No error should be returned if everything goes well.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							want := claim.New(claim.WithGroupVersionKind(gvk))
							want.SetConditions(resource.AgentSyncSuccess())
							if diff := cmp.Diff(want.GetUnstructured(), obj, test.EquateConditions()); diff != "" {
								reason := "No error should be returned if everything goes well."
								t.Errorf("\nReason: %s\n-want, +got:\n%s", reason, diff)
							}
							return nil
						},
					},
				},
				remote: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				opts: []ReconcilerOption{
					WithFinalizer(runtimeresource.FinalizerFns{AddFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return nil
					}}),
					WithPropagator(PropagateFn(func(_ context.Context, _, _ *claim.Unstructured) error {
						return nil
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: longWait},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewReconciler(tc.args.m, tc.args.remote, gvk, tc.args.opts...)
			got, err := r.Reconcile(reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nr.Reconcile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("\nReason: %s\nr.Reconcile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
