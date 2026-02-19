package serviceaccount

import (
	"github.com/ctfer-io/chall-manager/deploy/services"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
	"github.com/pkg/errors"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func Program() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := loadConfig(ctx)

		ns, err := parts.NewNamespace(ctx, "sa-ns", &parts.NamespaceArgs{})
		if err != nil {
			return errors.Wrap(err, "creating custom namespace")
		}

		role, err := rbacv1.NewClusterRole(ctx, "cluster-role", &rbacv1.ClusterRoleArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: ns.Name,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Rules: rbacv1.PolicyRuleArray{
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.ToStringArray([]string{
						"",
					}),
					Resources: pulumi.ToStringArray([]string{
						"namespaces",
					}),
					Verbs: pulumi.ToStringArray([]string{
						"create",
						"delete",
						"get",
						"list", // required to list resources in namespaces (queries)
						"patch",
						"update",
						"watch", // required to monitor resources when deployed/updated, else will get stucked
					}),
				},
			},
		})
		if err != nil {
			return err
		}

		// => ServiceAccount
		sa, err := corev1.NewServiceAccount(ctx, "chall-manager-account", &corev1.ServiceAccountArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: ns.Name,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
		})
		if err != nil {
			return err
		}

		// => RoleBinding, binds the ClusterRole and ServiceAccount
		_, err = rbacv1.NewClusterRoleBinding(ctx, "chall-manager-cluster-role-binding", &rbacv1.ClusterRoleBindingArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: ns.Name,
				Name:      pulumi.String("ctfer-io:chall-manager:cluster-wide"),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			RoleRef: rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     role.Metadata.Name().Elem(),
			},
			Subjects: rbacv1.SubjectArray{
				rbacv1.SubjectArgs{
					Kind:      pulumi.String("ServiceAccount"),
					Name:      sa.Metadata.Name().Elem(),
					Namespace: ns.Name,
				},
			},
		})
		if err != nil {
			return err
		}

		cm, err := services.NewChallManager(ctx, "chall-manager", &services.ChallManagerArgs{
			Namespace:      ns.Name,
			Tag:            pulumi.String(cfg.Tag),
			Registry:       pulumi.String(cfg.Registry),
			OCIInsecure:    true,
			Expose:         true,
			ServiceAccount: sa.Metadata.Name().Elem(),
		})
		if err != nil {
			return errors.Wrap(err, "deploying chall-manager")
		}

		ctx.Export("exposed_port", cm.ExposedPort)

		return nil
	})
}

type Config struct {
	Tag      string
	Registry string
}

func loadConfig(ctx *pulumi.Context) *Config {
	cfg := config.New(ctx, "")
	return &Config{
		Tag:      cfg.Get("tag"),
		Registry: cfg.Get("registry"),
	}
}
