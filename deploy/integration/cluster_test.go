package integration_test

import (
	"os"
	"path"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
)

func Test_I_Cluster(t *testing.T) {
	// This is a smoke test for a production-grade deployment.
	// We do not deploy replicas to avoid overloading CI, and only
	// require to check it could be deployed as a cluster.
	//
	// We do not run extra tests as we expect no impact on features.

	pwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(pwd, ".."),
		StackName:   stackName(t.Name()),
		Config: map[string]string{
			"namespace":        os.Getenv("NAMESPACE"),
			"registry":         os.Getenv("REGISTRY"),
			"tag":              os.Getenv("TAG"),
			"romeo-claim-name": os.Getenv("ROMEO_CLAIM_NAME"),
			"pvc-access-mode":  "ReadWriteOnce", // run 1 replica on 1 node, on need for RWX
			"replicas":         "1",             // no need to replicate, we test proper deployments
			"etcd-replicas":    "1",             // no need to replicate, we test proper deployment
		},
	})
}
