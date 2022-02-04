/*


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

package cluster

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler reconciles an OpenStackCluster object
type Reconciler struct {
	awsSession *session.Session
	client     client.Client
	logger     logr.Logger

	baseDomain        string
	managementCluster string
}

type Config struct {
	AWSSession *session.Session
	Client     client.Client
	Logger     logr.Logger

	BaseDomain        string
	ManagementCluster string
}
