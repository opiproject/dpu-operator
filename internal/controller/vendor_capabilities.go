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
	"strings"

	configv1 "github.com/openshift/dpu-operator/api/v1"
)

// RequiresProvider reports whether this vendor token needs an out-of-tree
// VendorProvider companion. Intel, Marvell, and empty never do.
func RequiresProvider(vendor string) bool {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case configv1.VendorNvidia:
		return true
	default:
		return false
	}
}
