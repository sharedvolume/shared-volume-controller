/*
Copyright 2025.

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

package v1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// SetupPodWebhookWithManager sets up the Pod webhook with the manager
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	// Register the Pod mutating webhook
	mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{
		Handler: &PodAnnotator{Client: mgr.GetClient()},
	})
	return nil
}
