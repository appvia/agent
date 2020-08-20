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

package resource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// SanitizedDeepCopyObject removes the metadata that can be specific to a cluster.
// For example, owner references are references to resources in that cluster and
// would be meaningless in another one.
func SanitizedDeepCopyObject(in runtime.Object) resource.Object {
	out, _ := in.DeepCopyObject().(resource.Object)
	out.SetResourceVersion("")
	out.SetUID("")
	out.SetCreationTimestamp(metav1.Unix(0, 0))
	out.SetSelfLink("")
	out.SetOwnerReferences(nil)
	out.SetManagedFields(nil)
	out.SetFinalizers(nil)
	return out
}
