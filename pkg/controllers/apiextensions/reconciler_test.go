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

package apiextensions

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/crossplane/crossplane/apis/apiextensions/v1alpha1"
)

var (
	errBoom = errors.New("boom")

	nl = func() runtime.Object { return &v1alpha1.CompositionList{} }
	gi = func(l runtime.Object) []runtimeresource.Object {
		list, _ := l.(*v1alpha1.CompositionList)
		result := make([]runtimeresource.Object, len(list.Items))
		for i, val := range list.Items {
			obj, _ := val.DeepCopyObject().(runtimeresource.Object)
			result[i] = obj
		}
		return result
	}
	ni = func() runtimeresource.Object { return &v1alpha1.Composition{} }

	established = apiextensions.CustomResourceDefinition{
		Status: apiextensions.CustomResourceDefinitionStatus{
			Conditions: []apiextensions.CustomResourceDefinitionCondition{
				{
					Type:   apiextensions.Established,
					Status: apiextensions.ConditionTrue,
				},
			},
		},
	}
)

func Test_Reconcile(t *testing.T) {
	type args struct {
		m     manager.Manager
		local runtimeresource.ClientApplicator
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
		"GetCRDFailed": {
			reason: "An error should be returned if CRD in local cannot be retrieved",
			args: args{
				m: &fake.Manager{},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
				},
			},
			want: want{
				err:    errors.Wrap(errBoom, localPrefix+errGetCRD),
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"NotEstablishedYet": {
			reason: "No error should be returned if CRD in local is not established yet",
			args: args{
				m: &fake.Manager{},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: tinyWait},
			},
		},
		"RemoteGetFailed": {
			reason: "An error should be returned if the instance in remote cluster cannot be retrieved",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(errBoom),
					},
				},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							o := obj.(*apiextensions.CustomResourceDefinition)
							established.DeepCopyInto(o)
							return nil
						},
					},
				},
			},
			want: want{
				err:    errors.Wrap(errBoom, remotePrefix+fmt.Sprintf(errFmtGetInstance, compositionCRDName)),
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"LocalApplyFailed": {
			reason: "An error should be returned if local Apply operation fails",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
					},
				},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							if o, ok := obj.(*apiextensions.CustomResourceDefinition); ok {
								established.DeepCopyInto(o)
							}
							return nil
						},
					},
					Applicator: runtimeresource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...runtimeresource.ApplyOption) error {
						return errBoom
					}),
				},
			},
			want: want{
				err:    errors.Wrap(errBoom, localPrefix+fmt.Sprintf(errFmtApplyInstance, compositionCRDName)),
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"LocalListFailed": {
			reason: "An error should be returned if local List fails",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
					},
				},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							if o, ok := obj.(*apiextensions.CustomResourceDefinition); ok {
								established.DeepCopyInto(o)
							}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil),
						MockList:   test.NewMockListFn(errBoom),
					},
					Applicator: runtimeresource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...runtimeresource.ApplyOption) error {
						return nil
					}),
				},
			},
			want: want{
				err:    errors.Wrap(errBoom, localPrefix+fmt.Sprintf(errFmtListInstance, compositionCRDName)),
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"RemoteListFailed": {
			reason: "An error should be returned if remote List fails",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:  test.NewMockGetFn(nil),
						MockList: test.NewMockListFn(errBoom),
					},
				},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							if o, ok := obj.(*apiextensions.CustomResourceDefinition); ok {
								established.DeepCopyInto(o)
							}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil),
						MockList:   test.NewMockListFn(nil),
					},
					Applicator: runtimeresource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...runtimeresource.ApplyOption) error {
						return nil
					}),
				},
			},
			want: want{
				err:    errors.Wrap(errBoom, remotePrefix+fmt.Sprintf(errFmtListInstance, compositionCRDName)),
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"LocalDeleteFailed": {
			reason: "An error should be returned if local Delete calls fail",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet:  test.NewMockGetFn(nil),
						MockList: test.NewMockListFn(nil),
					},
				},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							if o, ok := obj.(*apiextensions.CustomResourceDefinition); ok {
								established.DeepCopyInto(o)
							}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil),
						MockList: func(_ context.Context, list runtime.Object, _ ...client.ListOption) error {
							l := &v1alpha1.CompositionList{Items: []v1alpha1.Composition{{}}}
							l.DeepCopyInto(list.(*v1alpha1.CompositionList))
							return nil
						},
						MockDelete: test.NewMockDeleteFn(errBoom),
					},
					Applicator: runtimeresource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...runtimeresource.ApplyOption) error {
						return nil
					}),
				},
			},
			want: want{
				err:    errors.Wrap(errBoom, localPrefix+fmt.Sprintf(errFmtDeleteInstance, compositionCRDName)),
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"DeletesTheCorrectList": {
			reason: "The correct removal list should be deleted",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockList: func(_ context.Context, list runtime.Object, _ ...client.ListOption) error {
							l := &v1alpha1.CompositionList{Items: []v1alpha1.Composition{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "one",
									},
								},
							}}
							l.DeepCopyInto(list.(*v1alpha1.CompositionList))
							return nil
						},
					},
				},
				local: runtimeresource.ClientApplicator{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
							if o, ok := obj.(*apiextensions.CustomResourceDefinition); ok {
								established.DeepCopyInto(o)
							}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil),
						MockList: func(_ context.Context, list runtime.Object, _ ...client.ListOption) error {
							l := &v1alpha1.CompositionList{Items: []v1alpha1.Composition{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "one",
									},
								},
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "two",
									},
								},
							}}
							l.DeepCopyInto(list.(*v1alpha1.CompositionList))
							return nil
						},
						MockDelete: func(_ context.Context, obj runtime.Object, _ ...client.DeleteOption) error {
							c := obj.(*v1alpha1.Composition)
							if c.GetName() != "two" {
								t.Error("an incorrect deletion call is made")
							}
							return nil
						},
					},
					Applicator: runtimeresource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...runtimeresource.ApplyOption) error {
						return nil
					}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: longWait},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewReconciler(tc.args.m, tc.args.local,
				WithGetItemsFn(gi),
				WithNewInstanceFn(ni),
				WithNewObjectListFn(nl),
				WithCRDName(compositionCRDName))
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
