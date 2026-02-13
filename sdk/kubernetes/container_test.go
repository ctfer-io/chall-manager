package kubernetes_test

import (
	"sync"
	"testing"

	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

func Test_U_Container(t *testing.T) {
	t.Parallel()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		args := k8s.ContainerArgs{
			Image: pulumi.String("pandatix/license-lvl1:latest"),
			Ports: k8s.PortBindingArray{
				k8s.PortBindingArgs{
					Port: pulumi.Int(8080),
				},
			},
			Files: pulumi.StringMap{
				"/app/flag.txt": pulumi.String("BREFCTF{flag}"),
			},
			Requests: nil,
			Limits:   nil,
		}

		wg := sync.WaitGroup{}
		wg.Add(1)
		args.ToContainerOutput().ApplyT(func(c k8s.Container) error {
			defer wg.Done()

			assert.NotEmpty(t, c.Image, "image")
			assert.NotEmpty(t, c.Files, "files")
			assert.Empty(t, c.Requests, "requests")
			assert.Empty(t, c.Limits, "limits")

			return nil
		})
		wg.Wait()
		return nil
	}, pulumi.WithMocks("project", "stack", mocks{}))
	assert.NoError(t, err)
}
