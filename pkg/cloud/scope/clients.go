package scope

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/route53"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/version"

	"github.com/giantswarm/dns-operator-route53/pkg/cloud"
	awsmetrics "github.com/giantswarm/dns-operator-route53/pkg/cloud/metrics"
	"github.com/giantswarm/dns-operator-route53/pkg/record"
)

// AWSClients contains all the aws clients used by the scopes
type AWSClients struct {
	Route53 *route53.Route53
}

// NewRoute53Client creates a new Route53 API client for a given session
func NewRoute53Client(session cloud.Session, target runtime.Object) *route53.Route53 {
	Route53Client := route53.New(session.Session(), &aws.Config{Credentials: credentials.NewSharedCredentials("", "")})
	Route53Client.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	Route53Client.Handlers.CompleteAttempt.PushFront(awsmetrics.CaptureRequestMetrics("dns-operator-openstack"))
	Route53Client.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return Route53Client
}

func getUserAgentHandler() request.NamedHandler {
	return request.NamedHandler{
		Name: "dns-operator-openstack/user-agent",
		Fn:   request.MakeAddToUserAgentHandler("openstack.cluster.x-k8s.io", version.Get().String()),
	}
}

func recordAWSPermissionsIssue(target runtime.Object) func(r *request.Request) {
	return func(r *request.Request) {
		if awsErr, ok := r.Error.(awserr.Error); ok {
			switch awsErr.Code() {
			case "AuthFailure", "UnauthorizedOperation", "NoCredentialProviders":
				record.Warnf(target, awsErr.Code(), "Operation %s failed with a credentials or permission issue", r.Operation.Name)
			}
		}
	}
}
