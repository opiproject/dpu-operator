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

import "testing"

func TestRequiresProvider(t *testing.T) {
	cases := []struct {
		vendor string
		want   bool
	}{
		{"", false},
		{"intel", false},
		{"marvell", false},
		{"Intel", false},
		{"nvidia", true},
		{"NVIDIA", true},
		{" nvidia ", true},
		{"amd", false},
	}
	for _, tc := range cases {
		if got := RequiresProvider(tc.vendor); got != tc.want {
			t.Errorf("RequiresProvider(%q)=%v, want %v", tc.vendor, got, tc.want)
		}
	}
}
