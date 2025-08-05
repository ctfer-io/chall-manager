package integration_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

const (
	img     = "library/busybox:1.37.0"
	podName = "extractor"
)

func Test_T_Monitoring(t *testing.T) {
	cwd, _ := os.Getwd()

	sn := stackName(t.Name())
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		StackName:   sn,
		Dir:         filepath.Join(cwd, "monitoring"),
		Config: map[string]string{
			"namespace":        os.Getenv("NAMESPACE"),
			"registry":         os.Getenv("REGISTRY"),
			"tag":              os.Getenv("TAG"),
			"romeo-claim-name": os.Getenv("ROMEO_CLAIM_NAME"),
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			ctx := t.Context()

			// First stimulate a bit the system
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)

			_, err := chlCli.QueryChallenge(ctx, nil)
			require.NoError(t, err)

			// Wait enough for the traces to be sent to the OTEL collector,
			// then for the file processor to write it on disk
			time.Sleep(2 * time.Minute)

			// Then dump OTEL collector files
			dir := t.TempDir()
			err = dumpOtelCollector(ctx, dir,
				stack.Outputs["mon.namespace"].(string),
				stack.Outputs["mon.cold-extract-pvc-name"].(string),
			)
			require.NoError(t, err, "dumping OTEL collector files")

			// And check it contains spans from Chall-Manager and the janitor
			f, err := os.Open(filepath.Join(dir, "otel_traces"))
			require.NoError(t, err)
			defer f.Close()

			var foundCM, foundCMJ = false, false

			tracesUnmarshaler := &ptrace.JSONUnmarshaler{}
			scan := bufio.NewScanner(f)
			for scan.Scan() {
				if foundCM && foundCMJ {
					break
				}

				sb := scan.Bytes()
				fmt.Printf("sb: %s\n", sb)
				tr, err := tracesUnmarshaler.UnmarshalTraces(sb)
				require.NoError(t, err)

				for i := 0; i < tr.ResourceSpans().Len(); i++ {
					rs := tr.ResourceSpans().At(i)
					attrs := rs.Resource().Attributes().AsRaw()
					svcName, hasSvcName := attrs["service.name"]
					if !hasSvcName {
						continue
					}
					foundCM = foundCM || strings.HasSuffix(svcName.(string), "chall-manager")
					foundCMJ = foundCMJ || strings.HasSuffix(svcName.(string), "chall-manager-janitor")
				}
			}

			assert.True(t, foundCM, "found chall-manager spans")
			assert.True(t, foundCMJ, "found chall-manager-janitor spans")
		},
	})
}

func dumpOtelCollector(ctx context.Context, dir, namespace, pvcName string) error {
	clientset, config, err := getClient()
	if err != nil {
		return err
	}

	if _, err := clientset.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      podName,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "copy",
					Image:   img,
					Command: []string{"/bin/sh", "-c", "--"},
					Args:    []string{"sleep infinity"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/data",
						},
					},
					// The following matches the pod security policy "restricted".
					// It is not required for the extractor to work, but is a good
					// practice, plus we don't need large capabilities so it's OK.
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsUser:    ptr(int64(1000)), // Don't need to be root !
						RunAsNonRoot: ptr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}
	defer func() {
		_ = clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	}()

	if err := waitForPodReady(ctx, clientset, namespace, podName); err != nil {
		return err
	}

	return copyFromPod(ctx, config, clientset, namespace, podName, "copy", "/data", dir)
}

func getClient() (*kubernetes.Clientset, *rest.Config, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return clientset, config, nil
}

func waitForPodReady(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func copyFromPod(
	ctx context.Context,
	config *rest.Config,
	clientset *kubernetes.Clientset,
	namespace, podName, containerName, podPath, localDir string,
) error {
	req := clientset.CoreV1().RESTClient().
		Get().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		Param("container", containerName).
		Param("stdout", "true").
		Param("stderr", "true").
		Param("command", "tar").
		Param("command", "cf").
		Param("command", "-").
		Param("command", "-C").
		Param("command", podPath).
		Param("command", ".")

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return fmt.Errorf("stream error: %v\nstderr: %s", err, stderr.String())
	}

	// untar locally
	return untar(bytes.NewReader(stdout.Bytes()), localDir)
}

func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := path.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(path.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func ptr[T any](t T) *T {
	return &t
}
