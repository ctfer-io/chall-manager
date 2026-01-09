package integration_test

import (
	"os"
	"path"
	"testing"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"
)

func Test_I_Cluster(t *testing.T) {
	// This is a smoke test for a production-grade deployment.
	// We do not deploy replicas to avoid overloading CI, and only
	// require to check it could be deployed as a cluster.
	//
	// We simply issue some read (QueryXXX) to trigger a bit the locks.

	require.NotEmpty(t, Server)

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
			"oci-insecure":     "true",          // don't mind HTTPS on the CI registry
			"pvc-access-mode":  "ReadWriteOnce", // run 1 replica on 1 node, no need for RWX
			"replicas":         "1",             // no need to replicate, we test proper deployments
			"etcd-replicas":    "1",             // no need to replicate, we test proper deployment
			"expose":           "true",          // make API externally reachable
		},
		Secrets: map[string]string{
			"kubeconfig": "",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)
			istCli := instance.NewInstanceManagerClient(cli)

			ctx := t.Context()

			// Should not fail -> there is nothing, no problem
			_, err := chlCli.QueryChallenge(ctx, nil)
			require.NoError(t, err)

			// Won't fail becuse there are no challenges configured :)
			// If there was one, it would...
			_, err = istCli.QueryInstance(ctx, &instance.QueryInstanceRequest{
				SourceId: randomId(),
			})
			require.NoError(t, err)
		},
	})
}
